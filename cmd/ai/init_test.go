package ai

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

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
