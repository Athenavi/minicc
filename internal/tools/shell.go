package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ShellTool executes shell commands in a controlled sandbox.
// For security, only whitelisted commands are allowed by default.
type ShellTool struct {
	allowedCommands []string
	timeout         time.Duration
}

// NewShellTool creates a shell tool with the given command whitelist.
// If allowedCommands is empty, all commands are allowed (use with caution).
func NewShellTool(allowedCommands []string) *ShellTool {
	if allowedCommands == nil {
		allowedCommands = defaultAllowedCommands()
	}
	return &ShellTool{
		allowedCommands: allowedCommands,
		timeout:         30 * time.Second,
	}
}

func defaultAllowedCommands() []string {
	return []string{
		"ls", "cat", "head", "tail", "grep", "find", "wc",
		"echo", "pwd", "date", "whoami", "uname",
		"sort", "uniq", "cut", "tr", "diff", "cmp",
		"python", "python3", "node", "go", "npm",
		"git", "docker",
	}
}

func (t *ShellTool) Name() string        { return "shell_exec" }
func (t *ShellTool) Description() string { return "Execute a shell command and return its output." }
func (t *ShellTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"command": map[string]interface{}{
			"type":        "string",
			"description": "Shell command to execute",
		},
		"timeout": map[string]interface{}{
			"type":        "number",
			"description": "Timeout in seconds (default 30)",
		},
		"working_dir": map[string]interface{}{
			"type":        "string",
			"description": "Working directory for the command",
		},
	}
}

func (t *ShellTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	raw, _ := input["command"].(string)
	if raw == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeoutSec, _ := input["timeout"].(float64)
	if timeoutSec < 1 {
		timeoutSec = 30
	}

	workDir, _ := input["working_dir"].(string)

	// Parse command
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	cmdName := parts[0]

	// Check whitelist if not empty (empty = allow all)
	if len(t.allowedCommands) > 0 {
		allowed := false
		for _, ac := range t.allowedCommands {
			if ac == cmdName {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("command %q is not in the allowed list", cmdName)
		}
	}

	// Create timeout context
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if len(parts) == 1 {
		cmd = exec.CommandContext(execCtx, cmdName)
	} else {
		cmd = exec.CommandContext(execCtx, cmdName, parts[1:]...)
	}

	if workDir != "" {
		cmd.Dir = workDir
	}

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	if err != nil {
		// Include output even on error
		return map[string]interface{}{
			"output":   outStr,
			"error":    err.Error(),
			"exit_code": cmd.ProcessState.ExitCode(),
		}, nil
	}

	return map[string]interface{}{
		"output":    outStr,
		"exit_code": 0,
		"command":   raw,
	}, nil
}

// ExecutePythonTool runs Python code using the system python interpreter.
type ExecutePythonTool struct{}

func NewExecutePythonTool() *ExecutePythonTool { return &ExecutePythonTool{} }

func (t *ExecutePythonTool) Name() string        { return "execute_python" }
func (t *ExecutePythonTool) Description() string  { return "Execute Python code and return the output." }
func (t *ExecutePythonTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"code": map[string]interface{}{
			"type":        "string",
			"description": "Python code to execute",
		},
	}
}

func (t *ExecutePythonTool) Execute(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	code, _ := input["code"].(string)
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "python3", "-c", code)
	// Fallback to python if python3 not found
	if err := cmd.Err; err != nil {
		cmd = exec.CommandContext(execCtx, "python", "-c", code)
	}

	output, err := cmd.CombinedOutput()
	outStr := string(output)

	if err != nil {
		return map[string]interface{}{
			"output": outStr,
			"error":  err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"output": outStr,
	}, nil
}

// RegisterCommonTools adds the shell and execute_python tools.
// This augments the existing RegisterCommonTools in filesystem.go.
func RegisterShellTools(registry *ToolRegistry, allowedCommands []string) {
	registry.Register(NewShellTool(allowedCommands))
	registry.Register(NewExecutePythonTool())
}
