package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/athenavi/minicc/internal/auth"
)

// PythonClient calls the Python AI engine via HTTP SSE.
// Supports multiple addresses with round-robin load balancing.
type PythonClient struct {
	addresses     []string
	counter       uint64
	client        *http.Client
	internalToken string // Go↔Python 共享内部 token，用于网关代理身份校验

	// 熔断：每个地址的冷却截止时间（Unix 秒），0 = 正常
	cooldownUntil []int64}

// NewPythonClient creates a client for the Python engine HTTP API.
// Accepts one or more base URLs (comma-separated or variadic).
// Requests are distributed across addresses using round-robin.
func NewPythonClient(addresses ...string) *PythonClient {
	addrs := make([]string, 0, len(addresses))
	for _, a := range addresses {
		a = strings.TrimSpace(a)
		if a != "" {
			addrs = append(addrs, a)
		}
	}
	if len(addrs) == 0 {
		addrs = []string{"http://localhost:8000"}
	}

	// Configure transport with sensible timeouts to prevent resource leaks
	transport := &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	}

	return &PythonClient{
		addresses: addrs,
		client: &http.Client{
			Timeout:   0, // No overall timeout; individual requests use their own context
			Transport: transport,
		},
		cooldownUntil: make([]int64, len(addrs)),
	}
}

// SetInternalToken 配置 Go↔Python 共享内部 token。
// 转发到 Python 的请求会自动注入 X-Internal-Token header，
// Python 侧据此校验 ?tenant_id= 透传身份的合法性（P0-3 防伪造）。
func (c *PythonClient) SetInternalToken(token string) {
	c.internalToken = token
}

// injectInternalToken 把 X-Internal-Token header 注入到出站请求。
// 未配置 token 时为 no-op（部署侧未启用内部互信时降级，但 Python 侧会
// fail-close 拒绝 query 透传身份，强制走 JWT/API Key 鉴权）。
func (c *PythonClient) injectInternalToken(req *http.Request) {
	if c.internalToken != "" {
		req.Header.Set("X-Internal-Token", c.internalToken)
	}
}

// pythonCooldown 单个地址失败后的冷却时长：暂时跳过，避免每 N 个请求必败一个
const pythonCooldown = 5 * time.Second

// markFailure 记录地址失败，进入冷却
func (c *PythonClient) markFailure(addr string) {
	until := time.Now().Add(pythonCooldown).Unix()
	for i, a := range c.addresses {
		if a == addr {
			atomic.StoreInt64(&c.cooldownUntil[i], until)
			return
		}
	}
}

// markSuccess 清除地址冷却
func (c *PythonClient) markSuccess(addr string) {
	for i, a := range c.addresses {
		if a == addr {
			atomic.StoreInt64(&c.cooldownUntil[i], 0)
			return
		}
	}
}

// do 统一请求出口：记录成功/失败并更新熔断状态
func (c *PythonClient) do(req *http.Request) (*http.Response, error) {
	addr := req.URL.Scheme + "://" + req.URL.Host
	resp, err := c.client.Do(req)
	if err != nil {
		c.markFailure(addr)
		return nil, err
	}
	c.markSuccess(addr)
	return resp, nil
}

// pickAddress returns the next address using round-robin.
func (c *PythonClient) pickAddress() string {
	if len(c.addresses) == 0 {
		return "http://localhost:8000"
	}
	now := time.Now().Unix()
	start := int(atomic.AddUint64(&c.counter, 1)) % len(c.addresses)
	for i := 0; i < len(c.addresses); i++ {
		idx := (start + i) % len(c.addresses)
		if atomic.LoadInt64(&c.cooldownUntil[idx]) <= now {
			return c.addresses[idx]
		}
	}
	// 全部地址冷却中：退化为 round-robin
	return c.addresses[start]
}

// PythonRunRequest matches the Python engine's Pydantic RunRequest model.
type PythonRunRequest struct {
	SessionID    string            `json:"session_id"`
	UserID       string            `json:"user_id"`
	Content      string            `json:"content"`
	SystemPrompt string            `json:"system_prompt"`
	History      []PythonMessage   `json:"history"`
	Tools        []PythonToolDef   `json:"tools"`
	LLMConfig    *PythonLLMConfig  `json:"llm_config,omitempty"`
	MaxTurns     int               `json:"max_turns"`
}

// PythonMessage is a message in the conversation history.
type PythonMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PythonToolDef describes a tool available to the agent.
type PythonToolDef struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	ParametersJSON string `json:"parameters_json"`
}

// PythonLLMConfig configures the LLM for this inference call.
type PythonLLMConfig struct {
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
}

// PythonEvent is a single SSE event from the Python engine.
type PythonEvent struct {
	Type         string `json:"type"`
	Content      string `json:"content,omitempty"`
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Arguments    string `json:"arguments,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Message      string `json:"message,omitempty"`
}

// Run starts a streaming inference call to the Python engine.
func (c *PythonClient) Run(ctx context.Context, req PythonRunRequest) (<-chan PythonEvent, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.pickAddress()+"/v1/agent/run", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	c.injectInternalToken(httpReq)

	resp, err := c.do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call python engine: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("python engine returned status %d", resp.StatusCode)
	}

	events := make(chan PythonEvent, 64)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("python sse read panic", "panic", r)
			}
		}()
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

		// Use a ticker to periodically check context cancellation
		// This prevents the goroutine from blocking forever on scanner.Scan()
		// if the context is cancelled but no data is being sent.
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lineCh := make(chan string, 1)

		// Background goroutine to read lines
		go func() {
			defer close(lineCh)
			for scanner.Scan() {
				lineCh <- scanner.Text()
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-lineCh:
				if !ok {
					// Scanner finished (EOF or error)
					if err := scanner.Err(); err != nil && err != io.EOF {
						slog.Warn("python engine: read stream", "error", err)
					}
					return
				}
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := strings.TrimPrefix(line, "data: ")
				var event PythonEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					slog.Warn("python engine: unmarshal event", "error", err, "data", data)
					continue
				}
				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
			case <-ticker.C:
				// Periodic check, if context is done, we'll catch it in the first case
			}
		}
	}()

	return events, nil
}

// IsConnected checks if any Python engine instance is reachable.
func (c *PythonClient) IsConnected() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", c.pickAddress()+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := c.do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetJSON performs a GET request and decodes JSON into out.
func (c *PythonClient) GetJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.pickAddress()+path, nil)
	if err != nil {
		return err
	}
	c.injectInternalToken(req)
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("python GET %s returned %d: %s", path, resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// PostJSON performs a POST request with JSON body and decodes JSON into out.
func (c *PythonClient) PostJSON(ctx context.Context, path string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.pickAddress()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.injectInternalToken(req)
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("python POST %s returned %d: %s", path, resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// PutJSON performs a PUT request with JSON body and decodes JSON into out.
func (c *PythonClient) PutJSON(ctx context.Context, path string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.pickAddress()+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.injectInternalToken(req)
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("python PUT %s returned %d: %s", path, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// ForwardRequest forwards an incoming HTTP request to the Python engine,
// preserving method, headers, and body. The response status and body are
// written directly to w.
func (c *PythonClient) ForwardRequest(w http.ResponseWriter, r *http.Request, path string) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, c.pickAddress()+path, r.Body)
	if err != nil {
		slog.Error("create forward request", "path", path, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// 安全：仅转发必要的客户端头，排除认证/会话/身份相关头
	// 防止客户端通过伪造 Authorization/Cookie/X-API-Key 绕过网关认证链路，
	// 也防止伪造 X-User-ID/X-Tenant-ID/X-Internal-Token 冒用他人/他租户身份（P0）。
	skipHeaders := map[string]bool{
		"Authorization":       true,
		"Proxy-Authorization": true,
		"Cookie":              true,
		"Set-Cookie":          true,
		"X-Api-Key":           true,
		"X-Auth-Token":        true,
		"X-User-Id":           true,
		"X-User-Role":         true,
		"X-Tenant-Id":         true,
		"X-Internal-Token":    true,
	}
	for k, vv := range r.Header {
		if skipHeaders[k] {
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	// 注入 Go↔Python 内部 token + 从已验证的 JWT claims 可信注入身份头
	// （Python 引擎信任这些头，故必须由网关覆盖，禁止客户端直传）。
	c.injectInternalToken(req)
	if claims := auth.GetClaims(r.Context()); claims != nil {
		if claims.UserID != "" {
			req.Header.Set("X-User-Id", claims.UserID)
		}
		if claims.TenantID != "" {
			req.Header.Set("X-Tenant-Id", claims.TenantID)
		}
	}
	resp, err := c.do(req)
	if err != nil {
		slog.Error("forward to python engine", "path", path, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	// 安全：过滤响应中的 Set-Cookie，防止客户端 Cookie 被意外设置
	resp.Header.Del("Set-Cookie")
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// DeleteJSON performs a DELETE request and decodes JSON into out.
func (c *PythonClient) DeleteJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.pickAddress()+path, nil)
	if err != nil {
		return err
	}
	c.injectInternalToken(req)
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("python DELETE %s returned %d: %s", path, resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// RunSSE starts a streaming SSE call to any Python endpoint and returns events.
func (c *PythonClient) RunSSE(ctx context.Context, path string, body any, extraHeaders ...map[string]string) (<-chan PythonEvent, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.pickAddress()+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	c.injectInternalToken(httpReq)
	// 可选附加 header（如网关注入的用户身份 X-User-ID，供 Python 引擎信任）
	for _, h := range extraHeaders {
		for k, v := range h {
			httpReq.Header.Set(k, v)
		}
	}

	resp, err := c.do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call python engine: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("python engine returned status %d", resp.StatusCode)
	}

	events := make(chan PythonEvent, 64)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("python sse read panic", "panic", r)
			}
		}()
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)

		// Use a ticker to periodically check context cancellation
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		lineCh := make(chan string, 1)

		// Background goroutine to read lines
		go func() {
			defer close(lineCh)
			for scanner.Scan() {
				lineCh <- scanner.Text()
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case line, ok := <-lineCh:
				if !ok {
					if err := scanner.Err(); err != nil && err != io.EOF {
						slog.Warn("python engine: read stream", "error", err)
					}
					return
				}
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				raw := strings.TrimPrefix(line, "data: ")
				var event PythonEvent
				if err := json.Unmarshal([]byte(raw), &event); err != nil {
					slog.Warn("python engine: unmarshal event", "error", err, "data", raw)
					continue
				}
				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
			case <-ticker.C:
				// Periodic check
			}
		}
	}()

	return events, nil
}
