package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/billing"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/session"
)

// SubmitHandler proxies /submit requests to the Python AI engine.
type SubmitHandler struct {
	python     *engine.PythonClient
	sessionMgr *session.Manager
	eventHub   *broadcast.Hub
	biller     engine.Biller
}

func NewSubmitHandler(python *engine.PythonClient, sessionMgr *session.Manager, eventHub *broadcast.Hub, biller engine.Biller) *SubmitHandler {
	return &SubmitHandler{
		python:     python,
		sessionMgr: sessionMgr,
		eventHub:   eventHub,
		biller:     biller,
	}
}

// SubmitApproval proxies the user's tool-approval decision to the Python engine
// (S 安全修复：工具三态栅栏的“确认”态 — 前端确认卡片回调这里).
func (h *SubmitHandler) SubmitApproval(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID  string `json:"session_id"`
		ToolCallID string `json:"tool_call_id"`
		Approved   bool   `json:"approved"`
		Reason     string `json:"reason"`
		UserID     string `json:"user_id,omitempty"`
	}
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if req.SessionID == "" || req.ToolCallID == "" {
		BadRequest(w, "session_id and tool_call_id are required")
		return
	}
	var out map[string]any
	// S 修复：把已验证 JWT claims 的 user_id 一并透传，供 Python 端校验
	// 来电者是否为会话 owner，防止他人代批/拒批危险工具。
	if claims := auth.GetClaims(r.Context()); claims != nil {
		req.UserID = claims.UserID
	}
	if err := h.python.PostJSON(r.Context(), "/v1/agent/approval", req, &out); err != nil {
		slog.Error("approval: python proxy failed", "session", req.SessionID, "error", err)
		InternalError(w, "approval proxy failed")
		return
	}
	JSON(w, http.StatusOK, APIResponse{Success: true, Data: out})
}

// HandleSubmit proxies the submit request to Python engine and streams SSE events.
func (h *SubmitHandler) HandleSubmit(ctx context.Context, userID, sessionID, content string, llmConfig map[string]interface{}) {
	// P1 修复：与 Python 引擎 5min 客户端超时对齐，避免长任务被 180s 硬超时截断
	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	if sessionID != "" {
		sessionCancels.Store(sessionID, sessionCancel{userID: userID, cancel: cancel})
		defer sessionCancels.Delete(sessionID)
	}

	// 落库专用 ctx：不继承主 ctx 的取消/超时。
	// 流可能被 180s 超时、前端断开、会话取消等截断，但已产生的消息
	// （user/assistant/tool_call）必须写入，否则刷新后对话丢失。
	storeCtx, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()

	// S 修复：上下文丢失 — 提交时立即持久化用户消息（SSE 中断/停止也不丢历史）
	h.sessionMgr.SaveUserMessage(storeCtx, sessionID, userID, content)

	histMsgs := make([]map[string]string, 0)
	if hist, err := h.sessionMgr.GetMessages(ctx, sessionID, 50); err == nil && len(hist) > 0 {
		// 只保留最近 8 条消息（Python SessionStore 有完整缓存）
		const maxHistory = 8
		start := 0
		if len(hist) > maxHistory {
			start = len(hist) - maxHistory
		}
		for _, m := range hist[start:] {
			if (m.Role == "user" || m.Role == "assistant" || m.Role == "tool") && m.Content != "" {
				histMsgs = append(histMsgs, map[string]string{"role": m.Role, "content": m.Content})
			}
		}
	}

	// 默认 max_turns，若 llm_config 中有则使用前端指定的值
	defaultMaxTurns := 5
	if llmConfig != nil {
		if mt, ok := llmConfig["max_turns"].(float64); ok && mt > 0 {
			defaultMaxTurns = int(mt)
		}
	}
	pythonReq := map[string]interface{}{
		"session_id": sessionID,
		"user_id":    userID,
		"content":    content,
		"history":    histMsgs,
		"max_turns":  defaultMaxTurns,
	}
	if llmConfig != nil {
		pythonReq["llm_config"] = llmConfig
	}

	events, err := h.python.RunSSE(ctx, "/v1/agent/submit", pythonReq,
		map[string]string{"X-User-ID": userID})
	if err != nil {
		slog.Error("submit: python proxy failed", "error", err)
		h.eventHub.Publish(broadcast.Event{Type: "text", SessionID: sessionID, Data: map[string]string{"content": "Service temporarily unavailable. Please try again."}})
		h.eventHub.Publish(broadcast.Event{Type: "turn_done", SessionID: sessionID, Data: map[string]string{"session_id": sessionID}})
		return
	}

	var finalContent string
	var inputTokens, outputTokens int
	turnToolCallIDs := []string{} // S 修复：messages.tool_calls 列只存 tool_call id 集合（内容在 tool_calls 表）

	// P 性能：text 事件 50ms 合帧转发（公网多租户下 SSE 帧数减半，减少网关/前端处理开销）
	const textFrameInterval = 50 * time.Millisecond
	var textBuf strings.Builder
	lastTextFlush := time.Now()
	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		payload := textBuf.String()
		textBuf.Reset()
		finalContent += payload
		h.eventHub.Publish(broadcast.Event{
			Type: "text", SessionID: sessionID,
			Data: engine.PythonEvent{Type: "text", Content: payload},
		})
		lastTextFlush = time.Now()
	}

	for evt := range events {
		// 思考事件（[thinking] 前缀）不参与合帧：过程性内容需即时逐段推送，
		// 否则毫秒级到达的 thinking 片段会被 50ms 合帧合并成整段（思考不流式）。
		isThinking := strings.HasPrefix(evt.Content, "[thinking]")
		if evt.Type == "text" && evt.Content != "" && !isThinking {
			textBuf.WriteString(evt.Content)
			if time.Since(lastTextFlush) >= textFrameInterval {
				flushText()
			}
		} else {
			flushText() // 非 text 事件先冲刷缓冲，保持顺序
			h.eventHub.Publish(broadcast.Event{Type: evt.Type, SessionID: sessionID, Data: evt})
		}
		// S 修复：工具调用过程落库（tool_call 记录 + tool_result 回填），刷新后显示一致
		switch evt.Type {
		case "tool_call":
			h.sessionMgr.SaveToolCall(storeCtx, sessionID, evt.ID, evt.Name, evt.Arguments)
			if evt.ID != "" {
				turnToolCallIDs = append(turnToolCallIDs, evt.ID)
			}
		case "tool_result":
			h.sessionMgr.UpdateToolCall(storeCtx, evt.ID, evt.Content, strings.Contains(evt.Content, `"error"`))
		case "guardrail_blocked":
			// SaaS 合规：栅栏拒绝留痕（输入注入/输出泄露/工具 block 审计）
			h.sessionMgr.SaveToolCall(storeCtx, sessionID,
				"guard_"+evt.ID, "guardrail",
				fmt.Sprintf(`{"reason":%q}`, evt.Content))
		}
		if evt.InputTokens > 0 {
			inputTokens += evt.InputTokens
		}
		if evt.OutputTokens > 0 {
			outputTokens += evt.OutputTokens
		}
	}
	flushText() // 流结束兜底冲刷

	if finalContent != "" || len(turnToolCallIDs) > 0 {
		// S 修复：纯工具调用轮（无文本）也保存 assistant 消息；
		// messages.tool_calls 列只存 id 集合（内容在 tool_calls 表，避免重复存储）
		toolCallsJSON, _ := json.Marshal(turnToolCallIDs)
		h.sessionMgr.SaveAssistantMessage(storeCtx, sessionID, finalContent, string(toolCallsJSON))
	} else {
		// 无文本无工具：仅用户消息已由 SaveUserMessage 持久化
	}

	if inputTokens > 0 || outputTokens > 0 {
		if h.biller != nil {
			// 检查是否仍在免费额度内
			freeCount, fcErr := h.biller.DailyFreeCount(storeCtx, userID)
			if fcErr == nil && freeCount < billing.DailyFreeLimit {
				// 免费对话：记录使用，不扣费
				if markErr := h.biller.MarkFreeUsage(storeCtx, userID); markErr != nil {
					slog.Error("billing: MarkFreeUsage failed", "user", userID, "error", markErr)
				}
			} else {
				// 超出免费额度或查询失败：正常扣费
				if _, err := h.biller.DeductTokens(userID, inputTokens, outputTokens); err != nil {
					slog.Error("billing: DeductTokens failed", "user", userID, "error", err)
				}
			}
		}
	}

	h.eventHub.Publish(broadcast.Event{Type: "turn_done", SessionID: sessionID, Data: map[string]string{"session_id": sessionID}})
}
