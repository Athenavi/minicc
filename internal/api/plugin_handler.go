package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/chiron/config"
	"github.com/athenavi/chiron/internal/auth"
)

// allowedPluginCommands 杩斿洖 PLUGIN_COMMAND_ALLOWLIST 鐜鍙橀噺閰嶇疆鐨勫懡浠ょ櫧鍚嶅崟
// 锛堜互閫楀彿鍒嗛殧鐨勫彲鎵ц鏂囦欢 basename锛夈€傜┖鍒楄〃 = 榛樿绂佺敤鑷畾涔夋彃浠跺懡浠?// 锛堝畨鍏ㄩ粯璁わ細闃叉浠绘剰鐧诲綍鐢ㄦ埛閰嶇疆浠绘剰鍛戒护骞跺湪缃戝叧/寮曟搸涓绘満鎵ц锛夈€?func allowedPluginCommands() map[string]bool {
	raw := strings.TrimSpace(os.Getenv("PLUGIN_COMMAND_ALLOWLIST"))
	if raw == "" {
		return nil
	}
	allowed := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			allowed[part] = true
		}
	}
	return allowed
}

// checkPluginCommandAllowed 鏍￠獙鍛戒护 basename 鏄惁鍦ㄧ櫧鍚嶅崟鍐呫€?func checkPluginCommandAllowed(command string) error {
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command is required")
	}
	allowed := allowedPluginCommands()
	if allowed == nil {
		return fmt.Errorf("plugin command execution is disabled: set PLUGIN_COMMAND_ALLOWLIST to enable specific commands")
	}
	base := filepath.Base(command)
	if !allowed[base] {
		return fmt.Errorf("plugin command %q not in allowlist (PLUGIN_COMMAND_ALLOWLIST)", base)
	}
	return nil
}

// isAdminRole 鍒ゆ柇褰撳墠璇锋眰鏄惁涓?owner/admin锛堟彃浠跺懡浠ゆ墽琛屾晱鎰熸搷浣滐級銆?func isAdminRole(r *http.Request) bool {
	claims := auth.GetClaims(r.Context())
	if claims == nil {
		return false
	}
	return claims.Role == "owner" || claims.Role == "admin"
}

// maskSensitiveEnv 瀵?MCPPlugin.Env 涓枒浼兼晱鎰熷瓧娈靛仛鑴辨晱澶勭悊
func maskSensitiveEnv(plugins []MCPPlugin) {
	sensitiveKeys := []string{"key", "secret", "token", "password", "api_key", "apikey"}
	for i := range plugins {
		for k, v := range plugins[i].Env {
			if len(v) <= 4 {
				continue
			}
			for _, sk := range sensitiveKeys {
				if strings.Contains(strings.ToLower(k), sk) {
					plugins[i].Env[k] = v[:2] + "***" + v[len(v)-2:]
					break
				}
			}
		}
	}
}

// PluginHandler manages per-user MCP plugin configurations.
// 閰嶇疆瀛樺偍锛歿PluginDataDir}/{user_id}/plugins.json锛堢敤鎴风骇闅旂锛孲 瀹夊叏淇锛?// 鍘熷疄鐜板叏灞€鍗曟枃浠讹紝浠讳綍鐧诲綍鐢ㄦ埛閮藉彲璇诲啓/淇敼鍏朵粬鐢ㄦ埛鐨勬彃浠堕厤缃級銆?type PluginHandler struct {
	cfg           *config.Config
	authenticator *auth.Authenticator
	dataDir       string
	mu            sync.Mutex
}

// MCPPlugin represents an MCP server configuration entry.
type MCPPlugin struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
	Status      string            `json:"status"`
	Source      string            `json:"source,omitempty"` // "market" = 甯傚満鎺堟潈鍙犲姞椤癸紙闈炵敤鎴锋湰鍦伴厤缃級
}

// pluginsFile is the on-disk structure of plugins.json.
type pluginsFile struct {
	MCPServers []MCPPlugin `json:"mcp_servers"`
}

func NewPluginHandler(cfg *config.Config, authenticator *auth.Authenticator) *PluginHandler {
	dir := cfg.PluginDataDir
	if dir == "" {
		dir = filepath.Join(".", "data", "plugins")
	}
	return &PluginHandler{cfg: cfg, authenticator: authenticator, dataDir: dir}
}

// userPluginPath 杩斿洖褰撳墠鐢ㄦ埛鐨勬彃浠堕厤缃枃浠惰矾寰勩€?func (h *PluginHandler) userPluginPath(userID string) string {
	// 瀹夊叏锛氭竻鐞?userID 闃叉璺緞閬嶅巻锛堝 ../tenant/evil锛?	safe := filepath.Clean(filepath.Base(userID))
	if safe == "." || safe == "" {
		safe = "unknown"
	}
	return filepath.Join(h.dataDir, safe, "plugins.json")
}

// resolveUser 浠庤姹傝璇佷俊鎭彇褰撳墠鐢ㄦ埛 ID锛坅uthMW 宸蹭繚璇佺櫥褰曪級銆?func (h *PluginHandler) resolveUser(r *http.Request) string {
	claims := auth.GetClaims(r.Context())
	if claims != nil {
		return claims.UserID
	}
	return ""
}

// resolveTenant 鍙栧綋鍓嶇鎴?ID锛歝laims 浼樺厛锛岀己鐪佸洖閫€榛樿绉熸埛锛堝競鍦洪棬鎺х敤锛夈€?func (h *PluginHandler) resolveTenant(r *http.Request) string {
	claims := auth.GetClaims(r.Context())
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return DefaultTenantID
}

// 鈹€鈹€ List 鈹€鈹€

func (h *PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUser(r)
	if userID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	plugins, err := h.readPlugins(userID)
	if err != nil {
		if os.IsNotExist(err) {
			OK(w, []MCPPlugin{})
			return
		}
		slog.Error("plugin list: read plugins.json", "error", err)
		InternalError(w, "failed to read plugins config")
		return
	}

	for i := range plugins {
		if plugins[i].Status == "" {
			plugins[i].Status = "active"
		}
	}

	// 鍙犲姞甯傚満宸叉巿鏉冩彃浠讹紙鏉ユ簮鏍囨敞 market锛涙煡璇㈠け璐ラ潤榛樿烦杩囷紝涓嶅奖鍝嶆湰鍦板垪琛級
	plugins = h.overlayMarketPlugins(r, plugins)
	maskSensitiveEnv(plugins)
	OK(w, plugins)
}

// overlayMarketPlugins 灏嗙鎴峰凡瀹夎涓斿惎鐢ㄧ殑甯傚満鎻掍欢杩藉姞鍒板垪琛紙鍘婚噸锛夈€?func (h *PluginHandler) overlayMarketPlugins(r *http.Request, plugins []MCPPlugin) []MCPPlugin {
	items, err := ListEnabledMarketItems(r.Context(), "plugin", h.resolveTenant(r))
	if err != nil {
		slog.Debug("plugin list: market overlay skipped", "error", err)
		return plugins
	}
	existing := make(map[string]bool, len(plugins))
	for _, p := range plugins {
		existing[p.Name] = true
	}
	for _, it := range items {
		if existing[it.Name] {
			continue
		}
		var manifest struct {
			Command     string            `json:"command"`
			Args        []string          `json:"args"`
			Env         map[string]string `json:"env"`
			Description string            `json:"description"`
		}
		_ = json.Unmarshal(it.Manifest, &manifest)
		plugins = append(plugins, MCPPlugin{
			Name:        it.Name,
			Command:     manifest.Command,
			Args:        manifest.Args,
			Env:         manifest.Env,
			Description: manifest.Description,
			Version:     it.Version,
			Status:      "active",
			Source:      "market",
		})
	}
	return plugins
}

// 鈹€鈹€ Install 鈹€鈹€

func (h *PluginHandler) Install(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUser(r)
	if userID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "plugin name is required")
		return
	}
	if !validPluginName(name) {
		BadRequest(w, "invalid plugin name")
		return
	}

	// 浼佷笟甯傚満闂ㄦ帶锛氬競鍦哄瓨鍦ㄥ悓鍚?published 鏉＄洰涓旂鎴锋湭鍚敤鏃剁姝㈠畨瑁?	// 锛堟煡璇㈠け璐?/ 鏈笂鏋惰兘鍔涚敱 IsItemEnabledForTenant 鍐呴儴 fail-open 鏀捐锛?	if enabled, _ := IsItemEnabledForTenant(r.Context(), "plugin", name, h.resolveTenant(r)); !enabled {
		Forbidden(w, "plugin is not enabled for this tenant by market policy")
		return
	}

	var body struct {
		Command     string            `json:"command"`
		Args        []string          `json:"args,omitempty"`
		Env         map[string]string `json:"env,omitempty"`
		Description string            `json:"description"`
		Version     string            `json:"version"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Command == "" {
		BadRequest(w, "command is required")
		return
	}
	// P0-S7 淇锛氬懡浠ゅ繀椤诲懡涓櫧鍚嶅崟锛堥粯璁ょ鐢級锛岄槻浠绘剰鍛戒护鎵ц
	if err := checkPluginCommandAllowed(body.Command); err != nil {
		Forbidden(w, err.Error())
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	plugins, err := h.readPlugins(userID)
	if err != nil && !os.IsNotExist(err) {
		slog.Error("plugin install: read plugins.json", "error", err)
		InternalError(w, "failed to read plugins config")
		return
	}
	if plugins == nil {
		plugins = []MCPPlugin{}
	}

	for _, p := range plugins {
		if p.Name == name {
			BadRequest(w, "plugin already installed: "+name)
			return
		}
	}

	plugin := MCPPlugin{
		Name: name, Command: body.Command, Args: body.Args, Env: body.Env,
		Description: body.Description, Version: body.Version, Status: "active",
	}
	plugins = append(plugins, plugin)

	if err := h.writePlugins(userID, plugins); err != nil {
		slog.Error("plugin install: write plugins.json", "error", err)
		InternalError(w, "failed to save plugins config")
		return
	}

	slog.Info("plugin installed", "user", userID, "name", name, "command", body.Command)
	OK(w, plugin)
}

// 鈹€鈹€ Uninstall 鈹€鈹€

func (h *PluginHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUser(r)
	if userID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "plugin name is required")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	plugins, err := h.readPlugins(userID)
	if err != nil {
		if os.IsNotExist(err) {
			NotFound(w, "plugin not found: "+name)
			return
		}
		slog.Error("plugin uninstall: read plugins.json", "error", err)
		InternalError(w, "failed to read plugins config")
		return
	}

	found := false
	updated := make([]MCPPlugin, 0, len(plugins))
	for _, p := range plugins {
		if p.Name == name {
			found = true
			continue
		}
		updated = append(updated, p)
	}
	if !found {
		NotFound(w, "plugin not found: "+name)
		return
	}

	if err := h.writePlugins(userID, updated); err != nil {
		slog.Error("plugin uninstall: write plugins.json", "error", err)
		InternalError(w, "failed to save plugins config")
		return
	}

	slog.Info("plugin uninstalled", "user", userID, "name", name)
	OK(w, map[string]string{"status": "deleted", "name": name})
}

// 鈹€鈹€ Update 鈹€鈹€

func (h *PluginHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := h.resolveUser(r)
	if userID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "plugin name is required")
		return
	}
	var body struct {
		Command     *string            `json:"command"`
		Args        *[]string          `json:"args,omitempty"`
		Env         *map[string]string `json:"env,omitempty"`
		Description *string            `json:"description,omitempty"`
		Version     *string            `json:"version,omitempty"`
		Status      *string            `json:"status,omitempty"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Status != nil && *body.Status != "active" && *body.Status != "inactive" {
		BadRequest(w, "status must be active or inactive")
		return
	}
	if body.Command == nil && body.Args == nil && body.Env == nil &&
		body.Description == nil && body.Version == nil && body.Status == nil {
		BadRequest(w, "nothing to update")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	plugins, err := h.readPlugins(userID)
	if err != nil {
		InternalError(w, "failed to read plugins config")
		return
	}
	updated := false
	for i := range plugins {
		if plugins[i].Name != name {
			continue
		}
		if body.Command != nil {
			if strings.TrimSpace(*body.Command) == "" {
				BadRequest(w, "command must not be empty")
				return
			}
			// P0-S7 淇锛氬懡浠ゅ繀椤诲懡涓櫧鍚嶅崟锛堥粯璁ょ鐢級
			if err := checkPluginCommandAllowed(*body.Command); err != nil {
				Forbidden(w, err.Error())
				return
			}
			plugins[i].Command = *body.Command
		}
		if body.Args != nil {
			plugins[i].Args = *body.Args
		}
		if body.Env != nil {
			plugins[i].Env = *body.Env
		}
		if body.Description != nil {
			plugins[i].Description = *body.Description
		}
		if body.Version != nil {
			plugins[i].Version = *body.Version
		}
		if body.Status != nil {
			plugins[i].Status = *body.Status
		}
		updated = true
		break
	}
	if !updated {
		NotFound(w, "plugin not found: "+name)
		return
	}
	if err := h.writePlugins(userID, plugins); err != nil {
		InternalError(w, "failed to save plugins config")
		return
	}
	for _, p := range plugins {
		if p.Name == name {
			OK(w, p)
			return
		}
	}
	OK(w, map[string]string{"name": name, "updated": "true"})
}

// 鈹€鈹€ Test 鈹€鈹€

func (h *PluginHandler) Test(w http.ResponseWriter, r *http.Request) {
	// P0-S7 淇锛氭墽琛岀敤鎴疯嚜瀹氫箟鍛戒护鐨勬祴璇曠鐐逛粎闄?owner/admin
	if !isAdminRole(r) {
		Forbidden(w, "plugin test requires admin role")
		return
	}
	userID := h.resolveUser(r)
	if userID == "" {
		Unauthorized(w, ErrAuthRequired)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "plugin name is required")
		return
	}

	plugins, err := h.readPlugins(userID)
	if err != nil {
		InternalError(w, "failed to read plugins config")
		return
	}
	var plugin *MCPPlugin
	for i := range plugins {
		if plugins[i].Name == name {
			plugin = &plugins[i]
			break
		}
	}
	if plugin == nil {
		NotFound(w, "plugin not found: "+name)
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, plugin.Command, plugin.Args...)
	cmd.Env = os.Environ()
	for k, v := range plugin.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "stdin pipe failed")
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "stdout pipe failed")
		return
	}
	if err := cmd.Start(); err != nil {
		OK(w, map[string]interface{}{
			"ok": false, "message": "鏃犳硶鍚姩杩涚▼: " + err.Error(),
			"duration_ms": time.Since(start).Milliseconds(),
		})
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"chiron","version":"1.0"}}}`
	if _, err := stdin.Write([]byte(req + "\n")); err != nil {
		OK(w, map[string]interface{}{
			"ok": false, "message": "鍐欏叆鎻℃墜璇锋眰澶辫触: " + err.Error(),
			"duration_ms": time.Since(start).Milliseconds(),
		})
		return
	}

	type respLine struct {
		line string
		err  error
	}
	ch := make(chan respLine, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		ch <- respLine{line: line, err: err}
	}()

	select {
	case resp := <-ch:
		ok := resp.err == nil && strings.Contains(resp.line, "jsonrpc")
		msg := "MCP 鎻℃墜鎴愬姛"
		if !ok {
			msg = "鏃犳湁鏁?MCP initialize 鍝嶅簲" + strings.TrimSpace(resp.line)
			if resp.err != nil {
				msg = "璇诲彇鍝嶅簲澶辫触: " + resp.err.Error()
			}
		}
		OK(w, map[string]interface{}{
			"ok": ok, "message": msg,
			"duration_ms": time.Since(start).Milliseconds(),
		})
	case <-ctx.Done():
		OK(w, map[string]interface{}{
			"ok": false, "message": "杩炴帴瓒呮椂锛堟棤 MCP initialize 鍝嶅簲锛?,
			"duration_ms": time.Since(start).Milliseconds(),
		})
	}
}

// 鈹€鈹€ Internal helpers 鈹€鈹€

func (h *PluginHandler) readPlugins(userID string) ([]MCPPlugin, error) {
	data, err := os.ReadFile(h.userPluginPath(userID))
	if err != nil {
		return nil, err
	}
	var pf pluginsFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, err
	}
	if pf.MCPServers == nil {
		return []MCPPlugin{}, nil
	}
	return pf.MCPServers, nil
}

func (h *PluginHandler) writePlugins(userID string, plugins []MCPPlugin) error {
	if plugins == nil {
		plugins = []MCPPlugin{}
	}
	path := h.userPluginPath(userID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	pf := pluginsFile{MCPServers: plugins}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

var validPluginName = func() func(string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	return re.MatchString
}()
