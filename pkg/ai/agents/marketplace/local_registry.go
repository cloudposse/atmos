package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// LocalRegistry manages installed community agents in ~/.atmos/agents/registry.json.
type LocalRegistry struct {
	Version string                     `json:"version"`
	Agents  map[string]*InstalledAgent `json:"agents"`
	mu      sync.RWMutex
	path    string
}

// InstalledAgent represents an installed community agent.
type InstalledAgent struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Source      string    `json:"source"`  // e.g., "github.com/user/repo".
	Version     string    `json:"version"` // e.g., "1.2.3".
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Path        string    `json:"path"`       // Absolute path to agent directory.
	IsBuiltIn   bool      `json:"is_builtin"` // Always false for community agents.
	Enabled     bool      `json:"enabled"`    // Can disable without uninstalling.
}

// NewLocalRegistry creates or loads the local agent registry.
func NewLocalRegistry() (*LocalRegistry, error) {
	defer perf.Track(nil, "marketplace.NewLocalRegistry")()

	registryPath, err := getRegistryPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get registry path: %w", err)
	}

	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
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

// Add registers a newly installed agent.
func (r *LocalRegistry) Add(agent *InstalledAgent) error {
	defer perf.Track(nil, "marketplace.LocalRegistry.Add")()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for name conflicts.
	if _, exists := r.Agents[agent.Name]; exists {
		return fmt.Errorf("%w: %q", ErrAgentAlreadyInstalled, agent.Name)
	}

	r.Agents[agent.Name] = agent
	return r.save()
}

// Remove unregisters an agent.
func (r *LocalRegistry) Remove(name string) error {
	defer perf.Track(nil, "marketplace.LocalRegistry.Remove")()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.Agents[name]; !exists {
		return fmt.Errorf("%w: %q", ErrAgentNotFound, name)
	}

	delete(r.Agents, name)
	return r.save()
}

// Get retrieves an installed agent by name.
func (r *LocalRegistry) Get(name string) (*InstalledAgent, error) {
	defer perf.Track(nil, "marketplace.LocalRegistry.Get")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.Agents[name]
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrAgentNotFound, name)
	}

	return agent, nil
}

// List returns all installed community agents, sorted alphabetically by name.
func (r *LocalRegistry) List() []*InstalledAgent {
	defer perf.Track(nil, "marketplace.LocalRegistry.List")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*InstalledAgent, 0, len(r.Agents))
	for _, agent := range r.Agents {
		agents = append(agents, agent)
	}

	// Sort alphabetically by name.
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})

	return agents
}

// Update updates an existing agent's information.
func (r *LocalRegistry) Update(name string, updater func(*InstalledAgent) error) error {
	defer perf.Track(nil, "marketplace.LocalRegistry.Update")()

	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.Agents[name]
	if !exists {
		return fmt.Errorf("%w: %q", ErrAgentNotFound, name)
	}

	if err := updater(agent); err != nil {
		return err
	}

	agent.UpdatedAt = time.Now()
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".atmos", "agents", "registry.json"), nil
}

// GetAgentsDir returns the directory where agents are installed.
func GetAgentsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".atmos", "agents"), nil
}
