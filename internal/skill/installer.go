package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Installer handles skill installation from various sources.
type Installer struct {
	store *SkillStore
	client *http.Client
}

func NewInstaller(store *SkillStore) *Installer {
	return &Installer{
		store:  store,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// InstallFromURL downloads a skill definition from a URL and installs it.
func (inst *Installer) InstallFromURL(ctx context.Context, url string) (*SkillDef, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("install from url: %w", err)
	}

	resp, err := inst.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download skill: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download skill: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read skill: %w", err)
	}

	var skill SkillDef
	if err := json.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("parse skill: %w", err)
	}

	skill.Source = url
	skill.InstalledAt = time.Now()

	if err := inst.store.Save(&skill); err != nil {
		return nil, fmt.Errorf("save skill: %w", err)
	}

	return &skill, nil
}

// InstallFromFile loads a skill definition from a local JSON file.
func (inst *Installer) InstallFromFile(path string) (*SkillDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read skill file: %w", err)
	}

	var skill SkillDef
	if err := json.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("parse skill file: %w", err)
	}

	skill.Source = path
	skill.InstalledAt = time.Now()

	if err := inst.store.Save(&skill); err != nil {
		return nil, fmt.Errorf("save skill: %w", err)
	}

	return &skill, nil
}

// InstallFromInline creates a skill from an inline definition (JSON string).
func (inst *Installer) InstallFromInline(ctx context.Context, jsonData string) (*SkillDef, error) {
	var skill SkillDef
	if err := json.Unmarshal([]byte(jsonData), &skill); err != nil {
		return nil, fmt.Errorf("parse inline skill: %w", err)
	}

	skill.Source = "inline"
	skill.InstalledAt = time.Now()

	if err := inst.store.Save(&skill); err != nil {
		return nil, fmt.Errorf("save skill: %w", err)
	}

	return &skill, nil
}

// DiscoverRemoteSkills fetches a skill index from a URL and returns available skills.
// The index is a JSON array of skill metadata (name, description, version, url).
func (inst *Installer) DiscoverRemoteSkills(ctx context.Context, indexURL string) ([]SkillDef, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("discover remote: %w", err)
	}

	resp, err := inst.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	// Index can be a JSON array of skill defs or a JSON array of {name, url} refs
	// Try direct parse as []SkillDef first
	var skills []SkillDef
	if err := json.Unmarshal(data, &skills); err != nil {
		// Try as []map with name/url
		var refs []map[string]string
		if err2 := json.Unmarshal(data, &refs); err2 != nil {
			return nil, fmt.Errorf("parse index: %w", err)
		}
		for _, ref := range refs {
			if url, ok := ref["url"]; ok {
				skill, dlErr := inst.InstallFromURL(ctx, url)
				if dlErr == nil {
					skills = append(skills, *skill)
				}
			}
		}
	}

	if skills == nil {
		skills = []SkillDef{}
	}
	return skills, nil
}

// ScanDirectory scans a directory for .skill.json files and returns metadata.
func (inst *Installer) ScanDirectory(dir string) ([]SkillDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scan dir: %w", err)
	}

	var skills []SkillDef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".skill.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var skill SkillDef
		if json.Unmarshal(data, &skill) == nil {
			skills = append(skills, skill)
		}
	}

	if skills == nil {
		skills = []SkillDef{}
	}
	return skills, nil
}
