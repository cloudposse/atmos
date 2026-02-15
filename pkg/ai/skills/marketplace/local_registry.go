package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
)

// LocalRegistry manages installed community skills in ~/.atmos/skills/registry.json.
type LocalRegistry struct {
	Version string                     `json:"version"`
	Skills  map[string]*InstalledSkill `json:"skills"`
	mu      sync.RWMutex
	path    string
}

// InstalledSkill represents an installed community skill.
type InstalledSkill struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Source      string    `json:"source"`  // e.g., "github.com/user/repo".
	Version     string    `json:"version"` // e.g., "1.2.3".
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Path        string    `json:"path"`       // Absolute path to skill directory.
	IsBuiltIn   bool      `json:"is_builtin"` // Always false for community skills.
	Enabled     bool      `json:"enabled"`    // Can disable without uninstalling.
}

// NewLocalRegistry creates or loads the local skill registry.
func NewLocalRegistry() (*LocalRegistry, error) {
	defer perf.Track(nil, "marketplace.NewLocalRegistry")()

	registryPath, err := getRegistryPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get registry path: %w", err)
	}

	registry := &LocalRegistry{
		Version: "1.0.0",
		Skills:  make(map[string]*InstalledSkill),
		path:    registryPath,
	}

	// Load existing registry if it exists.
	if _, err := os.Stat(registryPath); err == nil {
		if err := registry.load(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrRegistryCorrupted, err)
		}
	} else {
		// Create new registry file.
		if err := registry.save(); err != nil {
			return nil, fmt.Errorf("failed to create registry: %w", err)
		}
	}

	return registry, nil
}

// Add registers a newly installed skill.
func (r *LocalRegistry) Add(skill *InstalledSkill) error {
	defer perf.Track(nil, "marketplace.LocalRegistry.Add")()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for name conflicts.
	if _, exists := r.Skills[skill.Name]; exists {
		return fmt.Errorf("%w: %q", ErrSkillAlreadyInstalled, skill.Name)
	}

	r.Skills[skill.Name] = skill
	return r.save()
}

// Remove unregisters a skill.
func (r *LocalRegistry) Remove(name string) error {
	defer perf.Track(nil, "marketplace.LocalRegistry.Remove")()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.Skills[name]; !exists {
		return fmt.Errorf("%w: %q", ErrSkillNotFound, name)
	}

	delete(r.Skills, name)
	return r.save()
}

// Get retrieves an installed skill by name.
func (r *LocalRegistry) Get(name string) (*InstalledSkill, error) {
	defer perf.Track(nil, "marketplace.LocalRegistry.Get")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, exists := r.Skills[name]
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrSkillNotFound, name)
	}

	return skill, nil
}

// List returns all installed community skills, sorted alphabetically by name.
func (r *LocalRegistry) List() []*InstalledSkill {
	defer perf.Track(nil, "marketplace.LocalRegistry.List")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	skills := make([]*InstalledSkill, 0, len(r.Skills))
	for _, skill := range r.Skills {
		skills = append(skills, skill)
	}

	// Sort alphabetically by name.
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills
}

// Update updates an existing skill's information.
func (r *LocalRegistry) Update(name string, updater func(*InstalledSkill) error) error {
	defer perf.Track(nil, "marketplace.LocalRegistry.Update")()

	r.mu.Lock()
	defer r.mu.Unlock()

	skill, exists := r.Skills[name]
	if !exists {
		return fmt.Errorf("%w: %q", ErrSkillNotFound, name)
	}

	if err := updater(skill); err != nil {
		return err
	}

	skill.UpdatedAt = time.Now()
	return r.save()
}

// load reads the registry from disk.
func (r *LocalRegistry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return fmt.Errorf("failed to read registry file: %w", err)
	}

	if err := json.Unmarshal(data, r); err != nil {
		return fmt.Errorf("failed to parse registry JSON: %w", err)
	}

	return nil
}

// save writes the registry to disk.
func (r *LocalRegistry) save() error {
	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	// Marshal to JSON with indentation.
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry: %w", err)
	}

	// Write to file.
	if err := os.WriteFile(r.path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write registry file: %w", err)
	}

	return nil
}

// getRegistryPath returns the path to the local registry file.
func getRegistryPath() (string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".atmos", "skills", "registry.json"), nil
}

// GetSkillsDir returns the directory where skills are installed.
func GetSkillsDir() (string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".atmos", "skills"), nil
}
