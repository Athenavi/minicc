package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/broadcast"
	"github.com/athenavi/chiron/internal/session"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// CheckOrigin 涓ユ牸鏍￠獙锛欳ORS_ORIGINS 鏈厤缃垯鎷掔粷鎵€鏈夊甫 Origin 鐨勬祻瑙堝櫒璇锋眰锛?	// 浠呮斁琛屾棤 Origin 鐨勯潪娴忚鍣紙curl/python websockets锛夊鎴风銆?	// 鐢熶骇閮ㄧ讲蹇呴』鏄惧紡閰嶇疆 CORS_ORIGINS 涓哄墠绔煙鍚嶇櫧鍚嶅崟銆?	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // curl / 鏈嶅姟绔?ws 瀹㈡埛绔棤 Origin
		}
		allowed := os.Getenv("CORS_ORIGINS")
		if allowed == "" || allowed == "*" {
			slog.Warn("websocket origin rejected: CORS_ORIGINS not configured",
				"origin", origin, "path", r.URL.Path)
			return false
		}
		for _, o := range strings.Split(allowed, ",") {
			if strings.TrimSpace(o) == origin {
				return true
			}
		}
		slog.Warn("websocket origin rejected: not in allowlist",
			"origin", origin, "path", r.URL.Path)
		return false
	},
}

// safeConn wraps a websocket.Conn with a write mutex.
// gorilla/websocket does not support concurrent writes.
type safeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// WebSocketHub manages active WebSocket connections per session.
type WebSocketHub struct {
	mu    sync.RWMutex
	conns map[string][]*safeConn
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{conns: make(map[string][]*safeConn)}
}

// Broadcast sends a JSON message to all connections for a session.
func (h *WebSocketHub) Broadcast(sessionID string, msg interface{}) {
	h.mu.RLock()
	// Make a shallow copy under the lock so iteration is safe from concurrent
	// removeConn modifying the underlying array.
	conns := append([]*safeConn(nil), h.conns[sessionID]...)
	h.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("ws marshal", "error", err)
		return
	}
	// Use a short write deadline to prevent blocking on slow connections
	deadline := 5 * time.Second
	for _, sc := range conns {
		sc.mu.Lock()
		_ = sc.conn.SetWriteDeadline(time.Now().Add(deadline))
		err := sc.conn.WriteMessage(websocket.TextMessage, data)
		sc.mu.Unlock()
		if err != nil {
			slog.Warn("ws write", "error", err)
		}
	}
}

func (h *WebSocketHub) addConn(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[sessionID] = append(h.conns[sessionID], &safeConn{conn: conn})
}

func (h *WebSocketHub) removeConn(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.conns[sessionID]
	for i, sc := range conns {
		if sc.conn == conn {
			conns = append(conns[:i], conns[i+1:]...)
			if len(conns) == 0 {
				// Clean up empty session entry to prevent map bloat
				delete(h.conns, sessionID)
			} else {
				h.conns[sessionID] = conns
			}
			return
		}
	}
}

// connCount returns the number of active connections for a session.
func (h *WebSocketHub) connCount(sessionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns[sessionID])
}

// WebSocketHandler handles WebSocket upgrade and message loop.
// If eventHub is non-nil, messages are bridged through Redis Pub/Sub for cross-instance delivery.
// 杩炴帴鍓嶆牎楠?JWT锛?token= / cookie / Authorization锛夊苟楠岃瘉 session 褰掑睘锛圫 瀹夊叏淇锛?// 鍘熷疄鐜版棤璁よ瘉锛屼换鎰忓鎴风鍙闃呬换鎰?session 鐨勪簨浠舵祦锛夈€?func WebSocketHandler(hub *WebSocketHub, eventHub *broadcast.Hub, authenticator *auth.Authenticator, sessionMgr *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionId")
		if sessionID == "" {
			sessionID = r.URL.Query().Get("session_id")
		}
		if sessionID == "" {
			http.Error(w, "sessionId required", http.StatusBadRequest)
			return
		}

		// 璁よ瘉锛氫笌 SSE/AuthMiddleware 鍚屾簮锛?token= 渚?ws 瀹㈡埛绔娇鐢級
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			if c, err := r.Cookie("chiron_token"); err == nil && c.Value != "" {
				tokenStr = c.Value
			}
		}
		if tokenStr == "" {
			if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
				tokenStr = strings.TrimPrefix(ah, "Bearer ")
			}
		}
		claims, err := authenticator.ValidateToken(tokenStr)
		if err != nil || claims == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// 浼氳瘽褰掑睘鏍￠獙锛氫粎鍏佽璁块棶鑷繁鐨勪細璇?		if sessionMgr != nil {
			sess, err := sessionMgr.GetSession(r.Context(), sessionID)
			if err != nil || sess.UserID != claims.UserID {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Warn("ws upgrade", "error", err)
			return
		}

		hub.addConn(sessionID, conn)
		slog.Debug("ws connected", "session", sessionID)

		hub.Broadcast(sessionID, map[string]string{
			"type": "connected", "session_id": sessionID,
		})

		// Subscribe to broadcast.Hub for cross-instance events targeting this session
		var subCh chan broadcast.Event
		var subID string
		if eventHub != nil {
			subID = fmt.Sprintf("ws:%s:%p", sessionID, conn)
			subCh = eventHub.Subscribe(subID)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("ws event forward panic", "panic", r)
					}
				}()
				for evt := range subCh {
					// Only forward events for this session (or broadcast events with no session)
					if evt.SessionID != "" && evt.SessionID != sessionID {
						continue
					}
					hub.Broadcast(sessionID, evt)
				}
			}()
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("ws read panic", "panic", r)
				}
			}()
			defer func() {
				hub.removeConn(sessionID, conn)
				if eventHub != nil {
					eventHub.Unsubscribe(subID)
				}
				conn.Close()
				slog.Debug("ws disconnected", "session", sessionID)
			}()
			for {
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
				_, message, err := conn.ReadMessage()
				if err != nil {
					break
				}
				var msg map[string]interface{}
				if err := json.Unmarshal(message, &msg); err != nil {
					slog.Warn("ws unmarshal", "error", err)
					continue
				}
				msg["type"] = "echo"

				// Bridge through broadcast.Hub for cross-instance delivery
				if eventHub != nil {
					eventHub.Publish(broadcast.Event{
						Type:      "ws_message",
						Data:      msg,
						SessionID: sessionID,
					})
				} else {
					// Fallback: local-only broadcast
					hub.Broadcast(sessionID, msg)
				}
			}
		}()
	}
}
