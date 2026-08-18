package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/session"
)

// handleSSE manages a Server-Sent Events connection for real-time streaming.
// It subscribes to the event hub and writes events to the response writer.
// When sessionID is non-empty, only events matching that session (or system events with no session) are forwarded.
func handleSSE(w http.ResponseWriter, r *http.Request, hub *broadcast.Hub, subID string, sessionID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		InternalError(w, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := hub.Subscribe(subID)
	defer hub.Unsubscribe(subID)

	// Send initial connected event
	w.Write([]byte(broadcast.FormatSSE(broadcast.Event{Type: "connected", Data: map[string]string{"id": subID}})))
	flusher.Flush()

	pingTimer := time.NewTimer(15 * time.Second)
	defer pingTimer.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			// Filter by session: skip events scoped to a different session
			if sessionID != "" && event.SessionID != "" && event.SessionID != sessionID {
				continue
			}
			w.Write([]byte(broadcast.FormatSSE(event)))
			flusher.Flush()
			// Reset ping timer after activity
			if !pingTimer.Stop() {
				select {
				case <-pingTimer.C:
				default:
				}
			}
			pingTimer.Reset(15 * time.Second)
		case <-pingTimer.C:
			// Keep-alive ping
			w.Write([]byte(": ping\n\n"))
			flusher.Flush()
			pingTimer.Reset(15 * time.Second)
		}
	}
}

// SSEHandler returns an http.HandlerFunc for SSE connections.
// Requires authentication (authMW) — session_id is checked for ownership
// against the authenticated user (S1: prevent subscribing to other users' streams).
func SSEHandler(hub *broadcast.Hub, sessionMgr *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subID := r.URL.Query().Get("client_id")
		if subID == "" {
			var buf [8]byte
			rand.Read(buf[:])
			subID = "anon-" + hex.EncodeToString(buf[:])
		}
		sessionID := r.URL.Query().Get("session_id")
		if sessionID != "" {
			claims := auth.GetClaims(r.Context())
			if claims == nil {
				Unauthorized(w, ErrAuthRequired)
				return
			}
			if sessionMgr != nil {
				s, err := sessionMgr.GetSession(r.Context(), sessionID)
				if err != nil {
					// 新会话：前端先建立 SSE 连接，/submit 才会创建 session。
					// 此时会话尚不存在、无历史事件可泄露，放行连接等待创建；
					// 其他错误（DB 故障等）拒绝。
					if !errors.Is(err, session.ErrSessionNotFound) {
						InternalError(w, "session check failed")
						return
					}
				} else if s == nil || s.UserID != claims.UserID {
					Forbidden(w, "session does not belong to the current user")
					return
				}
			}
		}
		handleSSE(w, r, hub, subID, sessionID)
	}
}
