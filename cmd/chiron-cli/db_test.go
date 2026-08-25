package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeDSN(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"postgres://user:secret@localhost:5432/db", "postgres://user:*****@localhost:5432/db"},
		{"postgres://user:p@ss@localhost:5432/db", "postgres://user:*****@localhost:5432/db"},
		{"postgres://nopass@localhost:5432/db", "postgres://nopass@localhost:5432/db"},
		{"not-a-dsn", "not-a-dsn"},
	}
	for _, c := range cases {
		got := sanitizeDSN(c.in)
		if got != c.want {
			t.Errorf("sanitizeDSN(%q) = %q, want %q", c.in, got, c.want)
		}
		if strings.Contains(got, "secret") || strings.Contains(got, "p@ss") {
			t.Errorf("sanitizeDSN(%q) leaked password: %q", c.in, got)
		}
	}
}

func TestHasInternalMigrationFiles(t *testing.T) {
	dir := t.TempDir()

	// 绌虹洰褰?/ 涓嶅瓨鍦?鈫?false
	if hasInternalMigrationFiles(dir) {
		t.Error("empty dir should return false")
	}
	if hasInternalMigrationFiles(filepath.Join(dir, "missing")) {
		t.Error("missing dir should return false")
	}

	// 鍙湁 atlas 鏍煎紡 .sql 鈫?false
	if err := os.WriteFile(filepath.Join(dir, "20260101000001_init.sql"), []byte("-- x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasInternalMigrationFiles(dir) {
		t.Error("only .sql files should return false")
	}

	// 鍑虹幇 .up.sql 鈫?true
	if err := os.WriteFile(filepath.Join(dir, "20260101000002_x.up.sql"), []byte("-- x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasInternalMigrationFiles(dir) {
		t.Error(".up.sql should return true")
	}

	// 浠?.down.sql 涔?true
	dir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir2, "20260101000003_y.down.sql"), []byte("-- y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasInternalMigrationFiles(dir2) {
		t.Error(".down.sql should return true")
	}
}
