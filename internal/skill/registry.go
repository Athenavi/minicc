package skill

import (
	"context"
	"fmt"
	"sync"

	"github.com/athenavi/minicc/internal/tools"
)

// DynamicRegistry wraps a ToolRegistry and adds skill management.
// It provides the bridge between SkillDef definitions and the tools.Tool interface.
type DynamicRegistry struct {
	store    *SkillStore
	executor *Executor
	tools    *tools.ToolRegistry
	mu       sync.RWMutex
}

// NewDynamicRegistry creates a registry that bridges skills → tools.
// Loads existing skills from the store and registers them as tools.
func NewDynamicRegistry(store *SkillStore, executor *Executor, toolReg *tools.ToolRegistry) (*DynamicRegistry, error) {
	dr := &DynamicRegistry{
		store:    store,
		executor: executor,
		tools:    toolReg,
	}

	// Register all existing skills from the store
	for _, skill := range store.List() {
		dr.registerSkillTool(skill)
	}

	return dr, nil
}

// InstallSkill installs a new skill: saves to store + registers as tool.
func (dr *DynamicRegistry) InstallSkill(skill *SkillDef) error {
	if err := dr.store.Save(skill); err != nil {
		return fmt.Errorf("install skill: %w", err)
	}
	dr.registerSkillTool(skill)
	return nil
}

// UninstallSkill removes a skill: deletes from store + unregisters tool.
func (dr *DynamicRegistry) UninstallSkill(name string) error {
	if err := dr.store.Remove(name); err != nil {
		return fmt.Errorf("uninstall skill: %w", err)
	}
	// Note: ToolRegistry doesn't support Unregister, so we just remove from store.
	// The tool remains registered until restart. This is intentional — avoids
	// race conditions with in-flight executions.
	return nil
}

// ListSkills returns all installed skill definitions.
func (dr *DynamicRegistry) ListSkills() []*SkillDef {
	return dr.store.List()
}

// GetSkill returns a skill definition by name.
func (dr *DynamicRegistry) GetSkill(name string) *SkillDef {
	return dr.store.Get(name)
}

// registerSkillTool wraps a SkillDef as a tools.Tool and registers it.
func (dr *DynamicRegistry) registerSkillTool(skill *SkillDef) {
	dr.mu.Lock()
	defer dr.mu.Unlock()

	tool := &SkillTool{
		def:      skill,
		executor: dr.executor,
	}
	dr.tools.Register(tool)
}

// SkillTool wraps a SkillDef as a tools.Tool for dynamic execution.
type SkillTool struct {
	def      *SkillDef
	executor *Executor
}

func (st *SkillTool) Name() string        { return st.def.Name }
func (st *SkillTool) Description() string  { return st.def.Description }

func (st *SkillTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	result, err := st.executor.Execute(ctx, st.def, input)
	if err != nil {
		return nil, fmt.Errorf("skill %q: %w", st.def.Name, err)
	}
	output := map[string]interface{}{
		"output":    result.Output,
		"exit_code": result.ExitCode,
	}
	if result.Error != "" {
		output["error"] = result.Error
	}
	return output, nil
}

// Ensure SkillTool implements tools.Tool
var _ tools.Tool = (*SkillTool)(nil)
