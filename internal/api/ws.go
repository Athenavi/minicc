package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// WebSocketHub manages active WebSocket connections per session.
type WebSocketHub struct {
	mu    sync.RWMutex
	conns map[string][]*websocket.Conn
}

func NewWebSocketHub() *WebSocketHub {
	return &WebSocketHub{conns: make(map[string][]*websocket.Conn)}
}

// Broadcast sends a JSON message to all connections for a session.
func (h *WebSocketHub) Broadcast(sessionID string, msg interface{}) {
	h.mu.RLock()
	conns := h.conns[sessionID]
	h.mu.RUnlock()

	data, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("ws marshal", "error", err)
		return
	}
	for _, conn := range conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			slog.Warn("ws write", "error", err)
		}
	}
}

func (h *WebSocketHub) addConn(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[sessionID] = append(h.conns[sessionID], conn)
}

func (h *WebSocketHub) removeConn(sessionID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.conns[sessionID]
	for i, c := range conns {
		if c == conn {
			h.conns[sessionID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
}

// WebSocketHandler handles WebSocket upgrade and message loop.
func WebSocketHandler(hub *WebSocketHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := chi.URLParam(r, "sessionId")
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

		go func() {
			defer func() {
				hub.removeConn(sessionID, conn)
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
				hub.Broadcast(sessionID, msg)
			}
		}()
	}
}
