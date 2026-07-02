package api

import (
	"net/http"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/llm"
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
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Message == "" {
		BadRequest(w, "message is required")
		return
	}

	claims := auth.GetClaims(r.Context())

	// Build messages
	messages := []llm.Message{
		{Role: "system", Content: "You are MiniCC V2, an AI coding assistant."},
		{Role: "user", Content: req.Message},
	}

	// Process
	result, err := h.engine.ProcessTurn(r.Context(), messages)
	if err != nil {
		InternalError(w, "processing failed: "+err.Error())
		return
	}

	OK(w, map[string]interface{}{
		"response":     result.Content,
		"usage":        result.Usage,
		"session_id":   req.SessionID,
		"user_id":      claims.UserID,
	})
}
