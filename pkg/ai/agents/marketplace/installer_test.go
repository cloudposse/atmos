package marketplace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDownloader mocks the Downloader by copying from local testdata instead of cloning.
type mockDownloader struct {
	testdataPath string
	t            *testing.T
}

func newMockDownloader(t *testing.T, testdataPath string) *mockDownloader {
	return &mockDownloader{
		testdataPath: testdataPath,
		t:            t,
	}
}

func (m *mockDownloader) Download(_ context.Context, source *SourceInfo) (string, error) {
	// Create temp directory using t.TempDir() for automatic cleanup.
	tempDir := m.t.TempDir()

	// Copy testdata to temp directory.
	srcPath := filepath.Join(m.testdataPath, source.Name)
	err := copyDir(srcPath, tempDir)
	if err != nil {
		return "", err
	}

	return tempDir, nil
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o600); err != nil {
				return err
			}
		}
	}

	return nil
}

func TestInstaller_Install_Success(t *testing.T) {
	// Create temporary directories.
	tmpHome := t.TempDir()
	agentsDir := filepath.Join(tmpHome, ".atmos", "agents")

	// Override HOME for this test.
	t.Setenv("HOME", tmpHome)

	// Set up test registry.
	registryPath := filepath.Join(agentsDir, "registry.json")
	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Create installer with mock downloader.
	testdataPath, _ := filepath.Abs("testdata")
	installer := &Installer{
		downloader:    newMockDownloader(t, testdataPath),
		validator:     NewValidator("1.50.0"),
		localRegistry: registry,
		atmosVersion:  "1.50.0",
	}

	// Install agent (skip confirmation).
	ctx := context.Background()
	opts := InstallOptions{
		Force:       false,
		SkipConfirm: true,
	}

	err := installer.Install(ctx, "github.com/test/valid-agent", opts)
	require.NoError(t, err)

	// Verify agent was registered.
	agent, err := registry.Get("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", agent.Name)
	assert.Equal(t, "Test Agent", agent.DisplayName)
	assert.Equal(t, "1.0.0", agent.Version)
	assert.True(t, agent.Enabled)
	assert.False(t, agent.IsBuiltIn)
}

func TestInstaller_Install_AlreadyInstalled(t *testing.T) {
	tmpHome := t.TempDir()
	agentsDir := filepath.Join(tmpHome, ".atmos", "agents")

	// Override HOME for this test.
	t.Setenv("HOME", tmpHome)

	registryPath := filepath.Join(agentsDir, "registry.json")
	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Pre-register agent.
	existingAgent := &InstalledAgent{
		Name:        "test-agent",
		DisplayName: "Test Agent",
		Source:      "github.com/test/valid-agent",
		Version:     "1.0.0",
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        "/some/path",
		Enabled:     true,
	}
	err := registry.Add(existingAgent)
	require.NoError(t, err)

	testdataPath, _ := filepath.Abs("testdata")
	installer := &Installer{
		downloader:    newMockDownloader(t, testdataPath),
		validator:     NewValidator("1.50.0"),
		localRegistry: registry,
		atmosVersion:  "1.50.0",
	}

	// Try to install again without force.
	ctx := context.Background()
	opts := InstallOptions{
		Force:       false,
		SkipConfirm: true,
	}

	err = installer.Install(ctx, "github.com/test/valid-agent", opts)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentAlreadyInstalled)
}

func TestInstaller_Install_ForceReinstall(t *testing.T) {
	tmpHome := t.TempDir()
	agentsDir := filepath.Join(tmpHome, ".atmos", "agents")

	// Override HOME for this test.
	t.Setenv("HOME", tmpHome)

	registryPath := filepath.Join(agentsDir, "registry.json")
	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Pre-register agent.
	existingAgent := &InstalledAgent{
		Name:        "test-agent",
		DisplayName: "Test Agent",
		Source:      "github.com/test/valid-agent",
		Version:     "0.9.0", // Old version.
		InstalledAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt:   time.Now().Add(-24 * time.Hour),
		Path:        filepath.Join(agentsDir, "github.com", "test", "valid-agent"),
		Enabled:     true,
	}
	err := registry.Add(existingAgent)
	require.NoError(t, err)

	testdataPath, _ := filepath.Abs("testdata")
	installer := &Installer{
		downloader:    newMockDownloader(t, testdataPath),
		validator:     NewValidator("1.50.0"),
		localRegistry: registry,
		atmosVersion:  "1.50.0",
	}

	// Force reinstall.
	ctx := context.Background()
	opts := InstallOptions{
		Force:       true,
		SkipConfirm: true,
	}

	err = installer.Install(ctx, "github.com/test/valid-agent", opts)
	require.NoError(t, err)

	// Verify agent was updated.
	agent, err := registry.Get("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", agent.Version) // Updated version.
}

func TestInstaller_Uninstall_Success(t *testing.T) {
	tmpHome := t.TempDir()
	agentsDir := filepath.Join(tmpHome, ".atmos", "agents")
	agentPath := filepath.Join(agentsDir, "test-agent")

	// Create agent directory.
	err := os.MkdirAll(agentPath, 0o755)
	require.NoError(t, err)

	registryPath := filepath.Join(agentsDir, "registry.json")
	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Register agent.
	agent := &InstalledAgent{
		Name:        "test-agent",
		DisplayName: "Test Agent",
		Source:      "github.com/test/agent",
		Version:     "1.0.0",
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
		Path:        agentPath,
		Enabled:     true,
	}
	err = registry.Add(agent)
	require.NoError(t, err)

	installer := &Installer{
		localRegistry: registry,
		atmosVersion:  "1.50.0",
	}

	// Uninstall (force to skip confirmation).
	err = installer.Uninstall("test-agent", true)
	require.NoError(t, err)

	// Verify agent is removed.
	_, err = registry.Get("test-agent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)

	// Verify directory is removed.
	_, err = os.Stat(agentPath)
	assert.True(t, os.IsNotExist(err))
}

func TestInstaller_List(t *testing.T) {
	tmpHome := t.TempDir()
	agentsDir := filepath.Join(tmpHome, ".atmos", "agents")

	registryPath := filepath.Join(agentsDir, "registry.json")
	registry := &LocalRegistry{
		Version: "1.0.0",
		Agents:  make(map[string]*InstalledAgent),
		path:    registryPath,
	}

	// Add multiple agents.
	agents := []*InstalledAgent{
		{
			Name:        "agent-1",
			DisplayName: "Agent 1",
			Source:      "github.com/test/1",
			Version:     "1.0.0",
			InstalledAt: time.Now(),
			UpdatedAt:   time.Now(),
			Path:        "/path/1",
			Enabled:     true,
		},
		{
			Name:        "agent-2",
			DisplayName: "Agent 2",
			Source:      "github.com/test/2",
			Version:     "2.0.0",
			InstalledAt: time.Now(),
			UpdatedAt:   time.Now(),
			Path:        "/path/2",
			Enabled:     true,
		},
	}

	for _, agent := range agents {
		err := registry.Add(agent)
		require.NoError(t, err)
	}

	installer := &Installer{
		localRegistry: registry,
		atmosVersion:  "1.50.0",
	}

	// List agents.
	list := installer.List()
	require.Len(t, list, 2)
	assert.Equal(t, "agent-1", list[0].Name)
	assert.Equal(t, "agent-2", list[1].Name)
}
