package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/broadcast"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/athenavi/chiron/internal/session"
)

// testRouterWithPython 鏋勫缓娉ㄥ叆鎸囧畾 Python 瀹㈡埛绔殑缃戝叧璺敱鍣ㄣ€?func testRouterWithPython(t *testing.T, pyClient *engine.PythonClient) http.Handler {
	t.Helper()
	os.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long!")
	os.Setenv("APP_SECRET", "test-app-secret-that-is-at-least-32-bytes-long!")
	cfg := config.Load()
	eventHub := broadcast.NewHub(nil)
	sessionMgr := session.NewManager(nil, nil)
	return NewGatewayRouter(cfg, pyClient, eventHub, sessionMgr, nil, nil, nil)
}

// TestMemoryProxy_RoutesExist 楠岃瘉鎵€鏈夎蹇嗚矾鐢遍兘宸叉纭敞鍐屻€?func TestMemoryProxy_RoutesExist(t *testing.T) {
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

			// 鏈璇佽姹傚簲杩斿洖 401锛堢敱 authMW 淇濇姢锛?			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: expected 401 (auth required), got %d", r.method, r.path, w.Code)
			}
		})
	}
}

// TestMemoryProxy_AuthRequired 楠岃瘉璁板繂璺敱闇€瑕佽璇併€?func TestMemoryProxy_AuthRequired(t *testing.T) {
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

// TestMemoryProxy_ProxyForwardsCorrectly 楠岃瘉浠ｇ悊璇锋眰姝ｇ‘杞彂鍒?Python 寮曟搸銆?func TestMemoryProxy_ProxyForwardsCorrectly(t *testing.T) {
	// 鍚姩涓€涓ā鎷?Python 寮曟搸鐨?HTTP 鏈嶅姟鍣?	pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 楠岃瘉鏌ヨ鍙傛暟涓寘鍚?user_id 鍜?tenant_id
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

	// 鐢熸垚璁よ瘉 token
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

// TestMemoryProxy_PathParameters 楠岃瘉璺緞鍙傛暟璺敱姝ｇ‘杞彂銆?func TestMemoryProxy_PathParameters(t *testing.T) {
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

	// 娴嬭瘯 DELETE /v1/memory/profile/{id}
	t.Run("DELETE profile with id", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/v1/memory/profile/item-123", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		// 楠岃瘉璺緞鍙傛暟琚纭紶閫?		if receivedPath != "/v1/memory/profile/item-123?user_id=test-user-id&tenant_id="+url.QueryEscape(db.DefaultTenantID) {
			t.Errorf("unexpected path: %s", receivedPath)
		}
	})

	// 娴嬭瘯 DELETE /v1/memory/conflicts/{conflict_id}
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

	// 娴嬭瘯 POST /v1/memory/conflicts/{conflict_id}/resolve
	t.Run("POST resolve conflict", func(t *testing.T) {
		// 娉ㄦ剰锛歅OST 璇锋眰蹇呴』鎻愪緵鍚堟硶 JSON body锛屽惁鍒?DecodeJSON 浼氬洜绌?body 杩斿洖 io.EOF锛?		// 浠ｇ悊澶勭悊閫昏緫鎻愬墠 return锛屽鑷?Python 绔粠鏈敹鍒拌姹傦紝鍝嶅簲浣撲负绌恒€?		req := httptest.NewRequest("POST", "/v1/memory/conflicts/conflict-789/resolve",
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

// TestMemoryProxy_MissingPythonEngine 楠岃瘉 Python 寮曟搸涓嶅彲鐢ㄦ椂杩斿洖 503銆?func TestMemoryProxy_MissingPythonEngine(t *testing.T) {
	// pyClient 鎸囧悜涓嶅瓨鍦ㄧ殑鍦板潃
	pyClient := engine.NewPythonClient("http://localhost:9999")
	router := testRouterWithPython(t, pyClient)
	token := testToken(t)

	req := httptest.NewRequest("GET", "/v1/memory/profile", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 浠ｇ悊璇锋眰浼氬け璐ワ紝鍥犱负 Python 寮曟搸涓嶅彲杈?	// 鏍规嵁瀹炵幇锛岃繖鍙兘杩斿洖 500 鎴?503
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusOK {
		t.Errorf("expected error when python engine unavailable, got %d", w.Code)
	}
}

// TestMemoryProxy_MissingTenantContext 楠岃瘉缂哄皯绉熸埛涓婁笅鏂囨椂杩斿洖 401銆?func TestMemoryProxy_MissingTenantContext(t *testing.T) {
	pythonServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer pythonServer.Close()

	pyClient := engine.NewPythonClient(pythonServer.URL)
	router := testRouterWithPython(t, pyClient)

	// 浣跨敤鑷畾涔?claims锛宼enant_id 涓虹┖
	claims := &auth.Claims{
		UserID:  "test-user",
		Role:    "user",
		TenantID: "", // 绌虹鎴?ID
	}
	req := httptest.NewRequest("GET", "/v1/memory/profile", nil)
	req = req.WithContext(auth.WithClaims(req.Context(), claims))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty tenant_id, got %d", w.Code)
	}
}
