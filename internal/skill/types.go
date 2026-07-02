package skill

import (
	"encoding/json"
	"fmt"
	"time"
)

// ExecType defines how a skill is executed.
type ExecType string

const (
	ExecPython ExecType = "python" // Run inline Python via subprocess
	ExecShell  ExecType = "shell"  // Run shell command
	ExecHTTP   ExecType = "http"   // Call external HTTP API
	ExecPrompt ExecType = "prompt" // LLM prompt template
)

// ExecConfig describes how to execute a skill.
type ExecConfig struct {
	Type    ExecType `json:"type"`
	Source  string   `json:"source"`            // inline code / command / URL / prompt template
	File    string   `json:"file,omitempty"`    // path to external script file
	Timeout int      `json:"timeout,omitempty"` // seconds, default 30
}

// Parameter describes a single input parameter for a skill.
type Parameter struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // string | number | boolean | array | object
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// SkillDef is the complete definition of a skill.
type SkillDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Version     string      `json:"version,omitempty"`
	Author      string      `json:"author,omitempty"`
	Tags        []string    `json:"tags,omitempty"`
	Exec        ExecConfig  `json:"exec"`
	Parameters  []Parameter `json:"parameters,omitempty"`
	Source      string      `json:"source,omitempty"`     // where this skill was installed from (URL / path)
	InstalledAt time.Time   `json:"installed_at,omitempty"`
}

// Validate checks that a SkillDef has all required fields.
func (s *SkillDef) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if s.Description == "" {
		return fmt.Errorf("skill description is required")
	}
	switch s.Exec.Type {
	case ExecPython, ExecShell, ExecHTTP, ExecPrompt:
		// valid
	default:
		return fmt.Errorf("unknown exec type: %q (valid: python, shell, http, prompt)", s.Exec.Type)
	}
	if s.Exec.Source == "" && s.Exec.File == "" {
		return fmt.Errorf("exec source or file is required")
	}
	return nil
}

// ToJSON serializes the skill definition.
func (s *SkillDef) ToJSON() (string, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal skill: %w", err)
	}
	return string(data), nil
}

// Filename returns the canonical filename for this skill.
func (s *SkillDef) Filename() string {
	return s.Name + ".skill.json"
}
