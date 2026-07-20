package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow requests with no Origin header (direct curl/ws clients)
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// Allow configured CORS origins from environment
		allowed := os.Getenv("CORS_ORIGINS")
		if allowed == "" {
			return true // no restriction configured
		}
		for _, o := range strings.Split(allowed, ",") {
			if strings.TrimSpace(o) == origin {
				return true
			}
		}
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
func WebSocketHandler(hub *WebSocketHub, eventHub *broadcast.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("sessionId")
		if sessionID == "" {
			sessionID = r.URL.Query().Get("session_id")
		}
		if sessionID == "" {
			http.Error(w, "sessionId required", http.StatusBadRequest)
			return
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
