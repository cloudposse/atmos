package ai

import (
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
	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		expectedPath string
	}{
		{
			name: "default path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/test/project",
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Sessions: schema.AISessionSettings{
							Path: "",
						},
					},
				},
			},
			expectedPath: "/test/project/.atmos/sessions/sessions.db",
		},
		{
			name: "custom relative path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/test/project",
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Sessions: schema.AISessionSettings{
							Path: "data/ai-sessions",
						},
					},
				},
			},
			expectedPath: "/test/project/data/ai-sessions/sessions.db",
		},
		{
			name: "absolute path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/test/project",
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Sessions: schema.AISessionSettings{
							Path: "/var/atmos/sessions",
						},
					},
				},
			},
			expectedPath: "/var/atmos/sessions/sessions.db",
		},
		{
			name: "path with nested directories",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/home/user/project",
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Sessions: schema.AISessionSettings{
							Path: ".config/atmos/ai/sessions",
						},
					},
				},
			},
			expectedPath: "/home/user/project/.config/atmos/ai/sessions/sessions.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSessionStoragePath(tt.atmosConfig)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
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
