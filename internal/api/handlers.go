package api

import (
	"context"
	"log/slog"
	"net/http"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"status": "ok"},
	})
}

func handleReadiness(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"status": "ready"},
	})
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		BadRequest(w, "session_id is required")
		return
	}
	if cancel, ok := sessionCancels.LoadAndDelete(sessionID); ok {
		cancel.(context.CancelFunc)()
		slog.Info("session cancelled", "session_id", sessionID)
		OK(w, map[string]string{"status": "cancelled", "session_id": sessionID})
	} else {
		OK(w, map[string]string{"status": "no_active_task", "session_id": sessionID})
	}
}
