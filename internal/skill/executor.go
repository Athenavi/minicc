package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Executor runs a skill's ExecConfig and returns the result.
type Executor struct {
	httpClient *http.Client
}

func NewExecutor() *Executor {
	return &Executor{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Result holds the output of a skill execution.
type Result struct {
	Output   string `json:"output"`
	Error    string `json:"error,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

// Execute runs the skill's exec configuration with the given inputs.
func (ex *Executor) Execute(ctx context.Context, skill *SkillDef, inputs map[string]interface{}) (*Result, error) {
	switch skill.Exec.Type {
	case ExecPython:
		return ex.execPython(ctx, skill, inputs)
	case ExecShell:
		return ex.execShell(ctx, skill, inputs)
	case ExecHTTP:
		return ex.execHTTP(ctx, skill, inputs)
	case ExecPrompt:
		return ex.execPrompt(skill, inputs)
	default:
		return nil, fmt.Errorf("unsupported exec type: %s", skill.Exec.Type)
	}
}

// ── Python Executor ───────────────────────────────────────────────────────

func (ex *Executor) execPython(ctx context.Context, skill *SkillDef, inputs map[string]interface{}) (*Result, error) {
	timeout := skill.Exec.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	source := skill.Exec.Source
	if source == "" && skill.Exec.File != "" {
		return nil, fmt.Errorf("file-based python skills not yet supported, use inline source")
	}

	wrapper := fmt.Sprintf(`import sys, json
input_data = json.loads(sys.stdin.read()) if not sys.stdin.isatty() else {}
try:
%s
except Exception as e:
    print(json.dumps({"error": str(e)}))
    sys.exit(1)
`, indentSource(source, "    "))

	cmd := exec.CommandContext(execCtx, "python3", "-c", wrapper)
	cmd.Stdin = bytes.NewReader(mustJSON(inputs))

	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &Result{Output: outStr, Error: err.Error(), ExitCode: -1}, nil
		}
	}
	return &Result{Output: outStr, ExitCode: exitCode}, nil
}

// ── Shell Executor ────────────────────────────────────────────────────────

func (ex *Executor) execShell(ctx context.Context, skill *SkillDef, inputs map[string]interface{}) (*Result, error) {
	timeout := skill.Exec.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	command := skill.Exec.Source
	for k, v := range inputs {
		command = strings.ReplaceAll(command, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(execCtx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(execCtx, "sh", "-c", command)
	}

	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return &Result{Output: outStr, Error: err.Error(), ExitCode: -1}, nil
		}
	}
	return &Result{Output: outStr, ExitCode: exitCode}, nil
}

// ── HTTP Executor ─────────────────────────────────────────────────────────

func (ex *Executor) execHTTP(ctx context.Context, skill *SkillDef, inputs map[string]interface{}) (*Result, error) {
	url := skill.Exec.Source
	for k, v := range inputs {
		url = strings.ReplaceAll(url, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(mustJSON(inputs)))
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ex.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http call: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	return &Result{Output: string(body), ExitCode: resp.StatusCode}, nil
}

// ── Prompt Executor ───────────────────────────────────────────────────────

func (ex *Executor) execPrompt(skill *SkillDef, inputs map[string]interface{}) (*Result, error) {
	template := skill.Exec.Source
	for k, v := range inputs {
		template = strings.ReplaceAll(template, "{{"+k+"}}", fmt.Sprintf("%v", v))
	}
	return &Result{Output: template, ExitCode: 0}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────

func indentSource(source, indent string) string {
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

func mustJSON(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}
