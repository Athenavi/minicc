package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/athenavi/minicc/internal/tools"
)

// ── Skill List Tool ───────────────────────────────────────────────────────

type skillListTool struct {
	registry *DynamicRegistry
}

func (t *skillListTool) Name() string        { return "skill_list" }
func (t *skillListTool) Description() string  { return "List all installed skills with their descriptions and versions." }
func (t *skillListTool) Parameters() map[string]interface{} { return map[string]interface{}{} }

func (t *skillListTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	skills := t.registry.ListSkills()
	lines := make([]string, 0, len(skills)+1)
	lines = append(lines, fmt.Sprintf("Installed skills (%d):", len(skills)))
	for _, s := range skills {
		lines = append(lines, fmt.Sprintf("  - %s: %s (v%s, %s)", s.Name, s.Description, s.Version, s.Exec.Type))
	}
	return map[string]interface{}{
		"output": strings.Join(lines, "\n"),
		"count":  len(skills),
		"skills": skills,
	}, nil
}

// ── Skill Install Tool ────────────────────────────────────────────────────

type skillInstallTool struct {
	registry  *DynamicRegistry
	installer *Installer
}

func (t *skillInstallTool) Name() string        { return "skill_install" }
func (t *skillInstallTool) Description() string  { return "Install a skill from a URL, local file path, or inline JSON definition." }
func (t *skillInstallTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"url": map[string]interface{}{"type": "string", "description": "URL to download the skill JSON from"},
		"file": map[string]interface{}{"type": "string", "description": "Local file path to the .skill.json file"},
		"inline": map[string]interface{}{"type": "string", "description": "Inline JSON skill definition"},
	}
}

func (t *skillInstallTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	url, _ := input["url"].(string)
	filePath, _ := input["file"].(string)
	inlineJSON, _ := input["inline"].(string)

	var installed *SkillDef
	var err error

	switch {
	case url != "":
		installed, err = t.installer.InstallFromURL(ctx, url)
	case filePath != "":
		installed, err = t.installer.InstallFromFile(filePath)
	case inlineJSON != "":
		installed, err = t.installer.InstallFromInline(ctx, inlineJSON)
	default:
		return nil, fmt.Errorf("one of url, file, or inline is required")
	}
	if err != nil {
		return nil, fmt.Errorf("install skill: %w", err)
	}
	if err := t.registry.InstallSkill(installed); err != nil {
		return nil, fmt.Errorf("register skill: %w", err)
	}

	jsonStr, _ := json.MarshalIndent(installed, "", "  ")
	return map[string]interface{}{
		"output":  fmt.Sprintf("Skill installed: %s (v%s)\n%s", installed.Name, installed.Version, string(jsonStr)),
		"skill":   installed.Name,
		"version": installed.Version,
	}, nil
}

// ── Skill Generate Tool ───────────────────────────────────────────────────

type skillGenerateTool struct {
	generator *Generator
	registry  *DynamicRegistry
	installer *Installer
}

func (t *skillGenerateTool) Name() string        { return "skill_generate" }
func (t *skillGenerateTool) Description() string  { return "Generate a new skill from a natural language description and optionally install it." }
func (t *skillGenerateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"description": map[string]interface{}{"type": "string", "description": "Natural language description of the skill to generate"},
		"install": map[string]interface{}{"type": "boolean", "description": "Auto-install after generation (default: false)"},
	}
}

func (t *skillGenerateTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	desc, _ := input["description"].(string)
	if desc == "" {
		return nil, fmt.Errorf("description is required")
	}

	autoInstall := false
	if ai, ok := input["install"].(bool); ok {
		autoInstall = ai
	}

	skillDef, err := t.generator.GenerateFromDesc(desc)
	if err != nil {
		return nil, fmt.Errorf("generate skill: %w", err)
	}

	jsonStr, _ := json.MarshalIndent(skillDef, "", "  ")
	result := map[string]interface{}{
		"output": fmt.Sprintf("Generated skill definition:\n%s", string(jsonStr)),
		"skill":  jsonStr,
		"name":   skillDef.Name,
		"type":   string(skillDef.Exec.Type),
	}

	if autoInstall {
		installed, err := t.installer.InstallFromInline(ctx, string(jsonStr))
		if err != nil {
			result["install_error"] = err.Error()
			return result, nil
		}
		if err := t.registry.InstallSkill(installed); err != nil {
			result["install_error"] = err.Error()
			return result, nil
		}
		result["installed"] = true
		result["output"] = fmt.Sprintf("Generated and installed skill: %s\n%s", skillDef.Name, string(jsonStr))
	}
	return result, nil
}

// ── Skill Discover Tool ───────────────────────────────────────────────────

type skillDiscoverTool struct {
	discoverer *Discoverer
}

func (t *skillDiscoverTool) Name() string        { return "skill_discover" }
func (t *skillDiscoverTool) Description() string  { return "Discover available skills from local directory or remote index." }
func (t *skillDiscoverTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"url": map[string]interface{}{"type": "string", "description": "Remote index URL to discover skills from (optional, local scan if empty)"},
	}
}

func (t *skillDiscoverTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	remoteURL, _ := input["url"].(string)

	var results []DiscoverResult
	var err error
	if remoteURL != "" {
		results, err = t.discoverer.DiscoverRemote(remoteURL)
	} else {
		results, err = t.discoverer.DiscoverLocal()
	}
	if err != nil {
		return nil, fmt.Errorf("discover: %w", err)
	}

	lines := make([]string, 0, len(results)+1)
	lines = append(lines, fmt.Sprintf("Available skills (%d):", len(results)))
	for _, r := range results {
		status := " "
		if r.Installed {
			status = "✓"
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s: %s (v%s)", status, r.Name, r.Description, r.Version))
	}
	return map[string]interface{}{
		"output":  strings.Join(lines, "\n"),
		"count":   len(results),
		"results": results,
	}, nil
}

// ── Registration ──────────────────────────────────────────────────────────

// RegisterSkillTools registers the 4 skill management tools into the tool registry.
func RegisterSkillTools(reg *tools.ToolRegistry, dynamicReg *DynamicRegistry, installer *Installer, generator *Generator, discoverer *Discoverer) {
	reg.Register(&skillListTool{registry: dynamicReg})
	reg.Register(&skillInstallTool{registry: dynamicReg, installer: installer})
	reg.Register(&skillGenerateTool{generator: generator, registry: dynamicReg, installer: installer})
	reg.Register(&skillDiscoverTool{discoverer: discoverer})
}
