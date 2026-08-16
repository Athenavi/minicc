package api

import (
	"github.com/athenavi/minicc/internal/auth"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ── RPA 消息类型 ──

type RPAMessageType string

const (
	RPAMsgCommand RPAMessageType = "command"
	RPAMsgResult  RPAMessageType = "result"
	RPAMsgEvent   RPAMessageType = "event"
	RPAMsgAck     RPAMessageType = "ack"
)

// RPAMessage 是所有 RPA WebSocket 消息的统一 envelope
type RPAMessage struct {
	Type   RPAMessageType       `json:"type"`
	ID     string               `json:"id"`
	Method string               `json:"method,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
	Result map[string]interface{} `json:"result,omitempty"`
	Error  *RPAError            `json:"error,omitempty"`
	TabID  int                  `json:"tabId,omitempty"`
	TS     int64                `json:"ts"`
}

type RPAError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPAError) Error() string {
	return fmt.Sprintf("rpa error %d: %s", e.Code, e.Message)
}

// RPACommand 封装发送给插件的命令
type RPACommand struct {
	Method string
	Params map[string]interface{}
	TabID  int
}

// RPAResult 封装插件返回的结果
type RPAResult struct {
	Result map[string]interface{}
	Error  *RPAError
}

// ── RPA 客户端 ──

type RPAClient struct {
	ID       string
	Conn     *websocket.Conn
	UserID   string
	Tabs     []RPATabInfo
	LastSeen time.Time
	mu       sync.Mutex
}

type RPATabInfo struct {
	ID    int    `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func (c *RPAClient) TouchLastSeen() {
	c.mu.Lock()
	c.LastSeen = time.Now()
	c.mu.Unlock()
}

func (c *RPAClient) SendMessage(msg RPAMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg.TS = time.Now().UnixMilli()
	return c.Conn.WriteJSON(msg)
}

// ── RPAHub ──

type RPAHub struct {
	mu      sync.RWMutex
	clients map[string]*RPAClient       // clientID → client
	pending map[string]chan *RPAResult   // msgID → result channel
}

func NewRPAHub() *RPAHub {
	return &RPAHub{
		clients: make(map[string]*RPAClient),
		pending: make(map[string]chan *RPAResult),
	}
}

// Register 注册一个插件连接
func (h *RPAHub) Register(client *RPAClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client.ID] = client
	slog.Info("rpa client registered", "client_id", client.ID, "user_id", client.UserID)
}

// Unregister 注销一个插件连接
func (h *RPAHub) Unregister(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, clientID)
	slog.Info("rpa client unregistered", "client_id", clientID)
}

// GetClient 获取指定客户端
func (h *RPAHub) GetClient(clientID string) (*RPAClient, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.clients[clientID]
	return c, ok
}

// GetClientByUser 获取指定用户的最近活跃客户端
func (h *RPAHub) GetClientByUser(userID string) (*RPAClient, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var latest *RPAClient
	for _, c := range h.clients {
		if c.UserID == userID {
			if latest == nil || c.LastSeen.After(latest.LastSeen) {
				latest = c
			}
		}
	}
	return latest, latest != nil
}

// SendCommand 发送命令并等待结果（带超时）
func (h *RPAHub) SendCommand(ctx context.Context, clientID string, cmd *RPACommand) (*RPAResult, error) {
	client, ok := h.GetClient(clientID)
	if !ok {
		return nil, fmt.Errorf("rpa client not connected: %s", clientID)
	}

	msgID := fmt.Sprintf("cmd_%d", time.Now().UnixNano())
	msg := RPAMessage{
		Type:   RPAMsgCommand,
		ID:     msgID,
		Method: cmd.Method,
		Params: cmd.Params,
		TabID:  cmd.TabID,
	}

	// 注册 pending channel
	ch := make(chan *RPAResult, 1)
	h.mu.Lock()
	h.pending[msgID] = ch
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, msgID)
		h.mu.Unlock()
	}()

	// 发送命令
	if err := client.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	// 等待结果
	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("rpa command timeout: %s", cmd.Method)
	}
}

// HandleResult 处理从插件返回的结果
func (h *RPAHub) HandleResult(msg *RPAMessage) {
	h.mu.RLock()
	ch, ok := h.pending[msg.ID]
	h.mu.RUnlock()

	if !ok {
		slog.Warn("rpa result for unknown msg", "id", msg.ID)
		return
	}

	result := &RPAResult{Result: msg.Result}
	if msg.Error != nil {
		result.Error = msg.Error
	}
	ch <- result
}

// BroadcastToUser 向指定用户的所有客户端广播事件
func (h *RPAHub) BroadcastToUser(userID string, msg RPAMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		if c.UserID == userID {
			if err := c.SendMessage(msg); err != nil {
				slog.Warn("rpa broadcast failed", "client_id", c.ID, "error", err)
			}
		}
	}
}

// ConnectedClients 返回已连接的客户端列表
func (h *RPAHub) ConnectedClients() []*RPAClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	clients := make([]*RPAClient, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	return clients
}

// ExecCommand sends a command to a connected browser extension and returns the result map.
// This implements tools.RPABrowserHub interface, breaking the import cycle.
func (h *RPAHub) ExecCommand(ctx context.Context, clientID string, method string, params map[string]interface{}) (map[string]interface{}, error) {
	cmd := &RPACommand{Method: method, Params: params}
	result, err := h.SendCommand(ctx, clientID, cmd)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("rpa error %d: %s", result.Error.Code, result.Error.Message)
	}
	return result.Result, nil
}

// ConnectedClientIDs returns the IDs of all connected RPA clients.
// This implements tools.RPABrowserHub interface.
func (h *RPAHub) ConnectedClientIDs() []string {
	clients := h.ConnectedClients()
	ids := make([]string, len(clients))
	for i, c := range clients {
		ids[i] = c.ID
	}
	return ids
}

// ── RPA WebSocket Handler ──

var rpaUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // 插件跨域连接，由 JWT 认证控制安全
	},
}

const (
	rpaReadTimeout  = 60 * time.Second
	rpaWriteTimeout = 10 * time.Second
	rpaPingInterval = 30 * time.Second
)

// RPAWebSocketHandler 处理 RPA 插件的 WebSocket 连接
func RPAWebSocketHandler(hub *RPAHub, authenticator *auth.Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 验证 JWT token
		token := r.URL.Query().Get("token")
		clientID := r.URL.Query().Get("client_id")
		if clientID == "" {
			http.Error(w, "client_id required", http.StatusBadRequest)
			return
		}

		claims, err := authenticator.ValidateToken(token)
		if err != nil || claims == nil {
			http.Error(w, "invalid or missing token", http.StatusUnauthorized)
			return
		}
		userID := claims.UserID

		conn, err := rpaUpgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Warn("rpa ws upgrade", "error", err)
			return
		}

		client := &RPAClient{
			ID:       clientID,
			Conn:     conn,
			UserID:   userID,
			LastSeen: time.Now(),
		}
		hub.Register(client)

		// 发送连接确认
		client.SendMessage(RPAMessage{
			Type: RPAMsgAck,
			ID:   "init",
			Result: map[string]interface{}{
				"status":    "connected",
				"client_id": clientID,
			},
		})

		// 设置读写超时
		conn.SetReadDeadline(time.Now().Add(rpaReadTimeout))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(rpaReadTimeout))
			client.TouchLastSeen()
			return nil
		})

		// 心跳 goroutine
		done := make(chan struct{})
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("rpa heartbeat panic", "panic", r)
				}
			}()
			ticker := time.NewTicker(rpaPingInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(rpaWriteTimeout)); err != nil {
						return
					}
				case <-done:
					return
				}
			}
		}()

		// 消息读取循环
		defer func() {
			close(done)
			hub.Unregister(clientID)
			conn.Close()
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					slog.Warn("rpa ws read error", "error", err)
				}
				break
			}

			var msg RPAMessage
			if err := json.Unmarshal(message, &msg); err != nil {
				slog.Warn("rpa ws unmarshal", "error", err)
				continue
			}

			client.TouchLastSeen()

			switch msg.Type {
			case RPAMsgResult:
				hub.HandleResult(&msg)
			case RPAMsgEvent:
				handleRPAEvent(hub, client, &msg)
			default:
				slog.Debug("rpa ws unknown msg type", "type", msg.Type)
			}
		}
	}
}

// handleRPAEvent 处理插件主动推送的事件
func handleRPAEvent(hub *RPAHub, client *RPAClient, msg *RPAMessage) {
	switch msg.Method {
	case "tab_updated", "tab_created", "tab_closed":
		slog.Info("rpa tab event", "client_id", client.ID, "event", msg.Method, "tab_id", msg.TabID)
	case "init":
		if tabs, ok := msg.Params["tabs"].([]interface{}); ok {
			client.Tabs = make([]RPATabInfo, 0, len(tabs))
			for _, t := range tabs {
				if tab, ok := t.(map[string]interface{}); ok {
					info := RPATabInfo{}
					if id, ok := tab["id"].(float64); ok {
						info.ID = int(id)
					}
					if url, ok := tab["url"].(string); ok {
						info.URL = url
					}
					if title, ok := tab["title"].(string); ok {
						info.Title = title
					}
					client.Tabs = append(client.Tabs, info)
				}
			}
		}
		slog.Info("rpa client init", "client_id", client.ID, "tabs", len(client.Tabs))
	default:
		slog.Debug("rpa unknown event", "method", msg.Method)
	}
}
