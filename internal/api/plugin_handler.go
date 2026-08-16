package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/athenavi/minicc/config"
)

// PluginHandler manages MCP plugin configurations stored in plugins.json.
type PluginHandler struct {
	cfg        *config.Config
	configPath string
	mu         sync.Mutex
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
}

// pluginsFile is the on-disk structure of plugins.json.
type pluginsFile struct {
	MCPServers []MCPPlugin `json:"mcp_servers"`
}

func NewPluginHandler(cfg *config.Config) *PluginHandler {
	path := cfg.PluginsConfigPath
	if path == "" {
		path = filepath.Join(".", "plugins.json")
	}
	return &PluginHandler{cfg: cfg, configPath: path}
}

// ── List ──

func (h *PluginHandler) List(w http.ResponseWriter, r *http.Request) {
	plugins, err := h.readPlugins()
	if err != nil {
		// File not found or parse error — return empty list
		if os.IsNotExist(err) {
			OK(w, []MCPPlugin{})
			return
		}
		slog.Error("plugin list: read plugins.json", "error", err)
		InternalError(w, "failed to read plugins config")
		return
	}

	// Ensure each plugin has a status
	for i := range plugins {
		if plugins[i].Status == "" {
			plugins[i].Status = "active"
		}
	}

	OK(w, plugins)
}

// ── Install ──

func (h *PluginHandler) Install(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "plugin name is required")
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
		BadRequest(w, "invalid request body")
		return
	}
	if body.Command == "" {
		BadRequest(w, "command is required")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	plugins, err := h.readPlugins()
	if err != nil && !os.IsNotExist(err) {
		slog.Error("plugin install: read plugins.json", "error", err)
		InternalError(w, "failed to read plugins config")
		return
	}
	if plugins == nil {
		plugins = []MCPPlugin{}
	}

	// Check for duplicate
	for _, p := range plugins {
		if p.Name == name {
			BadRequest(w, "plugin already installed: "+name)
			return
		}
	}

	plugin := MCPPlugin{
		Name:        name,
		Command:     body.Command,
		Args:        body.Args,
		Env:         body.Env,
		Description: body.Description,
		Version:     body.Version,
		Status:      "active",
	}
	plugins = append(plugins, plugin)

	if err := h.writePlugins(plugins); err != nil {
		slog.Error("plugin install: write plugins.json", "error", err)
		InternalError(w, "failed to save plugins config")
		return
	}

	slog.Info("plugin installed", "name", name, "command", body.Command)
	OK(w, plugin)
}

// ── Uninstall ──

func (h *PluginHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "plugin name is required")
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	plugins, err := h.readPlugins()
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

	if err := h.writePlugins(updated); err != nil {
		slog.Error("plugin uninstall: write plugins.json", "error", err)
		InternalError(w, "failed to save plugins config")
		return
	}

	slog.Info("plugin uninstalled", "name", name)
	OK(w, map[string]string{"status": "deleted", "name": name})
}

// ── Internal helpers ──

func (h *PluginHandler) readPlugins() ([]MCPPlugin, error) {
	data, err := os.ReadFile(h.configPath)
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

func (h *PluginHandler) writePlugins(plugins []MCPPlugin) error {
	if plugins == nil {
		plugins = []MCPPlugin{}
	}
	pf := pluginsFile{MCPServers: plugins}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.configPath, data, 0644)
}
