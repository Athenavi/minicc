package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/gorilla/websocket"
)

// 鈹€鈹€ RPA 娑堟伅绫诲瀷 鈹€鈹€

type RPAMessageType string

const (
	RPAMsgCommand RPAMessageType = "command"
	RPAMsgResult  RPAMessageType = "result"
	RPAMsgEvent   RPAMessageType = "event"
	RPAMsgAck     RPAMessageType = "ack"
)

// RPAMessage 鏄墍鏈?RPA WebSocket 娑堟伅鐨勭粺涓€ envelope
type RPAMessage struct {
	Type   RPAMessageType         `json:"type"`
	ID     string                 `json:"id"`
	Method string                 `json:"method,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
	Result map[string]interface{} `json:"result,omitempty"`
	Error  *RPAError              `json:"error,omitempty"`
	TabID  int                    `json:"tabId,omitempty"`
	TS     int64                  `json:"ts"`
}

type RPAError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPAError) Error() string {
	return fmt.Sprintf("rpa error %d: %s", e.Code, e.Message)
}

// RPACommand 灏佽鍙戦€佺粰鎻掍欢鐨勫懡浠?type RPACommand struct {
	Method string
	Params map[string]interface{}
	TabID  int
}

// RPAResult 灏佽鎻掍欢杩斿洖鐨勭粨鏋?type RPAResult struct {
	Result map[string]interface{}
	Error  *RPAError
}

// 鈹€鈹€ RPA 瀹㈡埛绔?鈹€鈹€

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
	// P1 淇锛氬啓瓒呮椂锛岄槻姝㈡瀹㈡埛绔棤闄愰樆濉炲箍鎾?goroutine
	_ = c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.Conn.WriteJSON(msg)
}

// 鈹€鈹€ RPAHub 鈹€鈹€

type RPAHub struct {
	mu      sync.RWMutex
	clients map[string]*RPAClient      // clientID 鈫?client
	pending map[string]chan *RPAResult // msgID 鈫?result channel
}

func NewRPAHub() *RPAHub {
	return &RPAHub{
		clients: make(map[string]*RPAClient),
		pending: make(map[string]chan *RPAResult),
	}
}

// Register 娉ㄥ唽涓€涓彃浠惰繛鎺?func (h *RPAHub) Register(client *RPAClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client.ID] = client
	slog.Info("rpa client registered", "client_id", client.ID, "user_id", client.UserID)
}

// Unregister 娉ㄩ攢涓€涓彃浠惰繛鎺?func (h *RPAHub) Unregister(clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, clientID)
	slog.Info("rpa client unregistered", "client_id", clientID)
}

// GetClient 鑾峰彇鎸囧畾瀹㈡埛绔?func (h *RPAHub) GetClient(clientID string) (*RPAClient, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	c, ok := h.clients[clientID]
	return c, ok
}

// GetClientByUser 鑾峰彇鎸囧畾鐢ㄦ埛鐨勬渶杩戞椿璺冨鎴风
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

// SendCommand 鍙戦€佸懡浠ゅ苟绛夊緟缁撴灉锛堝甫瓒呮椂锛?func (h *RPAHub) SendCommand(ctx context.Context, clientID string, cmd *RPACommand) (*RPAResult, error) {
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

	// 娉ㄥ唽 pending channel
	ch := make(chan *RPAResult, 1)
	h.mu.Lock()
	h.pending[msgID] = ch
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, msgID)
		h.mu.Unlock()
	}()

	// 鍙戦€佸懡浠?	if err := client.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	// 绛夊緟缁撴灉
	select {
	case result := <-ch:
		return result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("rpa command timeout: %s", cmd.Method)
	}
}

// HandleResult 澶勭悊浠庢彃浠惰繑鍥炵殑缁撴灉
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

// BroadcastToUser 鍚戞寚瀹氱敤鎴风殑鎵€鏈夊鎴风骞挎挱浜嬩欢
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

// ConnectedClients 杩斿洖宸茶繛鎺ョ殑瀹㈡埛绔垪琛?func (h *RPAHub) ConnectedClients() []*RPAClient {
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

// 鈹€鈹€ RPA WebSocket Handler 鈹€鈹€

var rpaUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // 鏈嶅姟绔?ws 瀹㈡埛绔棤 Origin
		}
		allowed := os.Getenv("CORS_ORIGINS")
		if allowed == "" || allowed == "*" {
			slog.Warn("rpa websocket origin rejected: CORS_ORIGINS not configured",
				"origin", origin, "path", r.URL.Path)
			return false
		}
		for _, o := range strings.Split(allowed, ",") {
			if strings.TrimSpace(o) == origin {
				return true
			}
		}
		slog.Warn("rpa websocket origin rejected: not in allowlist",
			"origin", origin, "path", r.URL.Path)
		return false
	},
}

const (
	rpaReadTimeout  = 60 * time.Second
	rpaWriteTimeout = 10 * time.Second
	rpaPingInterval = 30 * time.Second
)

// RPAWebSocketHandler 澶勭悊 RPA 鎻掍欢鐨?WebSocket 杩炴帴
func RPAWebSocketHandler(hub *RPAHub, authenticator *auth.Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 楠岃瘉 JWT token
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

		// 鍙戦€佽繛鎺ョ‘璁?		client.SendMessage(RPAMessage{
			Type: RPAMsgAck,
			ID:   "init",
			Result: map[string]interface{}{
				"status":    "connected",
				"client_id": clientID,
			},
		})

		// 璁剧疆璇诲啓瓒呮椂
		conn.SetReadDeadline(time.Now().Add(rpaReadTimeout))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(rpaReadTimeout))
			client.TouchLastSeen()
			return nil
		})

		// 蹇冭烦 goroutine
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

		// 娑堟伅璇诲彇寰幆
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

// 鈹€鈹€ RPA HTTP Bridge锛圥ython engine 鈫?Go gateway 鈫?娴忚鍣ㄦ彃浠讹級 鈹€鈹€

// rpaInternalTokenOK 甯搁噺鏃堕棿姣旇緝 X-Internal-Token锛堢綉鍏斥啍寮曟搸浜掍俊锛夈€?func rpaInternalTokenOK(r *http.Request, internalToken string) bool {
	if internalToken == "" {
		return false
	}
	provided := r.Header.Get("X-Internal-Token")
	return provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(internalToken)) == 1
}

// RPAExecHandler 渚?Python engine 鐨?GatewayBrowserHub 鎶婃祻瑙堝櫒鍛戒护鍙戠粰
// 宸茶繛鎺ユ彃浠?Chrome Extension /ws/rpa)銆傝姹傚叡浜?internal token锛岄槻姝㈢洿杩炴互鐢ㄣ€?func RPAExecHandler(hub *RPAHub, internalToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rpaInternalTokenOK(r, internalToken) {
			Unauthorized(w, "invalid internal token")
			return
		}
		var req struct {
			ClientID string                 `json:"client_id"`
			Method   string                 `json:"method"`
			Params   map[string]interface{} `json:"params"`
		}
		if err := DecodeJSON(w, r, &req); err != nil || req.Method == "" || req.ClientID == "" {
			BadRequest(w, "client_id and method are required")
			return
		}
		result, err := hub.ExecCommand(r.Context(), req.ClientID, req.Method, req.Params)
		if err != nil {
			JSON(w, http.StatusBadGateway, APIResponse{Success: false, Error: err.Error()})
			return
		}
		OK(w, result)
	}
}

// RPAClientsHandler 杩斿洖宸茶繛鎺ユ祻瑙堝櫒鎻掍欢瀹㈡埛绔垪琛ㄣ€?func RPAClientsHandler(hub *RPAHub, internalToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rpaInternalTokenOK(r, internalToken) {
			Unauthorized(w, "invalid internal token")
			return
		}
		OK(w, map[string]interface{}{"client_ids": hub.ConnectedClientIDs()})
	}
}

// handleRPAEvent 澶勭悊鎻掍欢涓诲姩鎺ㄩ€佺殑浜嬩欢
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
