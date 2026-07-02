package tools

import (
	"context"
	"strings"
	"testing"
)

func TestShellTool_Name(t *testing.T) {
	st := NewShellTool(nil)
	if st.Name() != "shell_exec" {
		t.Fatalf("expected 'shell_exec', got %q", st.Name())
	}
}

func TestShellTool_EmptyCommand(t *testing.T) {
	st := NewShellTool(nil)
	_, err := st.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestShellTool_DisallowedCommand(t *testing.T) {
	st := NewShellTool([]string{"ls", "echo"})
	_, err := st.Execute(context.Background(), map[string]interface{}{
		"command": "rm -rf /",
	})
	if err == nil {
		t.Fatal("expected error for disallowed command")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Fatalf("expected 'not in the allowed list' error, got %v", err)
	}
}

func TestShellTool_ValidCommand(t *testing.T) {
	st := NewShellTool(nil)
	result, err := st.Execute(context.Background(), map[string]interface{}{
		"command": "go version",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if !strings.Contains(output, "go") {
		t.Fatalf("expected output containing 'go', got %q", output)
	}
	exitCode, _ := result["exit_code"].(int)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}

func TestShellTool_Timeout(t *testing.T) {
	st := NewShellTool(nil)
	result, err := st.Execute(context.Background(), map[string]interface{}{
		"command": "go version",
		"timeout": float64(5),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if !strings.Contains(output, "go") {
		t.Fatalf("expected 'go' in output, got %q", output)
	}
}

func TestShellTool_WorkingDir(t *testing.T) {
	st := NewShellTool(nil)
	_, err := st.Execute(context.Background(), map[string]interface{}{
		"command":     "go version",
		"working_dir": ".",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestExecutePythonTool_Name(t *testing.T) {
	pt := NewExecutePythonTool()
	if pt.Name() != "execute_python" {
		t.Fatalf("expected 'execute_python', got %q", pt.Name())
	}
}

func TestExecutePythonTool_EmptyCode(t *testing.T) {
	pt := NewExecutePythonTool()
	_, err := pt.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestExecutePythonTool_BasicCode(t *testing.T) {
	pt := NewExecutePythonTool()
	result, err := pt.Execute(context.Background(), map[string]interface{}{
		"code": "print('hello from python')",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	output, _ := result["output"].(string)
	if !strings.Contains(output, "hello from python") {
		t.Fatalf("expected 'hello from python' in output, got %q", output)
	}
}

func TestDefaultAllowedCommands(t *testing.T) {
	cmds := defaultAllowedCommands()
	if len(cmds) == 0 {
		t.Fatal("expected non-empty default allowed commands")
	}
	found := false
	for _, c := range cmds {
		if c == "echo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'echo' in default allowed commands")
	}
}

func TestNewShellTool_CustomAllowed(t *testing.T) {
	st := NewShellTool([]string{"git", "echo"})
	if len(st.allowedCommands) != 2 {
		t.Fatalf("expected 2 allowed commands, got %d", len(st.allowedCommands))
	}
}
