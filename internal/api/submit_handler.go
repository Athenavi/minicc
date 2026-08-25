package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/billing"
	"github.com/athenavi/chiron/internal/broadcast"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/athenavi/chiron/internal/session"
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
// (S 瀹夊叏淇锛氬伐鍏蜂笁鎬佹爡鏍忕殑鈥滅‘璁も€濇€?鈥?鍓嶇纭鍗＄墖鍥炶皟杩欓噷).
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
	// S 淇锛氭妸宸查獙璇?JWT claims 鐨?user_id 涓€骞堕€忎紶锛屼緵 Python 绔牎楠?	// 鏉ョ數鑰呮槸鍚︿负浼氳瘽 owner锛岄槻姝粬浜轰唬鎵?鎷掓壒鍗遍櫓宸ュ叿銆?	if claims := auth.GetClaims(r.Context()); claims != nil {
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
	// P1 淇锛氫笌 Python 寮曟搸 5min 瀹㈡埛绔秴鏃跺榻愶紝閬垮厤闀夸换鍔¤ 180s 纭秴鏃舵埅鏂?	ctx, cancel := context.WithTimeout(ctx, 300*time.Second)
	defer cancel()
	if sessionID != "" {
		sessionCancels.Store(sessionID, sessionCancel{userID: userID, cancel: cancel})
		defer sessionCancels.Delete(sessionID)
	}

	// 钀藉簱涓撶敤 ctx锛氫笉缁ф壙涓?ctx 鐨勫彇娑?瓒呮椂銆?	// 娴佸彲鑳借 180s 瓒呮椂銆佸墠绔柇寮€銆佷細璇濆彇娑堢瓑鎴柇锛屼絾宸蹭骇鐢熺殑娑堟伅
	// 锛坲ser/assistant/tool_call锛夊繀椤诲啓鍏ワ紝鍚﹀垯鍒锋柊鍚庡璇濅涪澶便€?	storeCtx, storeCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer storeCancel()

	// S 淇锛氫笂涓嬫枃涓㈠け 鈥?鎻愪氦鏃剁珛鍗虫寔涔呭寲鐢ㄦ埛娑堟伅锛圫SE 涓柇/鍋滄涔熶笉涓㈠巻鍙诧級
	h.sessionMgr.SaveUserMessage(storeCtx, sessionID, userID, content)

	histMsgs := make([]map[string]string, 0)
	if hist, err := h.sessionMgr.GetMessages(ctx, sessionID, 50); err == nil && len(hist) > 0 {
		// 鍙繚鐣欐渶杩?8 鏉℃秷鎭紙Python SessionStore 鏈夊畬鏁寸紦瀛橈級
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

	// 榛樿 max_turns锛岃嫢 llm_config 涓湁鍒欎娇鐢ㄥ墠绔寚瀹氱殑鍊?	defaultMaxTurns := 5
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
	turnToolCallIDs := []string{} // S 淇锛歮essages.tool_calls 鍒楀彧瀛?tool_call id 闆嗗悎锛堝唴瀹瑰湪 tool_calls 琛級

	// P 鎬ц兘锛歵ext 浜嬩欢 50ms 鍚堝抚杞彂锛堝叕缃戝绉熸埛涓?SSE 甯ф暟鍑忓崐锛屽噺灏戠綉鍏?鍓嶇澶勭悊寮€閿€锛?	const textFrameInterval = 50 * time.Millisecond
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
		// 鎬濊€冧簨浠讹紙[thinking] 鍓嶇紑锛変笉鍙備笌鍚堝抚锛氳繃绋嬫€у唴瀹归渶鍗虫椂閫愭鎺ㄩ€侊紝
		// 鍚﹀垯姣绾у埌杈剧殑 thinking 鐗囨浼氳 50ms 鍚堝抚鍚堝苟鎴愭暣娈碉紙鎬濊€冧笉娴佸紡锛夈€?		isThinking := strings.HasPrefix(evt.Content, "[thinking]")
		if evt.Type == "text" && evt.Content != "" && !isThinking {
			textBuf.WriteString(evt.Content)
			if time.Since(lastTextFlush) >= textFrameInterval {
				flushText()
			}
		} else {
			flushText() // 闈?text 浜嬩欢鍏堝啿鍒风紦鍐诧紝淇濇寔椤哄簭
			h.eventHub.Publish(broadcast.Event{Type: evt.Type, SessionID: sessionID, Data: evt})
		}
		// S 淇锛氬伐鍏疯皟鐢ㄨ繃绋嬭惤搴擄紙tool_call 璁板綍 + tool_result 鍥炲～锛夛紝鍒锋柊鍚庢樉绀轰竴鑷?		switch evt.Type {
		case "tool_call":
			h.sessionMgr.SaveToolCall(storeCtx, sessionID, evt.ID, evt.Name, evt.Arguments)
			if evt.ID != "" {
				turnToolCallIDs = append(turnToolCallIDs, evt.ID)
			}
		case "tool_result":
			h.sessionMgr.UpdateToolCall(storeCtx, evt.ID, evt.Content, strings.Contains(evt.Content, `"error"`))
		case "guardrail_blocked":
			// SaaS 鍚堣锛氭爡鏍忔嫆缁濈暀鐥曪紙杈撳叆娉ㄥ叆/杈撳嚭娉勯湶/宸ュ叿 block 瀹¤锛?			h.sessionMgr.SaveToolCall(storeCtx, sessionID,
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
	flushText() // 娴佺粨鏉熷厹搴曞啿鍒?
	if finalContent != "" || len(turnToolCallIDs) > 0 {
		// S 淇锛氱函宸ュ叿璋冪敤杞紙鏃犳枃鏈級涔熶繚瀛?assistant 娑堟伅锛?		// messages.tool_calls 鍒楀彧瀛?id 闆嗗悎锛堝唴瀹瑰湪 tool_calls 琛紝閬垮厤閲嶅瀛樺偍锛?		toolCallsJSON, _ := json.Marshal(turnToolCallIDs)
		h.sessionMgr.SaveAssistantMessage(storeCtx, sessionID, finalContent, string(toolCallsJSON))
	} else {
		// 鏃犳枃鏈棤宸ュ叿锛氫粎鐢ㄦ埛娑堟伅宸茬敱 SaveUserMessage 鎸佷箙鍖?	}

	if inputTokens > 0 || outputTokens > 0 {
		if h.biller != nil {
			// 妫€鏌ユ槸鍚︿粛鍦ㄥ厤璐归搴﹀唴
			freeCount, fcErr := h.biller.DailyFreeCount(storeCtx, userID)
			if fcErr == nil && freeCount < billing.DailyFreeLimit {
				// 鍏嶈垂瀵硅瘽锛氳褰曚娇鐢紝涓嶆墸璐?				if markErr := h.biller.MarkFreeUsage(storeCtx, userID); markErr != nil {
					slog.Error("billing: MarkFreeUsage failed", "user", userID, "error", markErr)
				}
			} else {
				// 瓒呭嚭鍏嶈垂棰濆害鎴栨煡璇㈠け璐ワ細姝ｅ父鎵ｈ垂
				if _, err := h.biller.DeductTokens(userID, inputTokens, outputTokens); err != nil {
					slog.Error("billing: DeductTokens failed", "user", userID, "error", err)
				}
			}
		}
	}

	h.eventHub.Publish(broadcast.Event{Type: "turn_done", SessionID: sessionID, Data: map[string]string{"session_id": sessionID}})
}
