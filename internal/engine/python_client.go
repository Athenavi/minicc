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

	"github.com/athenavi/chiron/internal/auth"
)

// PythonClient calls the Python AI engine via HTTP SSE.
// Supports multiple addresses with round-robin load balancing.
type PythonClient struct {
	addresses     []string
	counter       uint64
	client        *http.Client
	internalToken string // Go鈫擯ython 鍏变韩鍐呴儴 token锛岀敤浜庣綉鍏充唬鐞嗚韩浠芥牎楠?
	// 鐔旀柇锛氭瘡涓湴鍧€鐨勫喎鍗存埅姝㈡椂闂达紙Unix 绉掞級锛? = 姝ｅ父
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

// SetInternalToken 閰嶇疆 Go鈫擯ython 鍏变韩鍐呴儴 token銆?// 杞彂鍒?Python 鐨勮姹備細鑷姩娉ㄥ叆 X-Internal-Token header锛?// Python 渚ф嵁姝ゆ牎楠??tenant_id= 閫忎紶韬唤鐨勫悎娉曟€э紙P0-3 闃蹭吉閫狅級銆?func (c *PythonClient) SetInternalToken(token string) {
	c.internalToken = token
}

// injectInternalToken 鎶?X-Internal-Token header 娉ㄥ叆鍒板嚭绔欒姹傘€?// 鏈厤缃?token 鏃朵负 no-op锛堥儴缃蹭晶鏈惎鐢ㄥ唴閮ㄤ簰淇℃椂闄嶇骇锛屼絾 Python 渚т細
// fail-close 鎷掔粷 query 閫忎紶韬唤锛屽己鍒惰蛋 JWT/API Key 閴存潈锛夈€?func (c *PythonClient) injectInternalToken(req *http.Request) {
	if c.internalToken != "" {
		req.Header.Set("X-Internal-Token", c.internalToken)
	}
}

// pythonCooldown 鍗曚釜鍦板潃澶辫触鍚庣殑鍐峰嵈鏃堕暱锛氭殏鏃惰烦杩囷紝閬垮厤姣?N 涓姹傚繀璐ヤ竴涓?const pythonCooldown = 5 * time.Second

// markFailure 璁板綍鍦板潃澶辫触锛岃繘鍏ュ喎鍗?func (c *PythonClient) markFailure(addr string) {
	until := time.Now().Add(pythonCooldown).Unix()
	for i, a := range c.addresses {
		if a == addr {
			atomic.StoreInt64(&c.cooldownUntil[i], until)
			return
		}
	}
}

// markSuccess 娓呴櫎鍦板潃鍐峰嵈
func (c *PythonClient) markSuccess(addr string) {
	for i, a := range c.addresses {
		if a == addr {
			atomic.StoreInt64(&c.cooldownUntil[i], 0)
			return
		}
	}
}

// do 缁熶竴璇锋眰鍑哄彛锛氳褰曟垚鍔?澶辫触骞舵洿鏂扮啍鏂姸鎬?func (c *PythonClient) do(req *http.Request) (*http.Response, error) {
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
	// 鍏ㄩ儴鍦板潃鍐峰嵈涓細閫€鍖栦负 round-robin
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
		// S 淇锛歴ender 鍦ㄥ彂閫佸墠妫€鏌?ctx.Done锛岄伩鍏嶅灞傞€€鍑哄悗闃诲鍙戦€佽€屾硠婕?goroutine銆?		go func() {
			defer close(lineCh)
			for scanner.Scan() {
				select {
				case lineCh <- scanner.Text():
				case <-ctx.Done():
					return
				}
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
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.pickAddress()+"/healthz", nil)
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
	// 瀹夊叏锛氫粎杞彂蹇呰鐨勫鎴风澶达紝鎺掗櫎璁よ瘉/浼氳瘽/韬唤鐩稿叧澶?	// 闃叉瀹㈡埛绔€氳繃浼€?Authorization/Cookie/X-API-Key 缁曡繃缃戝叧璁よ瘉閾捐矾锛?	// 涔熼槻姝吉閫?X-User-ID/X-Tenant-ID/X-Internal-Token 鍐掔敤浠栦汉/浠栫鎴疯韩浠斤紙P0锛夈€?	skipHeaders := map[string]bool{
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
	// 娉ㄥ叆 Go鈫擯ython 鍐呴儴 token + 浠庡凡楠岃瘉鐨?JWT claims 鍙俊娉ㄥ叆韬唤澶?	// 锛圥ython 寮曟搸淇′换杩欎簺澶达紝鏁呭繀椤荤敱缃戝叧瑕嗙洊锛岀姝㈠鎴风鐩翠紶锛夈€?	c.injectInternalToken(req)
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
	// 瀹夊叏锛氳繃婊ゅ搷搴斾腑鐨?Set-Cookie锛岄槻姝㈠鎴风 Cookie 琚剰澶栬缃?	resp.Header.Del("Set-Cookie")
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
	// 鍙€夐檮鍔?header锛堝缃戝叧娉ㄥ叆鐨勭敤鎴疯韩浠?X-User-ID锛屼緵 Python 寮曟搸淇′换锛?	for _, h := range extraHeaders {
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
		// S 淇锛歴ender 鍦ㄥ彂閫佸墠妫€鏌?ctx.Done锛岄伩鍏嶅灞傞€€鍑哄悗闃诲鍙戦€佽€屾硠婕?goroutine銆?		go func() {
			defer close(lineCh)
			for scanner.Scan() {
				select {
				case lineCh <- scanner.Text():
				case <-ctx.Done():
					return
				}
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
