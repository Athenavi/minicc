package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/broadcast"
	"github.com/athenavi/chiron/internal/session"
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

	// P1 淇锛歋SE 闀胯繛鎺ヨ眮鍏嶆湇鍔″櫒 WriteTimeout锛堥粯璁?60s 浼氬垏鏂祦锛夈€?	// 瀹㈡埛绔柇寮€浠嶇敱 r.Context().Done() 妫€娴嬨€?	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}

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
// Requires authentication (authMW) 鈥?session_id is checked for ownership
// against the authenticated user (S1: prevent subscribing to other users' streams).
func SSEHandler(hub *broadcast.Hub, sessionMgr *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subID := r.URL.Query().Get("client_id")
		if subID == "" {
			var buf [8]byte
			if _, err := rand.Read(buf[:]); err != nil {
				// 鏋佺鍥為€€锛歝rypto/rand 鍑犱箮涓嶄細澶辫触锛屾澶勪粎浣滈槻寰℃€у厹搴?				subID = fmt.Sprintf("anon-%d-%d", os.Getpid(), time.Now().UnixNano())
			} else {
				subID = "anon-" + hex.EncodeToString(buf[:])
			}
		}
		sessionID := r.URL.Query().Get("session_id")
		// P0-S5: 蹇呴』鏄惧紡鎸囧畾 session_id锛屽惁鍒欒闃呭埌鍏ㄧ珯浜嬩欢娴侊紙鍚叾浠栫敤鎴峰璇濆唴瀹癸級
		if sessionID == "" {
			BadRequest(w, "session_id is required")
			return
		}
		claims := auth.GetClaims(r.Context())
		if claims == nil {
			Unauthorized(w, ErrAuthRequired)
			return
		}
		if sessionMgr != nil {
			s, err := sessionMgr.GetSession(r.Context(), sessionID)
			if err != nil {
				// 鏂颁細璇濓細鍓嶇鍏堝缓绔?SSE 杩炴帴锛?submit 鎵嶄細鍒涘缓 session銆?				// 姝ゆ椂浼氳瘽灏氫笉瀛樺湪銆佹棤鍘嗗彶浜嬩欢鍙硠闇诧紝鏀捐杩炴帴绛夊緟鍒涘缓锛?				// 鍏朵粬閿欒锛圖B 鏁呴殰绛夛級鎷掔粷銆?				if !errors.Is(err, session.ErrSessionNotFound) {
					InternalError(w, "session check failed")
					return
				}
			} else if s == nil || s.UserID != claims.UserID {
				Forbidden(w, "session does not belong to the current user")
				return
			}
		}
		handleSSE(w, r, hub, subID, sessionID)
	}
}
