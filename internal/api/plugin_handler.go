package api

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/minicc/config"
	"github.com/athenavi/minicc/internal/auth"
)

// PluginHandler manages per-user MCP plugin configurations.
// 配置存储：{PluginDataDir}/{user_id}/plugins.json（用户级隔离，S 安全修复：
// 原实现全局单文件，任何登录用户都可读写/修改其他用户的插件配置）。
type PluginHandler struct {
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
	Source      string            `json:"source,omitempty"` // "market" = 市场授权叠加项（非用户本地配置）
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

// userPluginPath 返回当前用户的插件配置文件路径。
func (h *PluginHandler) userPluginPath(userID string) string {
	return filepath.Join(h.dataDir, userID, "plugins.json")
}

// resolveUser 从请求认证信息取当前用户 ID（authMW 已保证登录）。
func (h *PluginHandler) resolveUser(r *http.Request) string {
	claims := auth.GetClaims(r.Context())
	if claims != nil {
		return claims.UserID
	}
	return ""
}

// resolveTenant 取当前租户 ID：claims 优先，缺省回退默认租户（市场门控用）。
func (h *PluginHandler) resolveTenant(r *http.Request) string {
	claims := auth.GetClaims(r.Context())
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return DefaultTenantID
}

// ── List ──

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

	// 叠加市场已授权插件（来源标注 market；查询失败静默跳过，不影响本地列表）
	plugins = h.overlayMarketPlugins(r, plugins)
	OK(w, plugins)
}

// overlayMarketPlugins 将租户已安装且启用的市场插件追加到列表（去重）。
func (h *PluginHandler) overlayMarketPlugins(r *http.Request, plugins []MCPPlugin) []MCPPlugin {
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

// ── Install ──

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

	// 企业市场门控：市场存在同名 published 条目且租户未启用时禁止安装
	// （查询失败 / 未上架能力由 IsItemEnabledForTenant 内部 fail-open 放行）
	if enabled, _ := IsItemEnabledForTenant(r.Context(), "plugin", name, h.resolveTenant(r)); !enabled {
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

// ── Uninstall ──

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

// ── Update ──

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
		Command     *string           `json:"command"`
		Args        *[]string         `json:"args,omitempty"`
		Env         *map[string]string `json:"env,omitempty"`
		Description *string           `json:"description,omitempty"`
		Version     *string           `json:"version,omitempty"`
		Status      *string           `json:"status,omitempty"`
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

// ── Test ──

func (h *PluginHandler) Test(w http.ResponseWriter, r *http.Request) {
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
			"ok": false, "message": "无法启动进程: " + err.Error(),
			"duration_ms": time.Since(start).Milliseconds(),
		})
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"minicc","version":"1.0"}}}`
	if _, err := stdin.Write([]byte(req + "\n")); err != nil {
		OK(w, map[string]interface{}{
			"ok": false, "message": "写入握手请求失败: " + err.Error(),
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
		msg := "MCP 握手成功"
		if !ok {
			msg = "无有效 MCP initialize 响应" + strings.TrimSpace(resp.line)
			if resp.err != nil {
				msg = "读取响应失败: " + resp.err.Error()
			}
		}
		OK(w, map[string]interface{}{
			"ok": ok, "message": msg,
			"duration_ms": time.Since(start).Milliseconds(),
		})
	case <-ctx.Done():
		OK(w, map[string]interface{}{
			"ok": false, "message": "连接超时（无 MCP initialize 响应）",
			"duration_ms": time.Since(start).Milliseconds(),
		})
	}
}

// ── Internal helpers ──

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
