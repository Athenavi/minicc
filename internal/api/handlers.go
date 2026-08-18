package api

import (
	"log/slog"
	"net/http"

	"github.com/athenavi/minicc/internal/auth"
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
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	if v, ok := sessionCancels.Load(sessionID); ok {
		sc := v.(sessionCancel)
		if sc.userID != claims.UserID {
			Forbidden(w, "not your session")
			return
		}
		sessionCancels.Delete(sessionID)
		sc.cancel()
		slog.Info("session cancelled", "session_id", sessionID)
		OK(w, map[string]string{"status": "cancelled", "session_id": sessionID})
	} else {
		OK(w, map[string]string{"status": "no_active_task", "session_id": sessionID})
	}
}
