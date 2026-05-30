package instructions

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestMemory creates a temporary directory and memory manager for testing.
func setupTestMemory(t *testing.T, config *Config) (*Manager, string, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	if config == nil {
		config = DefaultConfig()
	}

	manager := NewManager(tmpDir, config)

	cleanup := func() {
		// Temp dir is automatically cleaned up by t.TempDir().
	}

	return manager, tmpDir, cleanup
}

func TestNewManager(t *testing.T) {
	t.Run("creates manager with config", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager := NewManager("/test/path", config)

		assert.NotNil(t, manager)
		assert.Equal(t, config, manager.config)
		assert.Equal(t, "/test/path", manager.basePath)
	})

	t.Run("uses default config when nil", func(t *testing.T) {
		manager := NewManager("/test/path", nil)

		assert.NotNil(t, manager)
		assert.NotNil(t, manager.config)
		assert.Equal(t, DefaultConfig().Enabled, manager.config.Enabled)
	})
}

func TestManager_Load(t *testing.T) {
	t.Run("loads existing file", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		// Create a test ATMOS.md file.
		testContent := `# Atmos Project Instructions

## Project Context

Test project context.

## Common Commands

Test commands.
`
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		err := os.WriteFile(filePath, []byte(testContent), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		memory, err := manager.Load(ctx)

		require.NoError(t, err)
		assert.NotNil(t, memory)
		assert.Equal(t, filePath, memory.FilePath)
		assert.Contains(t, memory.Content, "Test project context")
		assert.Len(t, memory.Sections, 2)
		assert.True(t, memory.Enabled)
	})

	t.Run("returns nil when file missing", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager, _, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		memory, err := manager.Load(ctx)

		assert.NoError(t, err)
		assert.Nil(t, memory)
	})

	t.Run("returns nil when disabled", func(t *testing.T) {
		config := &Config{
			Enabled:  false,
			FilePath: "ATMOS.md",
		}

		manager, _, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		memory, err := manager.Load(ctx)

		assert.NoError(t, err)
		assert.Nil(t, memory)
	})
}

func TestManager_GetContext(t *testing.T) {
	t.Run("returns full file content", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		// Create test file.
		testContent := `## Project Context

Org: acme-corp

## Common Commands

atmos validate stacks
`
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		err := os.WriteFile(filePath, []byte(testContent), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Load(ctx)
		require.NoError(t, err)

		result := manager.GetContext()

		assert.Contains(t, result, "# Project Instructions")
		assert.Contains(t, result, "Org: acme-corp")
		assert.Contains(t, result, "atmos validate stacks")
	})

	t.Run("returns empty when memory not loaded", func(t *testing.T) {
		manager, _, cleanup := setupTestMemory(t, nil)
		defer cleanup()

		result := manager.GetContext()
		assert.Empty(t, result)
	})

	t.Run("returns empty when disabled", func(t *testing.T) {
		config := &Config{
			Enabled:  false,
			FilePath: "ATMOS.md",
		}

		manager, _, cleanup := setupTestMemory(t, config)
		defer cleanup()

		result := manager.GetContext()
		assert.Empty(t, result)
	})
}

func TestManager_Reload(t *testing.T) {
	t.Run("reloads when file modified", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		// Create initial file.
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		initialContent := `## Project Context

Initial content.
`
		err := os.WriteFile(filePath, []byte(initialContent), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Load(ctx)
		require.NoError(t, err)

		// Modify file externally.
		time.Sleep(10 * time.Millisecond) // Ensure time difference.
		newContent := `## Project Context

Externally modified content.
`
		err = os.WriteFile(filePath, []byte(newContent), 0o644)
		require.NoError(t, err)

		// Reload.
		err = manager.Reload(ctx)
		require.NoError(t, err)

		// Verify new content is loaded.
		section := manager.memory.Sections["project_context"]
		assert.Contains(t, section.Content, "Externally modified")
	})

	t.Run("does not reload when file unchanged", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		filePath := filepath.Join(tmpDir, "ATMOS.md")
		err := os.WriteFile(filePath, []byte("## Project Context\n\nContent.\n"), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Load(ctx)
		require.NoError(t, err)

		oldTime := manager.memory.LastModified

		// Reload without changes.
		err = manager.Reload(ctx)
		require.NoError(t, err)

		// Time should be unchanged.
		assert.Equal(t, oldTime, manager.memory.LastModified)
	})

	t.Run("loads when memory not yet loaded", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		// Create file.
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		err := os.WriteFile(filePath, []byte("## Project Context\n\nContent.\n"), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		err = manager.Reload(ctx)
		require.NoError(t, err)

		assert.NotNil(t, manager.memory)
	})

	t.Run("clears memory when file deleted", func(t *testing.T) {
		config := &Config{
			Enabled:  true,
			FilePath: "ATMOS.md",
		}

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		filePath := filepath.Join(tmpDir, "ATMOS.md")
		err := os.WriteFile(filePath, []byte("## Project Context\n\nContent.\n"), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Load(ctx)
		require.NoError(t, err)
		assert.NotNil(t, manager.memory)

		// Delete file.
		err = os.Remove(filePath)
		require.NoError(t, err)

		// Reload should clear memory.
		err = manager.Reload(ctx)
		require.NoError(t, err)
		assert.Nil(t, manager.memory)
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.False(t, config.Enabled)
	assert.Equal(t, "ATMOS.md", config.FilePath)
}
