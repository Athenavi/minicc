package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    map[string]string{"status": "ok"},
	})
}

func handleReadiness(w http.ResponseWriter, r *http.Request) {
	// 浜у搧鍐崇瓥(2026-08-22)锛歊edis 涓哄繀闇€渚濊禆锛涘氨缁鏌ュ弽鏄犵湡瀹炰緷璧栫姸鎬侊紝
	// 渚涚紪鎺掑櫒(compose/K8s)鍦?Redis 鏁呴殰鏃惰Е鍙戦噸鍚€?	deps := map[string]string{
		"postgres": "up",
		"redis":    "up",
	}
	ready := true
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if db.Pool == nil || db.Pool.Ping(ctx) != nil {
		deps["postgres"] = "down"
		ready = false
	}
	if db.Redis == nil || db.Redis.Ping(ctx).Err() != nil {
		deps["redis"] = "down"
		ready = false
	}
	if !ready {
		JSON(w, http.StatusServiceUnavailable, APIResponse{
			Success: false,
			Error:   "dependencies not ready",
			Data:    deps,
		})
		return
	}
	JSON(w, http.StatusOK, APIResponse{
		Success: true,
		Data:    deps,
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
