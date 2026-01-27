package ai

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/schema"
)

// boolPtr returns a pointer to the given boolean value.
func boolPtr(b bool) *bool {
	return &b
}

// TestChatCmdStructure tests the chatCmd command structure and properties.
func TestChatCmdStructure(t *testing.T) {
	t.Run("command basics", func(t *testing.T) {
		assert.Equal(t, "chat", chatCmd.Use)
		assert.Equal(t, "Start interactive AI chat session", chatCmd.Short)
		assert.Contains(t, chatCmd.Long, "interactive chat session")
		assert.Contains(t, chatCmd.Long, "AI assistant")
	})

	t.Run("has session flag", func(t *testing.T) {
		flag := chatCmd.Flags().Lookup("session")
		require.NotNil(t, flag, "session flag should exist")
		assert.Equal(t, "string", flag.Value.Type())
		assert.Equal(t, "Resume or create a named session", flag.Usage)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("command has RunE", func(t *testing.T) {
		assert.NotNil(t, chatCmd.RunE, "chatCmd should have RunE defined")
	})

	t.Run("is subcommand of aiCmd", func(t *testing.T) {
		found := false
		for _, cmd := range aiCmd.Commands() {
			if cmd.Name() == "chat" {
				found = true
				break
			}
		}
		assert.True(t, found, "chat should be a subcommand of ai")
	})
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
		{
			name: "grok provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "grok",
					},
				},
			},
			expectedResult: "grok",
		},
		{
			name: "ollama provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "ollama",
					},
				},
			},
			expectedResult: "ollama",
		},
		{
			name: "bedrock provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "bedrock",
					},
				},
			},
			expectedResult: "bedrock",
		},
		{
			name: "azureopenai provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "azureopenai",
					},
				},
			},
			expectedResult: "azureopenai",
		},
		{
			name: "empty string provider uses default",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "",
					},
				},
			},
			expectedResult: "anthropic",
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
		{
			name: "returns model from openai provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "openai",
						Providers: map[string]*schema.AIProviderConfig{
							"openai": {
								Model:     "gpt-4-turbo",
								MaxTokens: 4096,
							},
						},
					},
				},
			},
			expectedResult: "gpt-4-turbo",
		},
		{
			name: "returns model from gemini provider",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "gemini",
						Providers: map[string]*schema.AIProviderConfig{
							"gemini": {
								Model:     "gemini-pro",
								MaxTokens: 2048,
							},
						},
					},
				},
			},
			expectedResult: "gemini-pro",
		},
		{
			name: "returns empty model when provider config has empty model",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "anthropic",
						Providers: map[string]*schema.AIProviderConfig{
							"anthropic": {
								Model:     "",
								MaxTokens: 4096,
							},
						},
					},
				},
			},
			expectedResult: "",
		},
		{
			name: "returns empty string with nil providers map",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "anthropic",
						Providers:       nil,
					},
				},
			},
			expectedResult: "",
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

	t.Run("single directory relative path", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Path: "sessions",
					},
				},
			},
		}
		result := getSessionStoragePath(atmosConfig)
		expected := filepath.Join(basePath, "sessions", "sessions.db")
		assert.Equal(t, expected, result)
	})

	t.Run("empty base path with relative session path", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: "",
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Path: "my-sessions",
					},
				},
			},
		}
		result := getSessionStoragePath(atmosConfig)
		expected := filepath.Join("my-sessions", "sessions.db")
		assert.Equal(t, expected, result)
	})

	t.Run("cross platform absolute path check", func(t *testing.T) {
		// This test verifies cross-platform behavior.
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Path: "relative-path",
					},
				},
			},
		}
		result := getSessionStoragePath(atmosConfig)
		// Verify the result is not absolute (relative path provided).
		assert.True(t, filepath.IsAbs(result) == filepath.IsAbs(basePath),
			"result should inherit absoluteness from basePath")
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
							// RequireConfirmation not set (nil) - defaults to prompt.
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
		{
			name: "YOLO mode with nil require confirmation",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							YOLOMode:            true,
							RequireConfirmation: nil,
						},
					},
				},
			},
			expectedMode: permission.ModeYOLO,
		},
		{
			name: "empty AI settings defaults to prompt",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expectedMode: permission.ModePrompt,
		},
		{
			name: "YOLO false with nil require confirmation defaults to prompt",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Tools: schema.AIToolSettings{
							YOLOMode:            false,
							RequireConfirmation: nil,
						},
					},
				},
			},
			expectedMode: permission.ModePrompt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPermissionMode(tt.atmosConfig)
			assert.Equal(t, tt.expectedMode, result)
		})
	}
}

// TestGetSessionStoragePathCrossPlatform tests path handling across different OS.
func TestGetSessionStoragePathCrossPlatform(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name        string
		sessionPath string
	}{
		{
			name:        "simple path",
			sessionPath: "sessions",
		},
		{
			name:        "nested path",
			sessionPath: filepath.Join("data", "ai", "sessions"),
		},
		{
			name:        "hidden directory",
			sessionPath: filepath.Join(".atmos", "sessions"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Sessions: schema.AISessionSettings{
							Path: tt.sessionPath,
						},
					},
				},
			}

			result := getSessionStoragePath(atmosConfig)

			// Verify the path contains the expected components.
			assert.Contains(t, result, "sessions.db")
			assert.True(t, filepath.IsAbs(result), "result should be an absolute path")

			// Verify the path uses the correct separator for the OS.
			if runtime.GOOS == "windows" {
				assert.NotContains(t, result, "/", "Windows paths should use backslashes")
			}
		})
	}
}

// TestGetModelFromConfigWithMultipleProviders tests model retrieval with multiple providers configured.
func TestGetModelFromConfigWithMultipleProviders(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				DefaultProvider: "openai",
				Providers: map[string]*schema.AIProviderConfig{
					"anthropic": {
						Model:     "claude-sonnet-4-20250514",
						MaxTokens: 4096,
					},
					"openai": {
						Model:     "gpt-4-turbo",
						MaxTokens: 8192,
					},
					"gemini": {
						Model:     "gemini-pro",
						MaxTokens: 2048,
					},
				},
			},
		},
	}

	// Should return the model for the default provider.
	result := getModelFromConfig(atmosConfig)
	assert.Equal(t, "gpt-4-turbo", result)

	// Change default provider and verify.
	atmosConfig.Settings.AI.DefaultProvider = "anthropic"
	result = getModelFromConfig(atmosConfig)
	assert.Equal(t, "claude-sonnet-4-20250514", result)

	// Change to gemini.
	atmosConfig.Settings.AI.DefaultProvider = "gemini"
	result = getModelFromConfig(atmosConfig)
	assert.Equal(t, "gemini-pro", result)
}

// TestGetProviderFromConfigWithEmptySettings tests edge cases.
func TestGetProviderFromConfigWithEmptySettings(t *testing.T) {
	tests := []struct {
		name           string
		atmosConfig    *schema.AtmosConfiguration
		expectedResult string
	}{
		{
			name: "nil providers map",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "anthropic",
						Providers:       nil,
					},
				},
			},
			expectedResult: "anthropic",
		},
		{
			name: "empty providers map",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "openai",
						Providers:       map[string]*schema.AIProviderConfig{},
					},
				},
			},
			expectedResult: "openai",
		},
		{
			name: "whitespace provider name",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						DefaultProvider: "   ",
					},
				},
			},
			expectedResult: "   ", // Function does not trim whitespace.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getProviderFromConfig(tt.atmosConfig)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestPermissionModeValues verifies the permission mode constants.
func TestPermissionModeValues(t *testing.T) {
	// Verify the permission modes are distinct.
	assert.NotEqual(t, permission.ModeYOLO, permission.ModePrompt)
	assert.NotEqual(t, permission.ModeYOLO, permission.ModeAllow)
	assert.NotEqual(t, permission.ModePrompt, permission.ModeAllow)
}

// TestChatCmdLongDescription verifies the long description contains expected content.
func TestChatCmdLongDescription(t *testing.T) {
	expectedContent := []string{
		"interactive chat session",
		"Atmos AI assistant",
		"Explaining Atmos concepts",
		"Analyzing your specific components",
		"Suggesting optimizations",
		"Debugging configuration issues",
		"implementation guidance",
	}

	for _, content := range expectedContent {
		assert.Contains(t, chatCmd.Long, content,
			"Long description should contain: %s", content)
	}
}

// TestChatCmdFlagDefaults verifies flag default values.
func TestChatCmdFlagDefaults(t *testing.T) {
	t.Run("session flag default", func(t *testing.T) {
		flag := chatCmd.Flags().Lookup("session")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})
}

// TestGetSessionStoragePathWithSpecialCharacters tests paths with special characters.
func TestGetSessionStoragePathWithSpecialCharacters(t *testing.T) {
	basePath := t.TempDir()

	tests := []struct {
		name        string
		sessionPath string
	}{
		{
			name:        "path with spaces",
			sessionPath: filepath.Join("my sessions", "data"),
		},
		{
			name:        "path with dots",
			sessionPath: filepath.Join(".hidden.dir", "sessions"),
		},
		{
			name:        "path with underscores",
			sessionPath: filepath.Join("my_sessions", "ai_data"),
		},
		{
			name:        "path with hyphens",
			sessionPath: filepath.Join("my-sessions", "ai-data"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: basePath,
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Sessions: schema.AISessionSettings{
							Path: tt.sessionPath,
						},
					},
				},
			}

			result := getSessionStoragePath(atmosConfig)

			// Verify result ends with sessions.db.
			assert.True(t, filepath.Base(result) == "sessions.db",
				"path should end with sessions.db")
		})
	}
}
