package ai

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetMemoryFilePath(t *testing.T) {
	// Use t.TempDir() for a cross-platform base path.
	basePath := t.TempDir()

	tests := []struct {
		name         string
		basePath     string
		filePath     string
		expectedPath string
	}{
		{
			name:         "default file path",
			basePath:     basePath,
			filePath:     "",
			expectedPath: filepath.Join(basePath, "ATMOS.md"),
		},
		{
			name:         "custom relative path",
			basePath:     basePath,
			filePath:     filepath.Join("docs", "MEMORY.md"),
			expectedPath: filepath.Join(basePath, "docs", "MEMORY.md"),
		},
		{
			name:         "nested relative path",
			basePath:     basePath,
			filePath:     filepath.Join(".config", "ai", "memory.md"),
			expectedPath: filepath.Join(basePath, ".config", "ai", "memory.md"),
		},
		{
			name:         "custom filename",
			basePath:     basePath,
			filePath:     "PROJECT_MEMORY.md",
			expectedPath: filepath.Join(basePath, "PROJECT_MEMORY.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: tt.basePath,
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Memory: schema.AIMemorySettings{
							FilePath: tt.filePath,
						},
					},
				},
			}
			result := getMemoryFilePath(atmosConfig)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

func TestGetMemoryFilePath_AbsolutePath(t *testing.T) {
	// Test with absolute path - use a platform-appropriate absolute path.
	basePath := t.TempDir()
	absolutePath := filepath.Join(basePath, "absolute", "ATMOS.md")

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Memory: schema.AIMemorySettings{
					FilePath: absolutePath,
				},
			},
		},
	}

	result := getMemoryFilePath(atmosConfig)
	// When an absolute path is provided, it should be returned as-is.
	assert.Equal(t, absolutePath, result)
}

func TestMemoryCommand_BasicProperties(t *testing.T) {
	t.Run("memory command properties", func(t *testing.T) {
		assert.Equal(t, "memory", memoryCmd.Use)
		assert.Equal(t, "Manage AI project memory", memoryCmd.Short)
		assert.NotEmpty(t, memoryCmd.Long)
		assert.NotNil(t, memoryCmd.RunE)
	})

	t.Run("memory init command properties", func(t *testing.T) {
		assert.Equal(t, "init", memoryInitCmd.Use)
		assert.Equal(t, "Initialize project memory file", memoryInitCmd.Short)
		assert.NotEmpty(t, memoryInitCmd.Long)
		assert.NotNil(t, memoryInitCmd.RunE)
	})

	t.Run("memory show command properties", func(t *testing.T) {
		assert.Equal(t, "show", memoryShowCmd.Use)
		assert.Equal(t, "Display project memory content", memoryShowCmd.Short)
		assert.NotEmpty(t, memoryShowCmd.Long)
		assert.NotNil(t, memoryShowCmd.RunE)
	})

	t.Run("memory validate command properties", func(t *testing.T) {
		assert.Equal(t, "validate", memoryValidateCmd.Use)
		assert.Equal(t, "Validate project memory file", memoryValidateCmd.Short)
		assert.NotEmpty(t, memoryValidateCmd.Long)
		assert.NotNil(t, memoryValidateCmd.RunE)
	})

	t.Run("memory edit command properties", func(t *testing.T) {
		assert.Equal(t, "edit", memoryEditCmd.Use)
		assert.Equal(t, "Edit project memory in your editor", memoryEditCmd.Short)
		assert.NotEmpty(t, memoryEditCmd.Long)
		assert.NotNil(t, memoryEditCmd.RunE)
	})

	t.Run("memory path command properties", func(t *testing.T) {
		assert.Equal(t, "path", memoryPathCmd.Use)
		assert.Equal(t, "Show project memory file path", memoryPathCmd.Short)
		assert.NotEmpty(t, memoryPathCmd.Long)
		assert.NotNil(t, memoryPathCmd.RunE)
	})
}

func TestMemoryCommand_Flags(t *testing.T) {
	t.Run("init command has force flag", func(t *testing.T) {
		forceFlag := memoryInitCmd.Flags().Lookup("force")
		require.NotNil(t, forceFlag, "force flag should be registered")
		assert.Equal(t, "bool", forceFlag.Value.Type())
		assert.Equal(t, "false", forceFlag.DefValue)
	})
}

func TestMemoryCommand_Subcommands(t *testing.T) {
	t.Run("memory command has expected subcommands", func(t *testing.T) {
		subcommands := memoryCmd.Commands()
		subcommandNames := make(map[string]bool)
		for _, subcmd := range subcommands {
			subcommandNames[subcmd.Name()] = true
		}

		expectedSubcommands := []string{"init", "show", "validate", "edit", "path"}
		for _, expected := range expectedSubcommands {
			assert.True(t, subcommandNames[expected], "expected subcommand %s not found", expected)
		}
	})
}

func TestMemoryCommand_ErrorCases(t *testing.T) {
	t.Run("initMemoryCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "init",
			RunE: initMemoryCommand,
		}
		testCmd.Flags().Bool("force", false, "Force overwrite")

		err := initMemoryCommand(testCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("showMemoryCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")
		err := showMemoryCommand(memoryShowCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("validateMemoryCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")
		err := validateMemoryCommand(memoryValidateCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("editMemoryCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")
		err := editMemoryCommand(memoryEditCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("pathMemoryCommand returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")
		err := pathMemoryCommand(memoryPathCmd, []string{})
		assert.Error(t, err)
	})
}

func TestMemoryCommand_LongDescriptions(t *testing.T) {
	t.Run("memory command has descriptive long text", func(t *testing.T) {
		assert.Contains(t, memoryCmd.Long, "ATMOS.md")
		assert.Contains(t, memoryCmd.Long, "project memory")
	})

	t.Run("init command describes template", func(t *testing.T) {
		assert.Contains(t, memoryInitCmd.Long, "template")
		assert.Contains(t, memoryInitCmd.Long, "ATMOS.md")
	})

	t.Run("show command describes formatted content", func(t *testing.T) {
		assert.Contains(t, memoryShowCmd.Long, "memory content")
		assert.Contains(t, memoryShowCmd.Long, "AI assistant")
	})

	t.Run("validate command describes checks", func(t *testing.T) {
		assert.Contains(t, memoryValidateCmd.Long, "Validate")
		assert.Contains(t, memoryValidateCmd.Long, "format")
	})

	t.Run("edit command describes editor usage", func(t *testing.T) {
		assert.Contains(t, memoryEditCmd.Long, "editor")
		assert.Contains(t, memoryEditCmd.Long, "EDITOR")
	})

	t.Run("path command describes output", func(t *testing.T) {
		assert.Contains(t, memoryPathCmd.Long, "path")
		assert.Contains(t, memoryPathCmd.Long, "ATMOS.md")
	})
}

func TestMemoryCommand_CommandHierarchy(t *testing.T) {
	t.Run("memory command is attached to ai command", func(t *testing.T) {
		// Check that memory command has ai as parent.
		parent := memoryCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "ai", parent.Name())
	})

	t.Run("init command is attached to memory command", func(t *testing.T) {
		parent := memoryInitCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "memory", parent.Name())
	})

	t.Run("show command is attached to memory command", func(t *testing.T) {
		parent := memoryShowCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "memory", parent.Name())
	})

	t.Run("validate command is attached to memory command", func(t *testing.T) {
		parent := memoryValidateCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "memory", parent.Name())
	})

	t.Run("edit command is attached to memory command", func(t *testing.T) {
		parent := memoryEditCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "memory", parent.Name())
	})

	t.Run("path command is attached to memory command", func(t *testing.T) {
		parent := memoryPathCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "memory", parent.Name())
	})
}
