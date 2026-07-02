package skill

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SkillStore manages installed skills with disk persistence.
// Skills are stored as individual JSON files in a designated directory.
type SkillStore struct {
	mu       sync.RWMutex
	dir      string
	skills   map[string]*SkillDef // name → skill
}

// NewSkillStore creates a store backed by the given directory.
// If the directory doesn't exist, it is created.
func NewSkillStore(dir string) (*SkillStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create skill dir: %w", err)
	}
	s := &SkillStore{
		dir:    dir,
		skills: make(map[string]*SkillDef),
	}
	if err := s.loadAll(); err != nil {
		slog.Warn("skill store: failed to load some skills", "error", err)
	}
	return s, nil
}

// List returns all installed skills.
func (s *SkillStore) List() []*SkillDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := make([]*SkillDef, 0, len(s.skills))
	for _, sk := range s.skills {
		list = append(list, sk)
	}
	return list
}

// Get retrieves a skill by name. Returns nil if not found.
func (s *SkillStore) Get(name string) *SkillDef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skills[name]
}

// Save persists a skill definition to disk and adds it to the in-memory map.
func (s *SkillStore) Save(skill *SkillDef) error {
	if err := skill.Validate(); err != nil {
		return fmt.Errorf("save skill: %w", err)
	}

	data, err := json.MarshalIndent(skill, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal skill: %w", err)
	}

	filePath := filepath.Join(s.dir, skill.Filename())
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write skill file: %w", err)
	}

	s.mu.Lock()
	s.skills[skill.Name] = skill
	s.mu.Unlock()

	slog.Info("skill saved", "name", skill.Name, "file", filePath)
	return nil
}

// Remove deletes a skill from disk and the in-memory map.
func (s *SkillStore) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	skill, ok := s.skills[name]
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}

	filePath := filepath.Join(s.dir, skill.Filename())
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove skill file: %w", err)
	}

	delete(s.skills, name)
	slog.Info("skill removed", "name", name)
	return nil
}

// Count returns the number of installed skills.
func (s *SkillStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.skills)
}

// loadAll scans the skill directory and loads all .skill.json files.
func (s *SkillStore) loadAll() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read skill dir: %w", err)
	}

	loaded := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".skill.json") {
			continue
		}
		filePath := filepath.Join(s.dir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			slog.Warn("skill store: read file", "file", filePath, "error", err)
			continue
		}
		var skill SkillDef
		if err := json.Unmarshal(data, &skill); err != nil {
			slog.Warn("skill store: parse skill", "file", filePath, "error", err)
			continue
		}
		if skill.Name == "" {
			slog.Warn("skill store: skill with no name", "file", filePath)
			continue
		}
		s.skills[skill.Name] = &skill
		loaded++
	}

	slog.Info("skill store loaded", "count", loaded, "dir", s.dir)
	return nil
}
