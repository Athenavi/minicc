package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebFetchTool fetches a URL and returns its content.
type WebFetchTool struct {
	client *http.Client
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		client: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

func (t *WebFetchTool) Name() string        { return "web_fetch" }
func (t *WebFetchTool) Description() string  { return "Fetch a URL and return its content as text." }

func (t *WebFetchTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	urlStr, _ := input["url"].(string)
	if urlStr == "" {
		return nil, fmt.Errorf("url is required")
	}

	// Basic URL validation
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return nil, fmt.Errorf("url must start with http:// or https://")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "MiniCC/2.0")
	req.Header.Set("Accept", "text/html,application/json,*/*")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	// Read response body (limit to 10MB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	return map[string]interface{}{
		"content":      string(body),
		"content_type": contentType,
		"status_code":  resp.StatusCode,
		"url":          urlStr,
		"size":         len(body),
	}, nil
}

// RegisterWebTools registers web-related tools.
func RegisterWebTools(registry *ToolRegistry) {
	registry.Register(NewWebFetchTool())
}
