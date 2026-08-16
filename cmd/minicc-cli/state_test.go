package main

import (
	"os"
	"testing"
)

// stateFilePath 基于当前工作目录；测试在临时目录中运行，避免污染仓库 .pids/

func withTempCwd(t *testing.T, fn func()) {
	t.Helper()
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	fn()
}

func TestStateUpsertRemoveRoundTrip(t *testing.T) {
	withTempCwd(t, func() {
		s := &State{}
		s.UpsertInstance(newInstance("gateway", 1234, 8080, "monolith", "logs/gateway.stdout.log"))
		s.UpsertInstance(newInstance("python-engine", 5678, 8000, "monolith", "logs/python-engine.stdout.log"))
		if err := saveState(s); err != nil {
			t.Fatalf("saveState: %v", err)
		}

		loaded, err := loadState()
		if err != nil {
			t.Fatalf("loadState: %v", err)
		}
		if len(loaded.Instances) != 2 {
			t.Fatalf("want 2 instances, got %d", len(loaded.Instances))
		}

		// upsert 按名称覆盖，不产生重复
		loaded.UpsertInstance(newInstance("gateway", 9999, 8080, "monolith", ""))
		if got := loaded.FindInstance("gateway"); got == nil || got.PID != 9999 {
			t.Fatalf("want pid 9999 after upsert, got %+v", got)
		}
		if len(loaded.Instances) != 2 {
			t.Fatalf("upsert should not add duplicate, got %d", len(loaded.Instances))
		}

		// remove
		if !loaded.RemoveInstance("gateway") {
			t.Fatal("remove should hit existing instance")
		}
		if loaded.FindInstance("gateway") != nil {
			t.Fatal("gateway should be gone after remove")
		}
		if loaded.RemoveInstance("gateway") {
			t.Fatal("second remove should miss")
		}
	})
}

func TestLoadStateMissingFileReturnsEmpty(t *testing.T) {
	withTempCwd(t, func() {
		s, err := loadState()
		if err != nil {
			t.Fatalf("loadState on missing file should not error: %v", err)
		}
		if len(s.Instances) != 0 {
			t.Fatalf("want empty state, got %d instances", len(s.Instances))
		}
	})
}

func TestDefaultPort(t *testing.T) {
	old := os.Getenv("PORT")
	defer os.Setenv("PORT", old)

	os.Unsetenv("PORT")
	if got := defaultPort(); got != 8080 {
		t.Fatalf("default port should be 8080, got %d", got)
	}
	os.Setenv("PORT", "9090")
	if got := defaultPort(); got != 9090 {
		t.Fatalf("port from env should be 9090, got %d", got)
	}
	os.Setenv("PORT", "not-a-number")
	if got := defaultPort(); got != 8080 {
		t.Fatalf("invalid PORT should fall back to 8080, got %d", got)
	}
}
