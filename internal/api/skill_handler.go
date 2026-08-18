package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/engine"
)

// SkillHandler proxies skill requests to the Python AI engine.
type SkillHandler struct {
	python *engine.PythonClient
}

func NewSkillHandler(python *engine.PythonClient) *SkillHandler {
	return &SkillHandler{python: python}
}

var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func (h *SkillHandler) RegisterRoutes(mux *http.ServeMux) {
	// 同时注册精确和尾斜杠变体：Go ServeMux 中 "/v1/skills/" 只匹配子树（不匹配 "/v1/skills"），
	// 而前端调用的是 GET /v1/skills，Python 端注册的也是 /v1/skills。
	mux.HandleFunc("GET /v1/skills", h.proxy)
	mux.HandleFunc("GET /v1/skills/", h.proxy)
	mux.HandleFunc("POST /v1/skills/install", h.proxy)
	mux.HandleFunc("POST /v1/skills/generate", h.proxy)
	mux.HandleFunc("DELETE /v1/skills/{name}", h.proxyDelete)
	mux.HandleFunc("GET /v1/skills/discover", h.proxy)
	// 启停（PUT）与运行（POST run）——技能工作台主链路
	mux.HandleFunc("PUT /v1/skills/{name}", h.proxy)
	mux.HandleFunc("POST /v1/skills/{name}/run", h.proxy)
}

// proxy forwards the request to the Python engine.
func (h *SkillHandler) proxy(w http.ResponseWriter, r *http.Request) {
	if h.python == nil || !h.python.IsConnected() {
		InternalError(w, "python engine not available")
		return
	}

	var result map[string]interface{}
	var err error

	switch r.Method {
	case "GET":
		// 规范化转发路径：Python 端注册的是 /v1/skills（无尾斜杠）
		path := strings.TrimSuffix(r.URL.Path, "/")
		err = h.python.GetJSON(r.Context(), path, &result)
		if err == nil && strings.HasSuffix(path, "/discover") {
			filterDiscoverByMarket(r.Context(), result, skillTenantID(r))
		}
	case "POST", "PUT":
		var body map[string]interface{}
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, ErrInvalidReq)
			return
		}
		if r.Method == "PUT" {
			err = h.python.PutJSON(r.Context(), r.URL.Path, body, &result)
		} else {
			err = h.python.PostJSON(r.Context(), r.URL.Path, body, &result)
		}
	default:
		BadRequest(w, "unsupported method")
		return
	}

	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "python engine error")
		return
	}
	OK(w, result)
}

// proxyDelete forwards DELETE requests to the Python engine.
func (h *SkillHandler) proxyDelete(w http.ResponseWriter, r *http.Request) {
	if h.python == nil || !h.python.IsConnected() {
		InternalError(w, "python engine not available")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "name is required")
		return
	}
	if !validSkillName.MatchString(name) {
		BadRequest(w, "invalid skill name")
		return
	}

	var result map[string]interface{}
	if err := h.python.DeleteJSON(r.Context(), "/v1/skills/"+name, &result); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "python engine error")
		return
	}
	OK(w, result)
}

// skillTenantID 取当前租户 ID：claims 优先，缺省回退默认租户。
// （/v1/skills 路由未强制 authMW，claims 可能为 nil）
func skillTenantID(r *http.Request) string {
	if claims := auth.GetClaims(r.Context()); claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return DefaultTenantID
}

// filterDiscoverByMarket 对 discover 代理响应做市场白名单过滤：
// 仅当市场存在同名 published 条目时按租户授权过滤（IsItemEnabledForTenant
// 内部保证未上架能力与查询故障均放行）。PG 不可用时整体跳过，避免逐条 warn。
func filterDiscoverByMarket(ctx context.Context, result map[string]interface{}, tenantID string) {
	if db.ReadPool() == nil {
		return
	}
	// 兼容多种响应结构：在常见列表键中定位技能数组
	var list []interface{}
	var listKey string
	for _, key := range []string{"skills", "items", "data", "list"} {
		if arr, ok := result[key].([]interface{}); ok {
			list, listKey = arr, key
			break
		}
	}
	if listKey == "" {
		return
	}
	result[listKey] = filterSkillsByMarket(ctx, list, tenantID)
}

// filterSkillsByMarket 对技能列表逐项做市场门控过滤（纯逻辑，独立可测）。
func filterSkillsByMarket(ctx context.Context, list []interface{}, tenantID string) []interface{} {
	kept := make([]interface{}, 0, len(list))
	for _, raw := range list {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			kept = append(kept, raw)
			continue
		}
		name, _ := entry["name"].(string)
		if name == "" {
			kept = append(kept, raw)
			continue
		}
		if enabled, _ := IsItemEnabledForTenant(ctx, "skill", name, tenantID); enabled {
			kept = append(kept, raw)
		}
	}
	return kept
}
