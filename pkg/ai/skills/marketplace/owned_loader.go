package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/ai/skills"
)

// LoadProjectSkills loads recorded project installations before user skills.
func (i *Installer) LoadProjectSkills(registry *skills.Registry, base string) error {
	if base == "" {
		base = "."
	}
	absolute, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	return loadOwnedSkills(registry, filepath.Join(absolute, ".atmos", "skills"))
}

//nolint:revive // Keep each validation and best-effort loading error adjacent to the corresponding read.
func loadOwnedSkills(registry *skills.Registry, dir string) error {
	base, dir, err := ownedSkillRoot(dir)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "installations.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state struct {
		Version int                                   `json:"version"`
		Records []struct{ Name, Client, Path string } `json:"records"`
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return err
	}
	if state.Version != 1 {
		return fmt.Errorf("%w: unsupported installation state version %d", ErrInvalidMetadata, state.Version)
	}
	for _, record := range state.Records {
		if record.Client != "atmos" {
			continue
		}
		expected := filepath.Join(dir, "content", record.Name)
		if !validOwnedName(record.Name) || record.Path != expected {
			return ErrInvalidMetadata
		}
		if err = validateOwnedPath(base, filepath.Join(expected, "SKILL.md")); err != nil {
			return err
		}
		metadata, err := ParseSkillMetadata(filepath.Join(expected, "SKILL.md"))
		if err != nil {
			log.Warnf("Failed to load installed skill %q: %v", record.Name, err)
			continue
		}
		prompt, err := readSkillPromptWithReferences(expected, metadata)
		if err != nil {
			return err
		}
		skill := &skills.Skill{Name: metadata.Name, DisplayName: metadata.GetDisplayName(), Description: metadata.Description, SystemPrompt: prompt, Category: metadata.GetCategory(), AllowedTools: metadata.AllowedTools, RestrictedTools: metadata.RestrictedTools}
		if err = registry.Register(skill); err != nil {
			log.Warnf("Shadowed installed skill %q: %v", skill.Name, err)
		}
	}
	return nil
}

func validateOwnedPath(base, path string) error {
	for p := path; p != base; p = filepath.Dir(p) {
		if filepath.Dir(p) == p {
			return ErrInvalidMetadata
		}
		info, err := os.Lstat(p)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return ErrInvalidMetadata
		}
	}
	return nil
}

func ownedSkillRoot(dir string) (string, string, error) {
	base, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(dir)))
	if err != nil {
		return "", "", err
	}
	dir = filepath.Join(base, ".atmos", "skills")
	if err = validateOwnedPath(base, filepath.Join(dir, "installations.json")); err != nil {
		return "", "", err
	}

	return base, dir, nil
}

func validOwnedName(name string) bool {
	return name != "" && filepath.Base(name) == name && name != "." && name != ".."
}
