package ai

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// boolPtr returns a pointer to the given boolean value.
func boolPtr(b bool) *bool {
	return &b
}

func TestGetProviderFromConfig(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedResult string
	}{
		{
			name: "default provider when not configured",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expectedResult: "anthropic",
		},
		{
			name: "custom provider configured",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "openai",
					},
				},
			},
			expectedResult: "openai",
		},
		{
			name: "gemini provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "gemini",
					},
				},
			},
			expectedResult: "gemini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getProviderFromConfig(tt.atmosConfig)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGetModelFromConfig(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedResult string
	}{
		{
			name: "returns model from configured provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "anthropic",
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model:     "claude-sonnet-4-20250514",
								MaxTokens: 4096,
							},
						},
					},
				},
			},
			expectedResult: "claude-sonnet-4-20250514",
		},
		{
			name: "returns empty string when provider not found",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "openai",
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model:     "claude-sonnet-4-20250514",
								MaxTokens: 4096,
							},
						},
					},
				},
			},
			expectedResult: "",
		},
		{
			name: "returns empty string when no providers configured",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "anthropic",
					},
				},
			},
			expectedResult: "",
		},
		{
			name: "returns model for default provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model:     "claude-3-5-sonnet-20241022",
								MaxTokens: 8192,
							},
						},
					},
				},
			},
			expectedResult: "claude-3-5-sonnet-20241022",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getModelFromConfig(tt.atmosConfig)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGetSessionStoragePath(t *testing.T) {
	// Use t.TempDir() for cross-platform base paths.
	basePath := t.TempDir()

	t.Run("default path", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Path: "",
					},
				},
			},
		}
		result := getSessionStoragePath(atmosConfig)
		expected := filepath.Join(basePath, ".atmos", "sessions", "sessions.db")
		assert.Equal(t, expected, result)
	})

	t.Run("custom relative path", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Path: filepath.Join("data", "ai-sessions"),
					},
				},
			},
		}
		result := getSessionStoragePath(atmosConfig)
		expected := filepath.Join(basePath, "data", "ai-sessions", "sessions.db")
		assert.Equal(t, expected, result)
	})

	t.Run("absolute path", func(t *testing.T) {
		// Use another temp directory as the absolute path.
		absolutePath := t.TempDir()
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Path: absolutePath,
					},
				},
			},
		}
		result := getSessionStoragePath(atmosConfig)
		expected := filepath.Join(absolutePath, "sessions.db")
		assert.Equal(t, expected, result)
	})

	t.Run("path with nested directories", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Path: filepath.Join(".config", "atmos", "ai", "sessions"),
					},
				},
			},
		}
		result := getSessionStoragePath(atmosConfig)
		expected := filepath.Join(basePath, ".config", "atmos", "ai", "sessions", "sessions.db")
		assert.Equal(t, expected, result)
	})
}

func TestGetPermissionMode(t *testing.T) {
	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		expectedMode permission.Mode
	}{
		{
			name: "YOLO mode enabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							YOLOMode: true,
						},
					},
				},
			},
			expectedMode: permission.ModeYOLO,
		},
		{
			name: "require confirmation explicitly enabled",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							RequireConfirmation: boolPtr(true),
						},
					},
				},
			},
			expectedMode: permission.ModePrompt,
		},
		{
			name: "default prompt mode (not configured)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							// RequireConfirmation not set (nil) - defaults to prompt
						},
					},
				},
			},
			expectedMode: permission.ModePrompt,
		},
		{
			name: "YOLO takes precedence over confirmation",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							YOLOMode:            true,
							RequireConfirmation: boolPtr(true),
						},
					},
				},
			},
			expectedMode: permission.ModeYOLO,
		},
		{
			name: "explicitly disabled (opt-out) defaults to allow",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							YOLOMode:            false,
							RequireConfirmation: boolPtr(false),
						},
					},
				},
			},
			expectedMode: permission.ModeAllow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPermissionMode(tt.atmosConfig)
			assert.Equal(t, tt.expectedMode, result)
		})
	}
}
