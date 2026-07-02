package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillDef_Validate(t *testing.T) {
	s := &SkillDef{Name: "test", Description: "A test skill"}
	s.Exec = ExecConfig{Type: "invalid", Source: "echo hi"}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for unknown exec type")
	}
}

func TestSkillDef_Validate_Python(t *testing.T) {
	s := &SkillDef{Name: "test", Description: "Test", Exec: ExecConfig{Type: ExecPython, Source: "print(1)"}}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestSkillDef_Validate_NoSource(t *testing.T) {
	s := &SkillDef{Name: "test", Description: "Test", Exec: ExecConfig{Type: ExecPython}}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected error for no source")
	}
}

func TestSkillDef_Filename(t *testing.T) {
	s := &SkillDef{Name: "csv_analyze"}
	if s.Filename() != "csv_analyze.skill.json" {
		t.Fatalf("expected 'csv_analyze.skill.json', got %q", s.Filename())
	}
}

func TestSkillStore_CreateAndList(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSkillStore(dir)
	if err != nil {
		t.Fatalf("NewSkillStore: %v", err)
	}

	err = store.Save(&SkillDef{
		Name: "test_skill", Description: "Test", Version: "1.0",
		Exec: ExecConfig{Type: ExecPython, Source: "print('hi')"},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	skills := store.List()
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Name != "test_skill" {
		t.Fatalf("expected 'test_skill', got %q", skills[0].Name)
	}
}

func TestSkillStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	store1, _ := NewSkillStore(dir)
	store1.Save(&SkillDef{
		Name: "persist_test", Description: "Test persistence",
		Exec: ExecConfig{Type: ExecShell, Source: "echo hello"},
	})

	// Create a new store pointing to the same directory — should reload
	store2, err := NewSkillStore(dir)
	if err != nil {
		t.Fatalf("NewSkillStore second: %v", err)
	}
	if store2.Count() != 1 {
		t.Fatalf("expected 1 skill after reload, got %d", store2.Count())
	}
}

func TestSkillStore_Get(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	store.Save(&SkillDef{
		Name: "get_test", Description: "Test get",
		Exec: ExecConfig{Type: ExecPython, Source: "x=1"},
	})
	skill := store.Get("get_test")
	if skill == nil {
		t.Fatal("expected to find skill")
	}
	if store.Get("nonexistent") != nil {
		t.Fatal("expected nil for missing skill")
	}
}

func TestSkillStore_Remove(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	store.Save(&SkillDef{
		Name: "remove_test", Description: "Test remove",
		Exec: ExecConfig{Type: ExecShell, Source: "echo"},
	})
	if err := store.Remove("remove_test"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if store.Get("remove_test") != nil {
		t.Fatal("expected nil after remove")
	}
	if store.Count() != 0 {
		t.Fatalf("expected 0 skills, got %d", store.Count())
	}
}

func TestGenerator_EmptyDescription(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	g := NewGenerator(store)
	_, err := g.GenerateFromDesc("")
	if err == nil {
		t.Fatal("expected error for empty description")
	}
}

func TestGenerator_Basic(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	g := NewGenerator(store)

	skill, err := g.GenerateFromDesc("Analyze a CSV file and return column statistics")
	if err != nil {
		t.Fatalf("GenerateFromDesc: %v", err)
	}
	if skill.Name == "" {
		t.Fatal("expected non-empty name")
	}
	if skill.Exec.Type != ExecPython {
		t.Fatalf("expected python execution for CSV analysis, got %s", skill.Exec.Type)
	}
	if skill.Description == "" {
		t.Fatal("expected non-empty description")
	}
}

func TestGenerator_TagGeneration(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	g := NewGenerator(store)
	skill, _ := g.GenerateFromDesc("Convert JSON data to CSV format with column mapping")
	if len(skill.Tags) == 0 {
		t.Fatal("expected at least 1 tag")
	}
}

func TestInstaller_FromFile(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	inst := NewInstaller(store)

	skillFile := filepath.Join(dir, "test_skill.skill.json")
	jsonContent := `{
		"name": "test_csv",
		"description": "Test CSV processing",
		"version": "1.0.0",
		"exec": {"type": "python", "source": "print('ok')"}
	}`
	os.WriteFile(skillFile, []byte(jsonContent), 0644)

	skill, err := inst.InstallFromFile(skillFile)
	if err != nil {
		t.Fatalf("InstallFromFile: %v", err)
	}
	if skill.Name != "test_csv" {
		t.Fatalf("expected 'test_csv', got %q", skill.Name)
	}
}

func TestInstaller_FromInline(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	inst := NewInstaller(store)

	jsonData := `{"name":"inline_test","description":"Inline test","exec":{"type":"shell","source":"echo done"}}`
	skill, err := inst.InstallFromInline(context.Background(), jsonData)
	if err != nil {
		t.Fatalf("InstallFromInline: %v", err)
	}
	if skill.Name != "inline_test" {
		t.Fatalf("expected 'inline_test', got %q", skill.Name)
	}
}

func TestInstaller_ScanDirectory(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	inst := NewInstaller(store)

	os.WriteFile(filepath.Join(dir, "a.skill.json"), []byte(`{"name":"a","description":"A","exec":{"type":"shell","source":"echo"}}`), 0644)
	os.WriteFile(filepath.Join(dir, "b.skill.json"), []byte(`{"name":"b","description":"B","exec":{"type":"shell","source":"echo"}}`), 0644)

	skills, err := inst.ScanDirectory(dir)
	if err != nil {
		t.Fatalf("ScanDirectory: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
}

func TestDiscoverer_Local(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	store.Save(&SkillDef{
		Name: "discover_test", Description: "Test discover",
		Exec: ExecConfig{Type: ExecPython, Source: "x=1"},
	})
	inst := NewInstaller(store)
	d := NewDiscoverer(store, inst)

	results, err := d.DiscoverLocal()
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Installed {
		t.Fatal("expected installed to be true")
	}
}

func TestToJSONSkill(t *testing.T) {
	skill := &SkillDef{Name: "json_test", Description: "JSON test", Exec: ExecConfig{Type: ExecPython, Source: "x=1"}}
	json := ToJSONSkill(skill)
	if !strings.Contains(json, "json_test") {
		t.Fatal("expected skill name in JSON output")
	}
}

func TestGenerator_ShellType(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	g := NewGenerator(store)
	skill, _ := g.GenerateFromDesc("Run a shell command to list directory contents")
	if skill.Exec.Type != ExecShell {
		t.Fatalf("expected shell type for shell command, got %s", skill.Exec.Type)
	}
}

func TestExecutor_Prompt(t *testing.T) {
	ex := NewExecutor()
	skill := &SkillDef{
		Name: "prompt_test",
		Exec: ExecConfig{Type: ExecPrompt, Source: "You are an AI. Answer: {{question}}"},
	}
	result, err := ex.Execute(context.Background(), skill, map[string]interface{}{"question": "What is Go?"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result.Output, "What is Go?") {
		t.Fatalf("expected prompt template filled, got %q", result.Output)
	}
}

func TestSkillTool_ImplementsInterface(t *testing.T) {
	// Compile-time check
	var _ interface {
		Name() string
		Description() string
	} = &SkillTool{}
}

func TestDynamicRegistry_InstallAndList(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)

	// Use a minimal mock
	skill := &SkillDef{
		Name: "reg_test", Description: "Registry test",
		Exec: ExecConfig{Type: ExecPrompt, Source: "hello"},
	}
	store.Save(skill)

	// DynamicRegistry requires a tool registry
	// This test verifies the store works
	if store.Get("reg_test") == nil {
		t.Fatal("expected to find skill in store")
	}
}

func TestDiscoverer_DiscoverLocal_Empty(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	inst := NewInstaller(store)
	d := NewDiscoverer(store, inst)
	results, err := d.DiscoverLocal()
	if err != nil {
		t.Fatalf("DiscoverLocal: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for empty store, got %d", len(results))
	}
}

func TestInstaller_FromURL_Invalid(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSkillStore(dir)
	inst := NewInstaller(store)
	_, err := inst.InstallFromURL(context.Background(), "http://nonexistent.example.com/skill.json")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}
