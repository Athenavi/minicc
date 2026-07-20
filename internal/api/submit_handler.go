package api

import (
	"context"
	"log/slog"
	"time"

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

// HandleSubmit proxies the submit request to Python engine and streams SSE events.
func (h *SubmitHandler) HandleSubmit(ctx context.Context, userID, sessionID, content string, llmConfig map[string]interface{}) {
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	if sessionID != "" {
		sessionCancels.Store(sessionID, cancel)
		defer sessionCancels.Delete(sessionID)
	}

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

	events, err := h.python.RunSSE(ctx, "/v1/agent/submit", pythonReq)
	if err != nil {
		slog.Error("submit: python proxy failed", "error", err)
		h.eventHub.Publish(broadcast.Event{Type: "text", SessionID: sessionID, Data: map[string]string{"content": "Service temporarily unavailable. Please try again."}})
		h.eventHub.Publish(broadcast.Event{Type: "turn_done", SessionID: sessionID, Data: map[string]string{"session_id": sessionID}})
		return
	}

	var finalContent string
	var inputTokens, outputTokens int

	for evt := range events {
		h.eventHub.Publish(broadcast.Event{Type: evt.Type, SessionID: sessionID, Data: evt})
		if evt.Content != "" && evt.Type == "text" {
			finalContent += evt.Content
		}
		if evt.InputTokens > 0 {
			inputTokens += evt.InputTokens
		}
		if evt.OutputTokens > 0 {
			outputTokens += evt.OutputTokens
		}
	}

	if finalContent != "" {
		h.sessionMgr.SaveMessages(ctx, sessionID, userID, content, finalContent)
	}

	if inputTokens > 0 || outputTokens > 0 {
		if h.biller != nil {
			// 检查是否仍在免费额度内
			freeCount, fcErr := h.biller.DailyFreeCount(ctx, userID)
			if fcErr == nil && freeCount < billing.DailyFreeLimit {
				// 免费对话：记录使用，不扣费
				if markErr := h.biller.MarkFreeUsage(ctx, userID); markErr != nil {
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
