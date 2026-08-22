package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/session"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// CheckOrigin 严格校验：CORS_ORIGINS 未配置则拒绝所有带 Origin 的浏览器请求，
	// 仅放行无 Origin 的非浏览器（curl/python websockets）客户端。
	// 生产部署必须显式配置 CORS_ORIGINS 为前端域名白名单。
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // curl / 服务端 ws 客户端无 Origin
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
	for _, sc := range conns {
		sc.mu.Lock()
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
			h.conns[sessionID] = append(conns[:i], conns[i+1:]...)
			break
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
// 连接前校验 JWT（?token= / cookie / Authorization）并验证 session 归属（S 安全修复：
// 原实现无认证，任意客户端可订阅任意 session 的事件流）。
func WebSocketHandler(hub *WebSocketHub, eventHub *broadcast.Hub, authenticator *auth.Authenticator, sessionMgr *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionId")
		if sessionID == "" {
			sessionID = r.URL.Query().Get("session_id")
		}
		if sessionID == "" {
			http.Error(w, "sessionId required", http.StatusBadRequest)
			return
		}

		// 认证：与 SSE/AuthMiddleware 同源（?token= 供 ws 客户端使用）
		tokenStr := r.URL.Query().Get("token")
		if tokenStr == "" {
			if c, err := r.Cookie("minicc_token"); err == nil && c.Value != "" {
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

		// 会话归属校验：仅允许访问自己的会话
		if sessionMgr != nil {
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
				defer eventHub.Unsubscribe(subID)
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
