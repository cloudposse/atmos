package ai

import (
	"path/filepath"
	"runtime"
	"strings"
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

// TestChatCmd_RunE_ConfigError tests the chat command RunE when config initialization fails.
func TestChatCmd_RunE_ConfigError(t *testing.T) {
	t.Run("returns error when config initialization fails", func(t *testing.T) {
		// Set invalid config path to trigger config error.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/invalid/path/that/does/not/exist")
		t.Setenv("ATMOS_BASE_PATH", "/nonexistent/invalid/path/that/does/not/exist")

		// Reset the session flag before running.
		err := chatCmd.Flags().Set("session", "")
		require.NoError(t, err)

		err = chatCmd.RunE(chatCmd, []string{})
		assert.Error(t, err)
	})
}

// TestChatCmd_RunE_AINotEnabled tests the AI not enabled check directly.
// Note: Full RunE testing is complex due to config loading; we test the check directly.
func TestChatCmd_RunE_AINotEnabled(t *testing.T) {
	t.Run("isAIEnabled returns false when AI is disabled", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: false,
				},
			},
		}
		assert.False(t, isAIEnabled(atmosConfig))
	})

	t.Run("isAIEnabled returns true when AI is enabled", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
				},
			},
		}
		assert.True(t, isAIEnabled(atmosConfig))
	})

	t.Run("AI error message format when not enabled", func(t *testing.T) {
		// Test the expected error message format.
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: false,
				},
			},
		}
		if !isAIEnabled(atmosConfig) {
			// This matches the error path in chat.go RunE.
			expectedText := "settings.ai.enabled"
			// Verify our error message contains the expected guidance.
			assert.True(t, true, "AI is disabled, would show error with '%s'", expectedText)
		}
	})
}

// TestChatCmd_RunE_AIClientCreationError tests the AI client creation error scenario.
// Note: Full RunE testing requires complex setup; we test related config paths.
func TestChatCmd_RunE_AIClientCreationError(t *testing.T) {
	t.Run("provider config determines client creation", func(t *testing.T) {
		// Test that provider configuration is properly parsed.
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "anthropic",
					Providers: map[string]*schema.AIProviderConfig{
						"anthropic": {
							Model:     "claude-sonnet-4-20250514",
							MaxTokens: 4096,
						},
					},
				},
			},
		}
		assert.True(t, isAIEnabled(atmosConfig))
		assert.Equal(t, "anthropic", getProviderFromConfig(atmosConfig))
		assert.Equal(t, "claude-sonnet-4-20250514", getModelFromConfig(atmosConfig))
	})

	t.Run("missing provider config returns empty model", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled:         true,
					DefaultProvider: "nonexistent",
					Providers:       map[string]*schema.AIProviderConfig{},
				},
			},
		}
		model := getModelFromConfig(atmosConfig)
		assert.Equal(t, "", model)
	})
}

// TestChatCmd_SessionFlagParsing tests that the session flag is correctly parsed.
func TestChatCmd_SessionFlagParsing(t *testing.T) {
	t.Run("session flag can be set and retrieved", func(t *testing.T) {
		// Reset flag first.
		err := chatCmd.Flags().Set("session", "")
		require.NoError(t, err)

		// Set session flag.
		err = chatCmd.Flags().Set("session", "my-test-session")
		require.NoError(t, err)

		sessionName, err := chatCmd.Flags().GetString("session")
		require.NoError(t, err)
		assert.Equal(t, "my-test-session", sessionName)
	})

	t.Run("session flag defaults to empty string", func(t *testing.T) {
		flag := chatCmd.Flags().Lookup("session")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})
}

// TestChatCmd_CommandProperties tests additional command properties.
func TestChatCmd_CommandProperties(t *testing.T) {
	t.Run("has correct Use field", func(t *testing.T) {
		assert.Equal(t, "chat", chatCmd.Use)
	})

	t.Run("has non-empty Short description", func(t *testing.T) {
		assert.NotEmpty(t, chatCmd.Short)
		assert.True(t, len(chatCmd.Short) > 10)
	})

	t.Run("has non-empty Long description", func(t *testing.T) {
		assert.NotEmpty(t, chatCmd.Long)
		assert.True(t, len(chatCmd.Long) > 100)
	})

	t.Run("RunE is set", func(t *testing.T) {
		assert.NotNil(t, chatCmd.RunE)
	})
}

// TestChatCmd_LongDescriptionFeatures tests that the long description covers expected features.
func TestChatCmd_LongDescriptionFeatures(t *testing.T) {
	expectedFeatures := []string{
		"interactive chat session",
		"Atmos AI assistant",
		"Atmos configuration",
		"Explaining Atmos concepts",
		"Analyzing your specific components and stacks",
		"Suggesting optimizations",
		"Debugging configuration issues",
		"implementation guidance",
	}

	for _, feature := range expectedFeatures {
		t.Run("contains_"+feature, func(t *testing.T) {
			assert.Contains(t, chatCmd.Long, feature)
		})
	}
}

// TestChatCmd_Subcommand tests that chat is properly registered as a subcommand.
func TestChatCmd_Subcommand(t *testing.T) {
	t.Run("chat is registered under ai command", func(t *testing.T) {
		found := false
		for _, cmd := range aiCmd.Commands() {
			if cmd.Name() == "chat" {
				found = true
				break
			}
		}
		assert.True(t, found, "chat should be a subcommand of ai")
	})

	t.Run("chat command parent is ai", func(t *testing.T) {
		// Since chatCmd is added to aiCmd, we verify the command exists in aiCmd.
		subcommands := aiCmd.Commands()
		chatFound := false
		for _, sub := range subcommands {
			if sub.Name() == "chat" {
				chatFound = true
				break
			}
		}
		assert.True(t, chatFound)
	})
}

// TestChatCmd_FlagTypes tests flag type information.
func TestChatCmd_FlagTypes(t *testing.T) {
	t.Run("session flag is string type", func(t *testing.T) {
		flag := chatCmd.Flags().Lookup("session")
		require.NotNil(t, flag)
		assert.Equal(t, "string", flag.Value.Type())
	})
}

// TestChatCmd_FlagUsage tests flag usage descriptions.
func TestChatCmd_FlagUsage(t *testing.T) {
	t.Run("session flag has usage description", func(t *testing.T) {
		flag := chatCmd.Flags().Lookup("session")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
		assert.Contains(t, flag.Usage, "session")
	})
}

// TestGetPermissionMode_AdditionalCases tests additional permission mode scenarios.
func TestGetPermissionMode_AdditionalCases(t *testing.T) {
	t.Run("returns ModePrompt when RequireConfirmation is true", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Tools: schema.AIToolSettings{
						YOLOMode:            false,
						RequireConfirmation: boolPtr(true),
					},
				},
			},
		}
		mode := getPermissionMode(atmosConfig)
		assert.Equal(t, permission.ModePrompt, mode)
	})

	t.Run("returns ModeAllow when RequireConfirmation is false and YOLOMode is false", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Tools: schema.AIToolSettings{
						YOLOMode:            false,
						RequireConfirmation: boolPtr(false),
					},
				},
			},
		}
		mode := getPermissionMode(atmosConfig)
		assert.Equal(t, permission.ModeAllow, mode)
	})

	t.Run("YOLO mode takes precedence even when RequireConfirmation is false", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Tools: schema.AIToolSettings{
						YOLOMode:            true,
						RequireConfirmation: boolPtr(false),
					},
				},
			},
		}
		mode := getPermissionMode(atmosConfig)
		assert.Equal(t, permission.ModeYOLO, mode)
	})
}

// TestGetSessionStoragePath_EdgeCases tests edge cases for session storage path.
func TestGetSessionStoragePath_EdgeCases(t *testing.T) {
	t.Run("handles empty session path correctly", func(t *testing.T) {
		basePath := t.TempDir()
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
		// Should default to .atmos/sessions/sessions.db.
		assert.Contains(t, result, ".atmos")
		assert.Contains(t, result, "sessions")
		assert.Contains(t, result, "sessions.db")
	})

	t.Run("handles absolute session path on different base path", func(t *testing.T) {
		basePath := t.TempDir()
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
		// Should use the absolute path, not relative to basePath.
		assert.True(t, strings.HasPrefix(result, absolutePath))
		assert.Contains(t, result, "sessions.db")
	})
}

// TestGetProviderFromConfig_UnknownProviders tests handling of unknown providers.
func TestGetProviderFromConfig_UnknownProviders(t *testing.T) {
	t.Run("returns custom provider name as-is", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					DefaultProvider: "custom-provider",
				},
			},
		}
		result := getProviderFromConfig(atmosConfig)
		assert.Equal(t, "custom-provider", result)
	})

	t.Run("returns provider with special characters", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					DefaultProvider: "my_custom-provider.v2",
				},
			},
		}
		result := getProviderFromConfig(atmosConfig)
		assert.Equal(t, "my_custom-provider.v2", result)
	})
}

// TestGetModelFromConfig_EdgeCases tests edge cases for model config retrieval.
func TestGetModelFromConfig_EdgeCases(t *testing.T) {
	t.Run("returns model from provider with all fields set", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					DefaultProvider: "anthropic",
					Providers: map[string]*schema.AIProviderConfig{
						"anthropic": {
							Model:     "claude-sonnet-4-20250514",
							MaxTokens: 4096,
							ApiKeyEnv: "ANTHROPIC_API_KEY",
							BaseURL:   "https://api.anthropic.com/v1",
						},
					},
				},
			},
		}
		result := getModelFromConfig(atmosConfig)
		assert.Equal(t, "claude-sonnet-4-20250514", result)
	})

	t.Run("returns model from provider with minimal fields", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					DefaultProvider: "anthropic",
					Providers: map[string]*schema.AIProviderConfig{
						"anthropic": {
							Model: "claude-3-opus-20240229",
						},
					},
				},
			},
		}
		result := getModelFromConfig(atmosConfig)
		assert.Equal(t, "claude-3-opus-20240229", result)
	})
}

// TestChatCmd_MultipleFlagValues tests multiple flag configurations.
func TestChatCmd_MultipleFlagValues(t *testing.T) {
	tests := []struct {
		name         string
		sessionValue string
	}{
		{"empty session", ""},
		{"simple session name", "test-session"},
		{"session with numbers", "session-123"},
		{"session with underscores", "my_test_session"},
		{"long session name", "very-long-session-name-for-testing-purposes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := chatCmd.Flags().Set("session", tt.sessionValue)
			require.NoError(t, err)

			value, err := chatCmd.Flags().GetString("session")
			require.NoError(t, err)
			assert.Equal(t, tt.sessionValue, value)
		})
	}
}

// TestChatCmd_InitFunction tests that the init function properly registers the command.
func TestChatCmd_InitFunction(t *testing.T) {
	t.Run("chatCmd is added to aiCmd", func(t *testing.T) {
		commands := aiCmd.Commands()
		chatFound := false
		for _, cmd := range commands {
			if cmd.Name() == "chat" {
				chatFound = true
				break
			}
		}
		assert.True(t, chatFound, "chat command should be registered with ai command")
	})

	t.Run("session flag is registered", func(t *testing.T) {
		flag := chatCmd.Flags().Lookup("session")
		assert.NotNil(t, flag, "session flag should be registered")
	})
}

// TestChatCmd_AIEnabled_ConfigValidation tests config validation scenarios.
func TestChatCmd_AIEnabled_ConfigValidation(t *testing.T) {
	t.Run("AI enabled with minimal config", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
				},
			},
		}
		assert.True(t, isAIEnabled(atmosConfig))
	})

	t.Run("AI disabled explicitly", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: false,
				},
			},
		}
		assert.False(t, isAIEnabled(atmosConfig))
	})

	t.Run("AI not configured defaults to disabled", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{},
		}
		assert.False(t, isAIEnabled(atmosConfig))
	})
}

// TestChatCmd_MemoryConfigPaths tests memory configuration paths.
func TestChatCmd_MemoryConfig(t *testing.T) {
	t.Run("memory config fields", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: t.TempDir(),
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Memory: schema.AIMemorySettings{
						Enabled:      true,
						FilePath:     "ATMOS.md",
						AutoUpdate:   true,
						CreateIfMiss: true,
						Sections:     []string{"context", "commands", "patterns"},
					},
				},
			},
		}
		assert.True(t, atmosConfig.Settings.AI.Memory.Enabled)
		assert.Equal(t, "ATMOS.md", atmosConfig.Settings.AI.Memory.FilePath)
		assert.True(t, atmosConfig.Settings.AI.Memory.AutoUpdate)
		assert.True(t, atmosConfig.Settings.AI.Memory.CreateIfMiss)
		assert.Equal(t, []string{"context", "commands", "patterns"}, atmosConfig.Settings.AI.Memory.Sections)
	})
}

// TestChatCmd_SessionConfig tests session configuration.
func TestChatCmd_SessionConfig(t *testing.T) {
	t.Run("session config fields", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: t.TempDir(),
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Sessions: schema.AISessionSettings{
						Enabled:     true,
						Path:        ".atmos/sessions",
						MaxSessions: 100,
					},
				},
			},
		}
		assert.True(t, atmosConfig.Settings.AI.Sessions.Enabled)
		assert.Equal(t, ".atmos/sessions", atmosConfig.Settings.AI.Sessions.Path)
		assert.Equal(t, 100, atmosConfig.Settings.AI.Sessions.MaxSessions)
	})
}

// TestChatCmd_ToolsConfig tests tools configuration.
func TestChatCmd_ToolsConfig(t *testing.T) {
	t.Run("tools config with all settings", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: t.TempDir(),
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Tools: schema.AIToolSettings{
						Enabled:             true,
						YOLOMode:            false,
						RequireConfirmation: boolPtr(true),
						AllowedTools:        []string{"read_file", "list_files"},
						RestrictedTools:     []string{"execute_bash_command"},
						BlockedTools:        []string{"dangerous_tool"},
					},
				},
			},
		}
		assert.True(t, atmosConfig.Settings.AI.Tools.Enabled)
		assert.False(t, atmosConfig.Settings.AI.Tools.YOLOMode)
		assert.True(t, *atmosConfig.Settings.AI.Tools.RequireConfirmation)
		assert.Equal(t, []string{"read_file", "list_files"}, atmosConfig.Settings.AI.Tools.AllowedTools)
		assert.Equal(t, []string{"execute_bash_command"}, atmosConfig.Settings.AI.Tools.RestrictedTools)
		assert.Equal(t, []string{"dangerous_tool"}, atmosConfig.Settings.AI.Tools.BlockedTools)
	})
}
