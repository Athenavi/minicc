package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/athenavi/chiron/internal/id"
)

// UserMarketHandler 闈㈠悜鐢ㄦ埛鐨勪笁澶у競鍦猴紙鎶€鑳?Agent/MCP锛夋祻瑙堜笌涓€閿畨瑁呫€?// 绠＄悊绔彂甯冧笌绉熸埛鎺堟潈浠嶈蛋 market_handler.go锛坋nt_catalog_items / ent_catalog_installs锛夈€?type UserMarketHandler struct {
	cfg          *config.Config
	pythonClient *engine.PythonClient
}

func NewUserMarketHandler(cfg *config.Config, pythonClient *engine.PythonClient) *UserMarketHandler {
	return &UserMarketHandler{cfg: cfg, pythonClient: pythonClient}
}

// marketItemSummary 鐢ㄦ埛鍙鐨勫競鍦烘潯鐩紙宸插彂甯?+ 绉熸埛鎺堟潈 fail-open锛夈€?type marketItemSummary struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"` // skill / agent / mcp
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Status    string          `json:"status"`
	Manifest  json.RawMessage `json:"manifest"`
	Installed bool            `json:"installed"`
}

// List 娴忚甯傚満锛欸ET /v1/market?type=skill|agent|mcp
func (h *UserMarketHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}
	itemType := strings.TrimSpace(r.URL.Query().Get("type"))
	if itemType == "" {
		BadRequest(w, "type is required (skill|agent|mcp)")
		return
	}
	if !validMarketItemTypes[itemType] {
		BadRequest(w, "invalid market type")
		return
	}

	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id::text, type, name, version, manifest, status FROM ent_catalog_items
		 WHERE type = $1 AND status = 'published' ORDER BY created_at DESC`, itemType)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "market list failed")
		return
	}
	defer rows.Close()

	items := make([]marketItemSummary, 0, 8)
	for rows.Next() {
		var it marketItemSummary
		var manifest []byte
		if err := rows.Scan(&it.ID, &it.Type, &it.Name, &it.Version, &manifest, &it.Status); err != nil {
			continue
		}
		it.Manifest = json.RawMessage(manifest)
		// 绉熸埛鎺堟潈 fail-open锛氭湁鎺堟潈璁板綍鍒欏繀椤?enabled锛涙棤璁板綍鏀捐
		if enabled, err := itemEnabledForTenant(r.Context(), it.ID, claims.TenantID); err == nil {
			it.Installed = enabled
		}
		items = append(items, it)
	}
	OK(w, map[string]interface{}{"items": items})
}

// itemEnabledForTenant 鏌ヨ绉熸埛瀹夎璁板綍锛坒ail-open锛氭棤璁板綍瑙嗕负鏈畨瑁咃級銆?func itemEnabledForTenant(ctx context.Context, itemID, tenantID string) (bool, error) {
	var enabled bool
	err := db.ReadPool().QueryRow(ctx,
		`SELECT COALESCE(enabled, false) FROM ent_catalog_installs WHERE item_id = $1 AND tenant_id = $2`,
		itemID, tenantID).Scan(&enabled)
	if err != nil {
		return false, err
	}
	return enabled, nil
}

// Install 涓€閿畨瑁咃細POST /v1/market/{type}/{itemID}/install
func (h *UserMarketHandler) Install(w http.ResponseWriter, r *http.Request) {
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}
	itemType := r.PathValue("type")
	itemID := r.PathValue("itemID")
	if !validMarketItemTypes[itemType] || itemID == "" {
		BadRequest(w, "invalid type or item id")
		return
	}

	var name string
	var manifest []byte
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT name, manifest FROM ent_catalog_items WHERE id = $1 AND type = $2 AND status = 'published'`,
		itemID, itemType).Scan(&name, &manifest); err != nil {
		NotFound(w, "market item not found or not published")
		return
	}

	var m map[string]interface{}
	if err := json.Unmarshal(manifest, &m); err != nil {
		InternalError(w, "invalid market manifest")
		return
	}

	switch itemType {
	case "skill":
		h.installSkill(w, r, claims, m)
	case "agent":
		h.installAgent(w, r, claims, m)
	case "mcp":
		h.installMCP(w, r, claims, m)
	default:
		BadRequest(w, "unsupported market type")
	}
}

// installSkill 鎶€鑳藉畨瑁咃細灏?manifest 瀹氫箟鍐欏叆 Python SkillStore锛堢敤鎴峰彲浜庡璇?Agent 涓皟鐢級銆?func (h *UserMarketHandler) installSkill(w http.ResponseWriter, r *http.Request, claims *auth.Claims, m map[string]interface{}) {
	if h.pythonClient == nil {
		InternalError(w, "python engine not available")
		return
	}
	skillName, _ := m["name"].(string)
	if skillName == "" {
		BadRequest(w, "manifest missing skill name")
		return
	}
	inline := map[string]interface{}{
		"name":        skillName,
		"description": m["description"],
		"exec":        m["exec"],
		"parameters":  m["parameters"],
	}
	body := map[string]interface{}{"inline": mustJSON(inline)}
	target := "/v1/skills/install?user_id=" + claims.UserID + "&tenant_id=" + claims.TenantID
	var resp map[string]interface{}
	if err := h.pythonClient.PostJSON(r.Context(), target, body, &resp); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "skill install failed")
		return
	}
	OK(w, map[string]interface{}{"success": true, "type": "skill", "name": skillName, "detail": resp})
}

// installAgent Agent 瀹夎锛歮anifest 蹇収澶嶅埗涓哄綋鍓嶇敤鎴风殑绉佹湁 Agent锛堜弗鏍肩鏈夛級銆?func (h *UserMarketHandler) installAgent(w http.ResponseWriter, r *http.Request, claims *auth.Claims, m map[string]interface{}) {
	agentID, err := id.UUID()
	if err != nil {
		InternalError(w, "generate id failed")
		return
	}
	name, _ := m["name"].(string)
	if name == "" {
		BadRequest(w, "manifest missing agent name")
		return
	}
	desc, _ := m["description"].(string)
	prompt, _ := m["system_prompt"].(string)
	tools, _ := json.Marshal(m["tools"])
	llm, _ := json.Marshal(m["llm_config"])
	maxTurns := intVal(m["max_turns"], 10)
	timeout := intVal(m["timeout_seconds"], 120)

	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO agents (id, tenant_id, user_id, name, description, system_prompt, tools, llm_config, max_turns, timeout_seconds, enabled)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true)`,
		agentID, claims.TenantID, claims.UserID, name, desc, prompt,
		string(tools), string(llm), maxTurns, timeout); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "agent install failed")
		return
	}
	OK(w, map[string]interface{}{"success": true, "type": "agent", "id": agentID, "name": name})
}

// installMCP MCP 瀹夎锛氬皢 manifest 鐨?MCP server 閰嶇疆杩藉姞鍒板綋鍓嶇敤鎴?plugins.json锛堝懡浠ら渶鍛戒腑鐧藉悕鍗曪級銆?func (h *UserMarketHandler) installMCP(w http.ResponseWriter, r *http.Request, claims *auth.Claims, m map[string]interface{}) {
	pName, _ := m["name"].(string)
	command, _ := m["command"].(string)
	if pName == "" || command == "" {
		BadRequest(w, "manifest missing mcp name/command")
		return
	}
	if err := checkPluginCommandAllowed(command); err != nil {
		Forbidden(w, err.Error())
		return
	}
	plugin := MCPPlugin{
		Name:    pName,
		Command: command,
		Args:    toStringSlice(m["args"]),
		Env:     toStringMap(m["env"]),
		Status:  "active",
		Source:  "market",
	}
	dir := h.cfg.PluginDataDir
	if dir == "" {
		dir = "./data/plugins"
	}
	path := userPluginPath(dir, claims.UserID)
	if err := appendPlugin(dir, claims.UserID, plugin); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "mcp install failed")
		return
	}
	_ = path
	OK(w, map[string]interface{}{"success": true, "type": "mcp", "name": pName})
}

// 鈹€鈹€ 灏忓伐鍏?鈹€鈹€

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func intVal(v interface{}, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return def
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toStringMap(v interface{}) map[string]string {
	obj, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, e := range obj {
		if s, ok := e.(string); ok {
			out[k] = s
		}
	}
	return out
}

// userPluginPath 杩斿洖鐢ㄦ埛鎻掍欢閰嶇疆璺緞锛堜笌 plugin_handler.go 鍚岃鍒欙級銆?func userPluginPath(dataDir, userID string) string {
	return dataDir + "/" + userID + "/plugins.json"
}

// appendPlugin 璇诲彇鐢ㄦ埛 plugins.json 骞惰拷鍔?MCP 鎻掍欢锛堝箓绛夛細鍚屽悕瑕嗙洊锛夈€?func appendPlugin(dataDir, userID string, plugin MCPPlugin) error {
	path := userPluginPath(dataDir, userID)
	plugins := []MCPPlugin{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		var pf pluginsFile
		if err := json.Unmarshal(data, &pf); err == nil {
			plugins = pf.MCPServers
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	replaced := false
	for i := range plugins {
		if plugins[i].Name == plugin.Name {
			plugins[i] = plugin
			replaced = true
			break
		}
	}
	if !replaced {
		plugins = append(plugins, plugin)
	}
	return writePluginsFile(path, plugins)
}

func writePluginsFile(path string, plugins []MCPPlugin) error {
	data, err := json.MarshalIndent(pluginsFile{MCPServers: plugins}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
