package marketplace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalRegistry_AddGetRemove(t *testing.T) {
	// Create temporary directory for registry.
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	// Create registry.
	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Add agent.
	agent := &InstalledAgent{
		Name:        "test-agent",
		DisplayName: "Test Agent",
		Source:      "github.com/test/agent",
		Version:     "1.0.0",
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        "/path/to/agent",
		IsBuiltIn:   false,
		Enabled:     true,
	}

	err := registry.Add(agent)
	require.NoError(t, err)

	// Verify registry file was created.
	_, err = os.Stat(registryPath)
	require.NoError(t, err)

	// Get agent.
	retrieved, err := registry.Get("test-agent")
	require.NoError(t, err)
	assert.Equal(t, agent.Name, retrieved.Name)
	assert.Equal(t, agent.DisplayName, retrieved.DisplayName)
	assert.Equal(t, agent.Source, retrieved.Source)
	assert.Equal(t, agent.Version, retrieved.Version)

	// Remove agent.
	err = registry.Remove("test-agent")
	require.NoError(t, err)

	// Verify agent is gone.
	_, err = registry.Get("test-agent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

func TestLocalRegistry_AddDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	agent := &InstalledAgent{
		Name:        "test-agent",
		DisplayName: "Test Agent",
		Source:      "github.com/test/agent",
		Version:     "1.0.0",
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        "/path/to/agent",
		Enabled:     true,
	}

	// Add agent first time.
	err := registry.Add(agent)
	require.NoError(t, err)

	// Try to add same agent again.
	err = registry.Add(agent)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentAlreadyInstalled)
}

func TestLocalRegistry_List(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Add multiple agents.
	agents := []*InstalledAgent{
		{
			Name:        "agent-c",
			DisplayName: "Agent C",
			Source:      "github.com/test/c",
			Version:     "1.0.0",
			InstalledAt: time.Now(),
			UpdatedAt:   time.Now(),
			Path:        "/path/to/c",
			Enabled:     true,
		},
		{
			Name:        "agent-a",
			DisplayName: "Agent A",
			Source:      "github.com/test/a",
			Version:     "1.0.0",
			InstalledAt: time.Now(),
			UpdatedAt:   time.Now(),
			Path:        "/path/to/a",
			Enabled:     true,
		},
		{
			Name:        "agent-b",
			DisplayName: "Agent B",
			Source:      "github.com/test/b",
			Version:     "1.0.0",
			InstalledAt: time.Now(),
			UpdatedAt:   time.Now(),
			Path:        "/path/to/b",
			Enabled:     true,
		},
	}

	for _, agent := range agents {
		err := registry.Add(agent)
		require.NoError(t, err)
	}

	// List agents.
	list := registry.List()
	require.Len(t, list, 3)

	// Verify alphabetical sorting.
	assert.Equal(t, "agent-a", list[0].Name)
	assert.Equal(t, "agent-b", list[1].Name)
	assert.Equal(t, "agent-c", list[2].Name)
}

func TestLocalRegistry_Update(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Add agent.
	agent := &InstalledAgent{
		Name:        "test-agent",
		DisplayName: "Test Agent",
		Source:      "github.com/test/agent",
		Version:     "1.0.0",
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        "/path/to/agent",
		Enabled:     true,
	}

	err := registry.Add(agent)
	require.NoError(t, err)

	// Update agent.
	err = registry.Update("test-agent", func(a *InstalledAgent) error {
		a.Version = "2.0.0"
		a.Enabled = false
		return nil
	})
	require.NoError(t, err)

	// Verify update.
	retrieved, err := registry.Get("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", retrieved.Version)
	assert.False(t, retrieved.Enabled)
}

func TestLocalRegistry_PersistenceAcrossInstances(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	// Create first registry instance and add agent.
	registry1 := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	agent := &InstalledAgent{
		Name:        "test-agent",
		DisplayName: "Test Agent",
		Source:      "github.com/test/agent",
		Version:     "1.0.0",
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        "/path/to/agent",
		Enabled:     true,
	}

	err := registry1.Add(agent)
	require.NoError(t, err)

	// Create second registry instance (simulates restart).
	registry2 := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Load from disk.
	err = registry2.load()
	require.NoError(t, err)

	// Verify agent persisted.
	retrieved, err := registry2.Get("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", retrieved.Name)
	assert.Equal(t, "Test Agent", retrieved.DisplayName)
}

func TestLocalRegistry_RemoveNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "registry.json")

	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	err := registry.Remove("nonexistent-agent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}
