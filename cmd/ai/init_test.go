package ai

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInitializeAIToolsAndExecutor_ToolsDisabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					Enabled: false,
				},
			},
		},
	}

	registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrAIToolsDisabled))
	assert.Nil(t, registry)
	assert.Nil(t, executor)
}

func TestInitializeAIToolsAndExecutor_ToolsEnabled(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					Enabled:             true,
					AllowedTools:        []string{"read_file"},
					RestrictedTools:     []string{"execute_bash_command"},
					BlockedTools:        []string{"dangerous_tool"},
					YOLOMode:            false,
					RequireConfirmation: boolPtr(true),
				},
			},
		},
	}

	registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, executor)
	// Registry should have registered tools.
	assert.Greater(t, registry.Count(), 0)
}

func TestInitializeAIToolsAndExecutor_YOLOMode(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					Enabled:  true,
					YOLOMode: true,
				},
			},
		},
	}

	registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, executor)
}

func TestInitializeAIToolsAndExecutor_WithToolLists(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name        string
		toolConfig  schema.AIToolSettings
		shouldError bool
	}{
		{
			name: "with allowed tools only",
			toolConfig: schema.AIToolSettings{
				Enabled:      true,
				AllowedTools: []string{"read_file", "list_files"},
			},
			shouldError: false,
		},
		{
			name: "with restricted tools only",
			toolConfig: schema.AIToolSettings{
				Enabled:         true,
				RestrictedTools: []string{"execute_bash_command"},
			},
			shouldError: false,
		},
		{
			name: "with blocked tools only",
			toolConfig: schema.AIToolSettings{
				Enabled:      true,
				BlockedTools: []string{"dangerous_tool"},
			},
			shouldError: false,
		},
		{
			name: "with all tool lists",
			toolConfig: schema.AIToolSettings{
				Enabled:         true,
				AllowedTools:    []string{"read_file"},
				RestrictedTools: []string{"write_file"},
				BlockedTools:    []string{"delete_file"},
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: tt.toolConfig,
					},
				},
			}

			registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, registry)
				assert.NotNil(t, executor)
			}
		})
	}
}

func TestInitializeAIToolsAndExecutor_RequireConfirmation(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name                string
		requireConfirmation *bool
	}{
		{
			name:                "require confirmation true",
			requireConfirmation: boolPtr(true),
		},
		{
			name:                "require confirmation false",
			requireConfirmation: boolPtr(false),
		},
		{
			name:                "require confirmation nil (default)",
			requireConfirmation: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							Enabled:             true,
							RequireConfirmation: tt.requireConfirmation,
						},
					},
				},
			}

			registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

			assert.NoError(t, err)
			assert.NotNil(t, registry)
			assert.NotNil(t, executor)
		})
	}
}

// TestInitializeAIToolsAndExecutor_PermissionCacheFailure tests the permission cache failure path.
// When permission.NewPermissionCache fails, the function should continue without a cache
// and use NewCLIPrompter instead of NewCLIPrompterWithCache.
func TestInitializeAIToolsAndExecutor_PermissionCacheFailure(t *testing.T) {
	// Create a file where the .atmos directory would be created.
	// This will cause os.MkdirAll to fail when trying to create .atmos directory.
	basePath := t.TempDir()

	// Create a file named ".atmos" to block the directory creation.
	atmosFilePath := filepath.Join(basePath, ".atmos")
	err := os.WriteFile(atmosFilePath, []byte("blocking file"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					Enabled:             true,
					RequireConfirmation: boolPtr(true),
				},
			},
		},
	}

	// The function should still succeed, but use NewCLIPrompter instead of
	// NewCLIPrompterWithCache due to the permission cache failure.
	registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, executor)
	assert.Greater(t, registry.Count(), 0)
}

// TestInitializeAIToolsAndExecutor_EmptyBasePath tests with an empty base path.
// This exercises the permission cache initialization with fallback to home directory.
func TestInitializeAIToolsAndExecutor_EmptyBasePath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "", // Empty base path - cache will use home directory.
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					Enabled:  true,
					YOLOMode: true, // Use YOLO mode to simplify testing.
				},
			},
		},
	}

	registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, executor)
}

// TestInitializeAIToolsAndExecutor_PermissionModes tests different permission mode configurations.
func TestInitializeAIToolsAndExecutor_PermissionModes(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name                string
		yoloMode            bool
		requireConfirmation *bool
		description         string
	}{
		{
			name:                "YOLO mode - no prompts",
			yoloMode:            true,
			requireConfirmation: nil,
			description:         "YOLO mode bypasses all permission checks",
		},
		{
			name:                "Prompt mode - explicit true",
			yoloMode:            false,
			requireConfirmation: boolPtr(true),
			description:         "Explicit require confirmation",
		},
		{
			name:                "Allow mode - explicit false",
			yoloMode:            false,
			requireConfirmation: boolPtr(false),
			description:         "Opt-out of prompting",
		},
		{
			name:                "Default prompt mode - nil",
			yoloMode:            false,
			requireConfirmation: nil,
			description:         "Default behavior - prompt for security",
		},
		{
			name:                "YOLO takes precedence over require confirmation true",
			yoloMode:            true,
			requireConfirmation: boolPtr(true),
			description:         "YOLO mode overrides RequireConfirmation=true",
		},
		{
			name:                "YOLO takes precedence over require confirmation false",
			yoloMode:            true,
			requireConfirmation: boolPtr(false),
			description:         "YOLO mode overrides RequireConfirmation=false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							Enabled:             true,
							YOLOMode:            tt.yoloMode,
							RequireConfirmation: tt.requireConfirmation,
						},
					},
				},
			}

			registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

			assert.NoError(t, err, tt.description)
			assert.NotNil(t, registry)
			assert.NotNil(t, executor)
		})
	}
}

// TestInitializeAIToolsAndExecutor_ToolRegistration tests that tools are properly registered.
func TestInitializeAIToolsAndExecutor_ToolRegistration(t *testing.T) {
	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				Tools: schema.AIToolSettings{
					Enabled: true,
				},
			},
		},
	}

	registry, executor, err := initializeAIToolsAndExecutor(atmosConfig)

	assert.NoError(t, err)
	assert.NotNil(t, registry)
	assert.NotNil(t, executor)

	// Verify multiple tools were registered.
	toolCount := registry.Count()
	assert.Greater(t, toolCount, 5, "Expected more than 5 tools to be registered")
}
