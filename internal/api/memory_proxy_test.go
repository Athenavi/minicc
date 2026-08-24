package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/broadcast"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
	"github.com/athenavi/minicc/internal/session"
)

// testRouterWithPython 构建注入指定 Python 客户端的网关路由器。
func testRouterWithPython(t *testing.T, pyClient *engine.PythonClient) http.Handler {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	os.Setenv("APP_SECRET", "test-app-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()
	eventHub := broadcast.NewHub(nil)
	sessionMgr := session.NewManager(nil, nil)
	return NewGatewayRouter(cfg, pyClient, eventHub, sessionMgr, nil, nil, nil)
}

// TestMemoryProxy_RoutesExist 验证所有记忆路由都已正确注册。
func TestMemoryProxy_RoutesExist(t *testing.T) {
	router := testRouter(t)

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/memory/profile"},
		{"POST", "/v1/memory/profile"},
		{"PUT", "/v1/memory/profile"},
		{"DELETE", "/v1/memory/profile/test-id"},
		{"POST", "/v1/memory/profile/clear"},
		{"POST", "/v1/memory/search"},
		{"POST", "/v1/memory/organize"},
		{"GET", "/v1/memory/organize/status"},
		{"GET", "/v1/memory/summaries"},
		{"GET", "/v1/memory/conflicts"},
		{"POST", "/v1/memory/conflicts/test-id/resolve"},
		{"DELETE", "/v1/memory/conflicts/test-id"},
	}

	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// 未认证请求应返回 401（由 authMW 保护）
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401 (auth required), got %d", r.method, r.path, w.Code)
			}
		})
	}
}

// TestMemoryProxy_AuthRequired 验证记忆路由需要认证。
func TestMemoryProxy_AuthRequired(t *testing.T) {
	router := testRouter(t)

	paths := []struct {
		method string
		path   string
	}{
		{"GET", "/v1/memory/profile"},
		{"GET", "/v1/memory/conflicts"},
		{"GET", "/v1/memory/summaries"},
		{"POST", "/v1/memory/search"}, // search only supports POST
	}

	for _, p := range paths {
		t.Run(p.method+" "+p.path, func(t *testing.T) {
			req := httptest.NewRequest(p.method, p.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401, got %d", p.method, p.path, w.Code)
			}
		})
	}
}

// TestMemoryProxy_ProxyForwardsCorrectly 验证代理请求正确转发到 Python 引擎。
func TestMemoryProxy_ProxyForwardsCorrectly(t *testing.T) {
	// 启动一个模拟 Python 引擎的 HTTP 服务器
	pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证查询参数中包含 user_id 和 tenant_id
		query := r.URL.Query()
		if query.Get("user_id") == "" {
			t.Error("missing user_id in proxied request")
		}
		if query.Get("tenant_id") == "" {
			t.Error("missing tenant_id in proxied request")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer pythonServer.Close()

	pyClient := engine.NewPythonClient(pythonServer.URL)
	router := testRouterWithPython(t, pyClient)

	// 生成认证 token
	token := testToken(t)

	t.Run("GET /v1/memory/profile", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/memory/profile", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("GET /v1/memory/conflicts", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/memory/conflicts", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

// TestMemoryProxy_PathParameters 验证路径参数路由正确转发。
func TestMemoryProxy_PathParameters(t *testing.T) {
	var receivedPath string
	pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true}`))
	}))
	defer pythonServer.Close()

	pyClient := engine.NewPythonClient(pythonServer.URL)
	router := testRouterWithPython(t, pyClient)
	token := testToken(t)

	// 测试 DELETE /v1/memory/profile/{id}
	t.Run("DELETE profile with id", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/memory/profile/item-123", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		// 验证路径参数被正确传递
		if receivedPath != "/v1/memory/profile/item-123?user_id=test-user-id&tenant_id="+url.QueryEscape(db.DefaultTenantID) {
			t.Errorf("unexpected path: %s", receivedPath)
		}
	})

	// 测试 DELETE /v1/memory/conflicts/{conflict_id}
	t.Run("DELETE conflict with id", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/memory/conflicts/conflict-456", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if receivedPath != "/v1/memory/conflicts/conflict-456?user_id=test-user-id&tenant_id="+url.QueryEscape(db.DefaultTenantID) {
			t.Errorf("unexpected path: %s", receivedPath)
		}
	})

	// 测试 POST /v1/memory/conflicts/{conflict_id}/resolve
	t.Run("POST resolve conflict", func(t *testing.T) {
		// 注意：POST 请求必须提供合法 JSON body，否则 DecodeJSON 会因空 body 返回 io.EOF，
		// 代理处理逻辑提前 return，导致 Python 端从未收到请求，响应体为空。
		req := httptest.NewRequest("POST", "/v1/memory/conflicts/conflict-789/resolve",
			bytes.NewBufferString(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		if receivedPath != "/v1/memory/conflicts/conflict-789/resolve?user_id=test-user-id&tenant_id="+url.QueryEscape(db.DefaultTenantID) {
			t.Errorf("unexpected path: %s", receivedPath)
		}
	})
}

// TestMemoryProxy_MissingPythonEngine 验证 Python 引擎不可用时返回 503。
func TestMemoryProxy_MissingPythonEngine(t *testing.T) {
	// pyClient 指向不存在的地址
	pyClient := engine.NewPythonClient("http://localhost:9999")
	router := testRouterWithPython(t, pyClient)
	token := testToken(t)

	req := httptest.NewRequest("GET", "/v1/memory/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 代理请求会失败，因为 Python 引擎不可达
	// 根据实现，这可能返回 500 或 503
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusOK {
		t.Errorf("expected error when python engine unavailable, got %d", w.Code)
	}
}

// TestMemoryProxy_MissingTenantContext 验证缺少租户上下文时返回 401。
func TestMemoryProxy_MissingTenantContext(t *testing.T) {
	pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer pythonServer.Close()

	pyClient := engine.NewPythonClient(pythonServer.URL)
	router := testRouterWithPython(t, pyClient)

	// 使用自定义 claims，tenant_id 为空
	claims := &auth.Claims{
		UserID:  "test-user",
		Role:    "user",
		TenantID: "", // 空租户 ID
	}
	req := httptest.NewRequest("GET", "/v1/memory/profile", nil)
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty tenant_id, got %d", w.Code)
	}
}
