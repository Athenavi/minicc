package api

import (
	"net/http"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/engine"
)

type ChatHandler struct {
	engine *engine.Engine
}

func NewChatHandler(eng *engine.Engine) *ChatHandler {
	return &ChatHandler{engine: eng}
}

type ChatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

func (h *ChatHandler) Chat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := DecodeJSON(w, r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Message == "" {
		BadRequest(w, "message is required")
		return
	}

	claims := auth.GetClaims(r.Context())

	// Process via ReAct engine
	result, usage, err := h.engine.ExecuteTask(r.Context(), req.Message, req.SessionID, nil)
	if err != nil {
		InternalError(w, "processing failed: "+err.Error())
		return
	}

	OK(w, map[string]interface{}{
		"response":   result,
		"usage":      usage,
		"session_id": req.SessionID,
		"user_id":    claims.UserID,
	})
}
