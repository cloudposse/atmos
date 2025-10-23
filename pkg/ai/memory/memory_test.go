package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
		// Temp dir is automatically cleaned up by t.TempDir()
	}

	return manager, tmpDir, cleanup
}

func TestNewManager(t *testing.T) {
	t.Run("creates manager with config", func(t *testing.T) {
		config := &Config{
			Enabled:      true,
			FilePath:     "ATMOS.md",
			AutoUpdate:   true,
			CreateIfMiss: true,
			Sections:     []string{"project_context"},
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

func TestManager_CreateDefault(t *testing.T) {
	manager, tmpDir, cleanup := setupTestMemory(t, nil)
	defer cleanup()

	ctx := context.Background()

	err := manager.CreateDefault(ctx)
	require.NoError(t, err)

	// Check file was created.
	filePath := filepath.Join(tmpDir, "ATMOS.md")
	_, err = os.Stat(filePath)
	require.NoError(t, err, "ATMOS.md should exist")

	// Check content.
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "# Atmos Project Memory")
	assert.Contains(t, string(content), "## Project Context")
	assert.Contains(t, string(content), "## Common Commands")
}

func TestManager_Load(t *testing.T) {
	t.Run("loads existing file", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true // Enable memory loading

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		// Create a test ATMOS.md file.
		testContent := `# Atmos Project Memory

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

	t.Run("creates default when file missing and CreateIfMiss is true", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.CreateIfMiss = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		memory, err := manager.Load(ctx)

		require.NoError(t, err)
		assert.NotNil(t, memory)

		// Check file was created.
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		_, err = os.Stat(filePath)
		require.NoError(t, err)
	})

	t.Run("returns error when file missing and CreateIfMiss is false", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.CreateIfMiss = false

		manager, _, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		memory, err := manager.Load(ctx)

		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAIProjectMemoryNotFound)
		assert.Nil(t, memory)
	})

	t.Run("returns nil when disabled", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = false

		manager, _, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		memory, err := manager.Load(ctx)

		assert.NoError(t, err)
		assert.Nil(t, memory)
	})
}

func TestManager_GetContext(t *testing.T) {
	t.Run("returns formatted context", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.Sections = []string{"project_context", "common_commands"}

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

		context := manager.GetContext()

		assert.Contains(t, context, "# Project Memory")
		assert.Contains(t, context, "## Project Context")
		assert.Contains(t, context, "Org: acme-corp")
		assert.Contains(t, context, "## Common Commands")
		assert.Contains(t, context, "atmos validate stacks")
	})

	t.Run("returns empty when memory not loaded", func(t *testing.T) {
		manager, _, cleanup := setupTestMemory(t, nil)
		defer cleanup()

		context := manager.GetContext()
		assert.Empty(t, context)
	})

	t.Run("returns empty when disabled", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = false

		manager, _, cleanup := setupTestMemory(t, config)
		defer cleanup()

		context := manager.GetContext()
		assert.Empty(t, context)
	})

	t.Run("includes only configured sections", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.Sections = []string{"project_context"} // Only one section

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		testContent := `## Project Context

Context content.

## Common Commands

Commands content.

## Stack Patterns

Patterns content.
`
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		err := os.WriteFile(filePath, []byte(testContent), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Load(ctx)
		require.NoError(t, err)

		context := manager.GetContext()

		assert.Contains(t, context, "Context content")
		assert.NotContains(t, context, "Commands content")
		assert.NotContains(t, context, "Patterns content")
	})
}

func TestManager_UpdateSection(t *testing.T) {
	t.Run("updates existing section", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.AutoUpdate = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		// Load initial content.
		testContent := `## Project Context

Old content.
`
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		err := os.WriteFile(filePath, []byte(testContent), 0o644)
		require.NoError(t, err)

		ctx := context.Background()
		_, err = manager.Load(ctx)
		require.NoError(t, err)

		// Update section.
		err = manager.UpdateSection(ctx, "project_context", "New content.", false)
		require.NoError(t, err)

		// Check in-memory update.
		section := manager.memory.Sections["project_context"]
		assert.Equal(t, "New content.", section.Content)
	})

	t.Run("creates new section if doesn't exist", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.AutoUpdate = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()
		_ = tmpDir // Used for setup

		ctx := context.Background()
		err := manager.CreateDefault(ctx)
		require.NoError(t, err)

		_, err = manager.Load(ctx)
		require.NoError(t, err)

		// Add new section.
		err = manager.UpdateSection(ctx, "custom_section", "Custom content.", false)
		require.NoError(t, err)

		section := manager.memory.Sections["custom_section"]
		require.NotNil(t, section)
		assert.Equal(t, "Custom content.", section.Content)
	})

	t.Run("writes to disk when writeToDisk is true", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.AutoUpdate = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		err := manager.CreateDefault(ctx)
		require.NoError(t, err)

		_, err = manager.Load(ctx)
		require.NoError(t, err)

		// Update and save.
		err = manager.UpdateSection(ctx, "project_context", "Updated content.", true)
		require.NoError(t, err)

		// Read file and verify.
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)

		assert.Contains(t, string(content), "Updated content.")
	})

	t.Run("does nothing when disabled", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = false

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()
		_ = tmpDir // Used for setup

		ctx := context.Background()

		err := manager.UpdateSection(ctx, "project_context", "Content.", false)
		assert.NoError(t, err) // Should not error, just no-op
	})

	t.Run("does nothing when auto_update is false", func(t *testing.T) {
		config := DefaultConfig()
		config.AutoUpdate = false

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()
		_ = tmpDir // Used for setup

		ctx := context.Background()

		err := manager.UpdateSection(ctx, "project_context", "Content.", false)
		assert.NoError(t, err) // Should not error, just no-op
	})

	t.Run("returns error when memory not loaded", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true
		config.AutoUpdate = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()
		_ = tmpDir // Used for setup

		ctx := context.Background()

		err := manager.UpdateSection(ctx, "project_context", "Content.", false)
		assert.ErrorIs(t, err, errUtils.ErrAIProjectMemoryNotLoaded)
	})
}

func TestManager_Save(t *testing.T) {
	t.Run("saves memory to disk", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		err := manager.CreateDefault(ctx)
		require.NoError(t, err)

		_, err = manager.Load(ctx)
		require.NoError(t, err)

		// Modify section.
		manager.memory.Sections["project_context"].Content = "Modified content"

		// Save.
		err = manager.Save(ctx)
		require.NoError(t, err)

		// Read and verify.
		filePath := filepath.Join(tmpDir, "ATMOS.md")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)

		assert.Contains(t, string(content), "Modified content")
	})

	t.Run("updates last modified time", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()
		_ = tmpDir // Used for setup

		ctx := context.Background()
		err := manager.CreateDefault(ctx)
		require.NoError(t, err)

		_, err = manager.Load(ctx)
		require.NoError(t, err)

		oldTime := manager.memory.LastModified
		time.Sleep(10 * time.Millisecond) // Ensure time difference

		err = manager.Save(ctx)
		require.NoError(t, err)

		assert.True(t, manager.memory.LastModified.After(oldTime))
	})

	t.Run("returns error when memory not loaded", func(t *testing.T) {
		manager, tmpDir, cleanup := setupTestMemory(t, nil)
		defer cleanup()
		_ = tmpDir // Used for setup

		ctx := context.Background()

		err := manager.Save(ctx)
		assert.ErrorIs(t, err, errUtils.ErrAIProjectMemoryNotLoaded)
	})
}

func TestManager_Reload(t *testing.T) {
	t.Run("reloads when file modified", func(t *testing.T) {
		config := DefaultConfig()
		config.Enabled = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()

		ctx := context.Background()
		err := manager.CreateDefault(ctx)
		require.NoError(t, err)

		_, err = manager.Load(ctx)
		require.NoError(t, err)

		// Modify file externally.
		time.Sleep(10 * time.Millisecond) // Ensure time difference
		filePath := filepath.Join(tmpDir, "ATMOS.md")
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
		config := DefaultConfig()
		config.Enabled = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()
		_ = tmpDir // Used for setup

		ctx := context.Background()
		err := manager.CreateDefault(ctx)
		require.NoError(t, err)

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
		config := DefaultConfig()
		config.Enabled = true

		manager, tmpDir, cleanup := setupTestMemory(t, config)
		defer cleanup()
		_ = tmpDir // Used for setup

		// Create file.
		ctx := context.Background()
		err := manager.CreateDefault(ctx)
		require.NoError(t, err)

		// Reload without prior load.
		err = manager.Reload(ctx)
		require.NoError(t, err)

		assert.NotNil(t, manager.memory)
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.False(t, config.Enabled) // Disabled by default
	assert.Equal(t, "ATMOS.md", config.FilePath)
	assert.False(t, config.AutoUpdate)
	assert.True(t, config.CreateIfMiss)
	assert.NotEmpty(t, config.Sections)
	assert.Contains(t, config.Sections, "project_context")
	assert.Contains(t, config.Sections, "common_commands")
}

func TestGetDefaultTemplate(t *testing.T) {
	template := GetDefaultTemplate()

	assert.Contains(t, template, "# Atmos Project Memory")
	assert.Contains(t, template, "## Project Context")
	assert.Contains(t, template, "## Common Commands")
	assert.Contains(t, template, "## Stack Patterns")
	assert.Contains(t, template, "## Frequent Issues")
	assert.Contains(t, template, "## Infrastructure Patterns")
	assert.Contains(t, template, "## Component Catalog Structure")
	assert.Contains(t, template, "## Team Conventions")
	assert.Contains(t, template, "## Recent Learnings")

	// Check for placeholder content.
	assert.Contains(t, template, "your-org")
	assert.Contains(t, template, "atmos describe component")
}
