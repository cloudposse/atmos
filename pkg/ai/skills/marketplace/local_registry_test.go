package marketplace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
)

func TestNewLocalRegistry_CreatesNew(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	registry, err := NewLocalRegistry()

	require.NoError(t, err)
	assert.NotNil(t, registry)
	assert.Equal(t, "1.0.0", registry.Version)
	assert.Empty(t, registry.Skills)

	// Verify file was created on disk.
	registryPath := filepath.Join(tempDir, ".atmos", "skills", "registry.json")
	_, err = os.Stat(registryPath)
	assert.NoError(t, err)
}

func TestNewLocalRegistry_LoadsExisting(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a registry file on disk first.
	registryDir := filepath.Join(tempDir, ".atmos", "skills")
	err := os.MkdirAll(registryDir, 0o755)
	require.NoError(t, err)

	existing := &LocalRegistry{
		Version: "1.0.0",
		Skills: map[string]*InstalledSkill{
			"test-skill": {
				Name:        "test-skill",
				DisplayName: "Test Skill",
				Version:     "2.0.0",
				Enabled:     true,
			},
		},
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(registryDir, "registry.json"), data, 0o600)
	require.NoError(t, err)

	// Load it.
	registry, err := NewLocalRegistry()

	require.NoError(t, err)
	assert.Equal(t, 1, len(registry.Skills))
	assert.Equal(t, "Test Skill", registry.Skills["test-skill"].DisplayName)
}

func TestNewLocalRegistry_CorruptedFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create a corrupted registry file.
	registryDir := filepath.Join(tempDir, ".atmos", "skills")
	err := os.MkdirAll(registryDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(registryDir, "registry.json"), []byte("not json{{{"), 0o600)
	require.NoError(t, err)

	_, err = NewLocalRegistry()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registry file corrupted")
}

func TestLocalRegistry_Update_Success(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	registry, err := NewLocalRegistry()
	require.NoError(t, err)

	// Add a skill.
	err = registry.Add(&InstalledSkill{
		Name:    "update-test",
		Version: "1.0.0",
		Enabled: true,
	})
	require.NoError(t, err)

	// Update it.
	err = registry.Update("update-test", func(s *InstalledSkill) error {
		s.Version = "2.0.0"
		return nil
	})
	require.NoError(t, err)

	// Verify update.
	skill, err := registry.Get("update-test")
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", skill.Version)
	assert.False(t, skill.UpdatedAt.IsZero())
}

func TestLocalRegistry_Update_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	registry, err := NewLocalRegistry()
	require.NoError(t, err)

	err = registry.Update("nonexistent", func(s *InstalledSkill) error {
		return nil
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "skill not found")
}

func TestLocalRegistry_Update_UpdaterError(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	registry, err := NewLocalRegistry()
	require.NoError(t, err)

	err = registry.Add(&InstalledSkill{
		Name:    "updater-err-test",
		Version: "1.0.0",
	})
	require.NoError(t, err)

	err = registry.Update("updater-err-test", func(_ *InstalledSkill) error {
		return assert.AnError
	})

	assert.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestLocalRegistry_Add_Conflict(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	registry, err := NewLocalRegistry()
	require.NoError(t, err)

	err = registry.Add(&InstalledSkill{Name: "dup"})
	require.NoError(t, err)

	err = registry.Add(&InstalledSkill{Name: "dup"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
}

func TestLocalRegistry_Remove_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	registry, err := NewLocalRegistry()
	require.NoError(t, err)

	err = registry.Remove("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "skill not found")
}

func TestLocalRegistry_List_Sorted(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	registry, err := NewLocalRegistry()
	require.NoError(t, err)

	_ = registry.Add(&InstalledSkill{Name: "charlie"})
	_ = registry.Add(&InstalledSkill{Name: "alpha"})
	_ = registry.Add(&InstalledSkill{Name: "bravo"})

	list := registry.List()

	assert.Equal(t, 3, len(list))
	assert.Equal(t, "alpha", list[0].Name)
	assert.Equal(t, "bravo", list[1].Name)
	assert.Equal(t, "charlie", list[2].Name)
}

func TestLocalRegistry_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	// Create registry and add a skill.
	registry1, err := NewLocalRegistry()
	require.NoError(t, err)
	err = registry1.Add(&InstalledSkill{Name: "persist-test", Version: "1.0.0"})
	require.NoError(t, err)

	// Load a fresh registry and verify persistence.
	registry2, err := NewLocalRegistry()
	require.NoError(t, err)

	skill, err := registry2.Get("persist-test")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", skill.Version)
}

func TestGetSkillsDir(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	homedir.Reset()

	dir, err := GetSkillsDir()

	require.NoError(t, err)
	assert.Contains(t, dir, ".atmos")
	assert.Contains(t, dir, "skills")
}
