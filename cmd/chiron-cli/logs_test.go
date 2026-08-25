package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readLastLines 鐨勫洖褰掓祴璇曪細璺ㄥ潡鏂銆侀暱琛屻€佺┖琛屻€佽鏁拌竟鐣?
func writeTempLog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readLastFromFile(t *testing.T, content string, n int) []string {
	t.Helper()
	path := writeTempLog(t, content)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lines, err := readLastLines(f, n)
	if err != nil {
		t.Fatalf("readLastLines: %v", err)
	}
	return lines
}

func TestReadLastLinesEmptyFile(t *testing.T) {
	lines := readLastFromFile(t, "", 10)
	if len(lines) != 0 {
		t.Fatalf("want no lines, got %d", len(lines))
	}
}

func TestReadLastLinesFewerThanN(t *testing.T) {
	content := "a\nb\nc\n"
	lines := readLastFromFile(t, content, 10)
	if len(lines) != 3 || lines[0] != "a" || lines[2] != "c" {
		t.Fatalf("want [a b c], got %v", lines)
	}
}

func TestReadLastLinesMoreThanNKeepsTailOrder(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("line-")
		sb.WriteString(string(rune('0' + i%10)))
		sb.WriteString("\n")
	}
	lines := readLastFromFile(t, sb.String(), 5)
	if len(lines) != 5 {
		t.Fatalf("want 5 lines, got %d", len(lines))
	}
	if lines[0] != "line-5" || lines[4] != "line-9" {
		t.Fatalf("tail order wrong: %v", lines)
	}
}

func TestReadLastLinesLongLineOverChunkBoundary(t *testing.T) {
	// 鍗曡 > 4096B锛堣法鍧楋級+ 鍚庣画琛岋細涓嶅緱鏂銆佷笉寰楅敊搴?	line := strings.Repeat("x", 5000)
	content := line + "\n" + "tail-line\n"
	lines := readLastFromFile(t, content, 3)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if lines[0] != line {
		t.Fatalf("long line broken: got %d bytes, want %d", len(lines[0]), len(line))
	}
	if lines[1] != "tail-line" {
		t.Fatalf("second line wrong: %v", lines[1])
	}
}

func TestReadLastLinesExactChunkBoundary(t *testing.T) {
	// 琛屾伆濂借法 4096 杈圭晫锛?095B 琛?+ 1B 濉厖鍚庡啀鎹㈣
	line := strings.Repeat("y", 4095)
	content := line + "z\nafter\n"
	lines := readLastFromFile(t, content, 2)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(lines))
	}
	if lines[0] != line+"z" {
		t.Fatalf("boundary line broken")
	}
	if lines[1] != "after" {
		t.Fatalf("second line wrong")
	}
}

func TestReadLastLinesEmptyLinesPreserved(t *testing.T) {
	content := "a\n\nb\n"
	lines := readLastFromFile(t, content, 3)
	if len(lines) != 3 {
		t.Fatalf("empty lines should be preserved, got %d: %v", len(lines), lines)
	}
}

func TestReadLastLinesNoTrailingNewline(t *testing.T) {
	content := "a\nb\nc"
	lines := readLastFromFile(t, content, 10)
	if len(lines) != 3 || lines[2] != "c" {
		t.Fatalf("want [a b c], got %v", lines)
	}
}

func TestServiceNameRe(t *testing.T) {
	valid := []string{"gateway", "python-engine", "svc_1"}
	for _, name := range valid {
		if !serviceNameRe.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	invalid := []string{"../etc", "..", "a b", `a\b`, "a*b", ""}
	for _, name := range invalid {
		if serviceNameRe.MatchString(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}
