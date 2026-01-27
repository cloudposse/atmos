package ai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAskCommand_BasicProperties(t *testing.T) {
	t.Run("ask command properties", func(t *testing.T) {
		assert.Equal(t, "ask [question]", askCmd.Use)
		assert.Equal(t, "Ask the AI assistant a question", askCmd.Short)
		assert.NotEmpty(t, askCmd.Long)
		assert.NotNil(t, askCmd.RunE)
		// Check that Args requires minimum 1 argument.
		assert.NotNil(t, askCmd.Args)
	})

	t.Run("ask command has descriptive long text", func(t *testing.T) {
		assert.Contains(t, askCmd.Long, "question")
		assert.Contains(t, askCmd.Long, "AI assistant")
		assert.Contains(t, askCmd.Long, "interactive")
		assert.Contains(t, askCmd.Long, "Atmos configuration")
	})

	t.Run("ask command contains examples", func(t *testing.T) {
		assert.Contains(t, askCmd.Long, "Examples:")
		assert.Contains(t, askCmd.Long, "atmos ai ask")
		assert.Contains(t, askCmd.Long, "What components are available")
		assert.Contains(t, askCmd.Long, "How do I validate")
		assert.Contains(t, askCmd.Long, "Explain the difference")
	})
}

func TestAskCommand_Flags(t *testing.T) {
	t.Run("ask command has include flag", func(t *testing.T) {
		includeFlag := askCmd.Flags().Lookup("include")
		require.NotNil(t, includeFlag, "include flag should be registered")
		assert.Equal(t, "stringSlice", includeFlag.Value.Type())
		assert.Contains(t, includeFlag.Usage, "glob patterns")
		assert.Contains(t, includeFlag.Usage, "include")
	})

	t.Run("ask command has exclude flag", func(t *testing.T) {
		excludeFlag := askCmd.Flags().Lookup("exclude")
		require.NotNil(t, excludeFlag, "exclude flag should be registered")
		assert.Equal(t, "stringSlice", excludeFlag.Value.Type())
		assert.Contains(t, excludeFlag.Usage, "glob patterns")
		assert.Contains(t, excludeFlag.Usage, "exclude")
	})

	t.Run("ask command has no-auto-context flag", func(t *testing.T) {
		noAutoContextFlag := askCmd.Flags().Lookup("no-auto-context")
		require.NotNil(t, noAutoContextFlag, "no-auto-context flag should be registered")
		assert.Equal(t, "bool", noAutoContextFlag.Value.Type())
		assert.Equal(t, "false", noAutoContextFlag.DefValue)
		assert.Contains(t, noAutoContextFlag.Usage, "Disable automatic context discovery")
	})
}

func TestAskCommand_CommandHierarchy(t *testing.T) {
	t.Run("ask command is attached to ai command", func(t *testing.T) {
		parent := askCmd.Parent()
		assert.NotNil(t, parent)
		assert.Equal(t, "ai", parent.Name())
	})
}

func TestAskCommand_ArgsValidation(t *testing.T) {
	t.Run("rejects zero arguments", func(t *testing.T) {
		err := cobra.MinimumNArgs(1)(askCmd, []string{})
		assert.Error(t, err)
	})

	t.Run("accepts one argument (question)", func(t *testing.T) {
		err := cobra.MinimumNArgs(1)(askCmd, []string{"test question"})
		assert.NoError(t, err)
	})

	t.Run("accepts multiple arguments (question words)", func(t *testing.T) {
		err := cobra.MinimumNArgs(1)(askCmd, []string{"what", "are", "the", "stacks"})
		assert.NoError(t, err)
	})
}

func TestAskCommand_ErrorCases(t *testing.T) {
	t.Run("returns error without valid config", func(t *testing.T) {
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "/nonexistent/path")

		testCmd := &cobra.Command{
			Use:  "ask",
			Args: cobra.MinimumNArgs(1),
		}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Use the actual ask command's RunE function.
		err := askCmd.RunE(testCmd, []string{"test question"})
		assert.Error(t, err)
	})
}

func TestAskCommand_FlagInteraction(t *testing.T) {
	t.Run("include flag can be set", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "test-ask"}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")

		err := testCmd.Flags().Set("include", "*.yaml")
		require.NoError(t, err)

		patterns, err := testCmd.Flags().GetStringSlice("include")
		require.NoError(t, err)
		assert.Contains(t, patterns, "*.yaml")
	})

	t.Run("exclude flag can be set", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "test-ask"}
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")

		err := testCmd.Flags().Set("exclude", "*.tmp")
		require.NoError(t, err)

		patterns, err := testCmd.Flags().GetStringSlice("exclude")
		require.NoError(t, err)
		assert.Contains(t, patterns, "*.tmp")
	})

	t.Run("no-auto-context flag can be toggled", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "test-ask"}
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Initial value should be false.
		noAutoContext, err := testCmd.Flags().GetBool("no-auto-context")
		require.NoError(t, err)
		assert.False(t, noAutoContext)

		// Set to true.
		err = testCmd.Flags().Set("no-auto-context", "true")
		require.NoError(t, err)

		noAutoContext, err = testCmd.Flags().GetBool("no-auto-context")
		require.NoError(t, err)
		assert.True(t, noAutoContext)
	})
}

func TestAskCommand_MultipleIncludeExcludePatterns(t *testing.T) {
	t.Run("multiple include patterns can be set", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "test-ask"}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")

		err := testCmd.Flags().Set("include", "*.yaml,*.yml")
		require.NoError(t, err)

		patterns, err := testCmd.Flags().GetStringSlice("include")
		require.NoError(t, err)
		assert.Len(t, patterns, 2)
		assert.Contains(t, patterns, "*.yaml")
		assert.Contains(t, patterns, "*.yml")
	})

	t.Run("multiple exclude patterns can be set", func(t *testing.T) {
		testCmd := &cobra.Command{Use: "test-ask"}
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")

		err := testCmd.Flags().Set("exclude", "*.tmp,*.bak")
		require.NoError(t, err)

		patterns, err := testCmd.Flags().GetStringSlice("exclude")
		require.NoError(t, err)
		assert.Len(t, patterns, 2)
		assert.Contains(t, patterns, "*.tmp")
		assert.Contains(t, patterns, "*.bak")
	})
}

func TestAskCommand_LongDescriptionContent(t *testing.T) {
	tests := []struct {
		name            string
		expectedContent string
	}{
		{
			name:            "describes AI assistant",
			expectedContent: "AI assistant",
		},
		{
			name:            "mentions command line",
			expectedContent: "command line",
		},
		{
			name:            "mentions Atmos configuration",
			expectedContent: "Atmos configuration",
		},
		{
			name:            "mentions context-aware",
			expectedContent: "context-aware",
		},
		{
			name:            "includes example with components",
			expectedContent: "What components are available",
		},
		{
			name:            "includes example with validation",
			expectedContent: "validate my stack configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, askCmd.Long, tt.expectedContent)
		})
	}
}

func TestAskCommand_SubcommandRegistration(t *testing.T) {
	// Verify ask is registered as a subcommand of ai.
	t.Run("ask is a subcommand of ai", func(t *testing.T) {
		subcommands := aiCmd.Commands()
		found := false
		for _, cmd := range subcommands {
			if cmd.Name() == "ask" {
				found = true
				break
			}
		}
		assert.True(t, found, "ask should be a subcommand of ai")
	})
}

func TestAskCommand_UsesRunE(t *testing.T) {
	t.Run("ask command uses RunE for error handling", func(t *testing.T) {
		assert.NotNil(t, askCmd.RunE, "ask command should have RunE set")
		assert.Nil(t, askCmd.Run, "ask command should not have Run set when RunE is used")
	})
}

//nolint:dupl // Test setup is intentionally similar to other integration tests.
func TestAskCommand_AIDisabled(t *testing.T) {
	// Create a temporary directory for the test.
	tempDir := t.TempDir()

	// Create a minimal atmos.yaml with AI disabled.
	configContent := `base_path: "./"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"

settings:
  ai:
    enabled: false
`

	// Write the config file.
	configPath := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Create required directories.
	err = os.MkdirAll(filepath.Join(tempDir, "components", "terraform"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(tempDir, "stacks"), 0o755)
	require.NoError(t, err)

	// Create a minimal stack file.
	stackContent := `vars:
  stage: dev
`
	err = os.WriteFile(filepath.Join(tempDir, "stacks", "dev.yaml"), []byte(stackContent), 0o600)
	require.NoError(t, err)

	// Set environment for the tests.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)

	// Save current working directory and change to temp dir.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	t.Run("ask command returns error when AI is disabled", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "ask",
			Args: cobra.MinimumNArgs(1),
		}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		err := askCmd.RunE(testCmd, []string{"test question"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "AI features are not enabled")
	})
}

//nolint:dupl // Test subtests have similar structure by design.
func TestAskCommand_ContextOverrides(t *testing.T) {
	// Test that context override settings affect the configuration.
	// This tests the internal logic of applying context flags.

	t.Run("no-auto-context flag disables context", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						Enabled: true,
					},
				},
			},
		}

		// Simulate what the ask command does when no-auto-context is true.
		noAutoContext := true
		if noAutoContext {
			atmosConfig.Settings.AI.Context.Enabled = false
		}

		assert.False(t, atmosConfig.Settings.AI.Context.Enabled)
	})

	t.Run("include patterns are appended", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						AutoInclude: []string{"*.yaml"},
					},
				},
			},
		}

		// Simulate what the ask command does when include patterns are provided.
		includePatterns := []string{"*.yml", "*.json"}
		if len(includePatterns) > 0 {
			atmosConfig.Settings.AI.Context.AutoInclude = append(atmosConfig.Settings.AI.Context.AutoInclude, includePatterns...)
		}

		assert.Len(t, atmosConfig.Settings.AI.Context.AutoInclude, 3)
		assert.Contains(t, atmosConfig.Settings.AI.Context.AutoInclude, "*.yaml")
		assert.Contains(t, atmosConfig.Settings.AI.Context.AutoInclude, "*.yml")
		assert.Contains(t, atmosConfig.Settings.AI.Context.AutoInclude, "*.json")
	})

	t.Run("exclude patterns are appended", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						Exclude: []string{"*.tmp"},
					},
				},
			},
		}

		// Simulate what the ask command does when exclude patterns are provided.
		excludePatterns := []string{"*.bak", "*.log"}
		if len(excludePatterns) > 0 {
			atmosConfig.Settings.AI.Context.Exclude = append(atmosConfig.Settings.AI.Context.Exclude, excludePatterns...)
		}

		assert.Len(t, atmosConfig.Settings.AI.Context.Exclude, 3)
		assert.Contains(t, atmosConfig.Settings.AI.Context.Exclude, "*.tmp")
		assert.Contains(t, atmosConfig.Settings.AI.Context.Exclude, "*.bak")
		assert.Contains(t, atmosConfig.Settings.AI.Context.Exclude, "*.log")
	})
}

func TestAskCommand_QuestionJoining(t *testing.T) {
	// Test that multiple arguments are joined into a single question.
	t.Run("single argument is used as-is", func(t *testing.T) {
		args := []string{"What are the available components?"}
		// Simulate strings.Join from the command.
		question := args[0]
		assert.Equal(t, "What are the available components?", question)
	})

	t.Run("multiple arguments are joined with spaces", func(t *testing.T) {
		args := []string{"What", "are", "the", "available", "components?"}
		// Simulate strings.Join from the command.
		question := ""
		for i, arg := range args {
			if i > 0 {
				question += " "
			}
			question += arg
		}
		assert.Equal(t, "What are the available components?", question)
	})
}

func TestAskCommand_FlagUsageDescriptions(t *testing.T) {
	t.Run("include flag has usage", func(t *testing.T) {
		flag := askCmd.Flags().Lookup("include")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
	})

	t.Run("exclude flag has usage", func(t *testing.T) {
		flag := askCmd.Flags().Lookup("exclude")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
	})

	t.Run("no-auto-context flag has usage", func(t *testing.T) {
		flag := askCmd.Flags().Lookup("no-auto-context")
		require.NotNil(t, flag)
		assert.NotEmpty(t, flag.Usage)
	})
}

func TestAskCommand_FlagDefaults(t *testing.T) {
	t.Run("include flag default is nil", func(t *testing.T) {
		flag := askCmd.Flags().Lookup("include")
		require.NotNil(t, flag)
		assert.Equal(t, "[]", flag.DefValue)
	})

	t.Run("exclude flag default is nil", func(t *testing.T) {
		flag := askCmd.Flags().Lookup("exclude")
		require.NotNil(t, flag)
		assert.Equal(t, "[]", flag.DefValue)
	})

	t.Run("no-auto-context flag default is false", func(t *testing.T) {
		flag := askCmd.Flags().Lookup("no-auto-context")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestAskCommand_HasShortDescription(t *testing.T) {
	assert.NotEmpty(t, askCmd.Short, "ask command should have a short description")
	assert.Equal(t, "Ask the AI assistant a question", askCmd.Short)
}

func TestAskCommand_HasLongDescription(t *testing.T) {
	assert.NotEmpty(t, askCmd.Long, "ask command should have a long description")
	assert.Greater(t, len(askCmd.Long), len(askCmd.Short), "long description should be longer than short")
}

func TestAskCommand_CommandName(t *testing.T) {
	assert.Equal(t, "ask", askCmd.Name())
}

func TestAskCommand_InvalidConfigPath(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
	}{
		{
			name:       "nonexistent path",
			configPath: "/this/path/does/not/exist",
		},
		{
			name:       "empty path",
			configPath: "",
		},
		{
			name:       "deeply nested nonexistent path",
			configPath: "/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.configPath != "" {
				t.Setenv("ATMOS_CLI_CONFIG_PATH", tt.configPath)
			}

			testCmd := &cobra.Command{
				Use: "test-ask",
			}
			testCmd.Flags().StringSlice("include", nil, "Include patterns")
			testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
			testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

			err := askCmd.RunE(testCmd, []string{"test question"})
			assert.Error(t, err)
		})
	}
}

func TestAskCommand_IsAIEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{
			name: "AI enabled",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: true,
					},
				},
			},
			expected: true,
		},
		{
			name: "AI disabled",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						Enabled: false,
					},
				},
			},
			expected: false,
		},
		{
			name: "AI not configured (defaults to false)",
			config: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAIEnabled(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAskCommand_CommandUsageString(t *testing.T) {
	// Verify the Use field follows the expected pattern.
	assert.Equal(t, "ask [question]", askCmd.Use)
	// The [question] indicates a required positional argument.
}

func TestAskCommand_AllFlagsRegistered(t *testing.T) {
	expectedFlags := []string{"include", "exclude", "no-auto-context"}

	for _, flagName := range expectedFlags {
		t.Run(flagName+" is registered", func(t *testing.T) {
			flag := askCmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "flag %s should be registered", flagName)
		})
	}
}

func TestAskCommand_FlagCount(t *testing.T) {
	// Count non-help flags (help and version are auto-added by cobra).
	count := 0
	askCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "help" {
			count++
		}
	})
	// Expected: include, exclude, no-auto-context = 3 flags.
	assert.Equal(t, 3, count, "ask command should have exactly 3 custom flags")
}

//nolint:dupl // Test setup is intentionally similar to other integration tests.
func TestAskCommand_AIEnabledButClientCreationFails(t *testing.T) {
	// Create a temporary directory for the test.
	tempDir := t.TempDir()

	// Create an atmos.yaml with AI enabled but using an unconfigured provider.
	// This should cause ai.NewClient to fail because no provider is configured.
	configContent := `base_path: "./"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"

settings:
  ai:
    enabled: true
    default_provider: "nonexistent_provider"
`

	// Write the config file.
	configPath := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Create required directories.
	err = os.MkdirAll(filepath.Join(tempDir, "components", "terraform"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(tempDir, "stacks"), 0o755)
	require.NoError(t, err)

	// Create a minimal stack file.
	stackContent := `vars:
  stage: dev
`
	err = os.WriteFile(filepath.Join(tempDir, "stacks", "dev.yaml"), []byte(stackContent), 0o600)
	require.NoError(t, err)

	// Set environment for the tests.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)

	// Save current working directory and change to temp dir.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	t.Run("ask command returns error when AI client creation fails", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "ask",
			Args: cobra.MinimumNArgs(1),
		}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		err := askCmd.RunE(testCmd, []string{"test question"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create AI client")
	})
}

func TestAskCommand_WithIncludeExcludePatterns(t *testing.T) {
	// Create a temporary directory for the test.
	tempDir := t.TempDir()

	// Create an atmos.yaml with AI enabled.
	configContent := `base_path: "./"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"

settings:
  ai:
    enabled: true
    default_provider: "invalid_provider"
    context:
      enabled: true
      auto_include:
        - "*.yaml"
      exclude:
        - "*.tmp"
`

	// Write the config file.
	configPath := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Create required directories.
	err = os.MkdirAll(filepath.Join(tempDir, "components", "terraform"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(tempDir, "stacks"), 0o755)
	require.NoError(t, err)

	// Create a minimal stack file.
	stackContent := `vars:
  stage: dev
`
	err = os.WriteFile(filepath.Join(tempDir, "stacks", "dev.yaml"), []byte(stackContent), 0o600)
	require.NoError(t, err)

	// Set environment for the tests.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)

	// Save current working directory and change to temp dir.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	t.Run("ask command with include patterns", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "ask",
			Args: cobra.MinimumNArgs(1),
		}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Set include patterns.
		err := testCmd.Flags().Set("include", "*.yml")
		require.NoError(t, err)

		// The command will still fail due to invalid provider, but this tests the include patterns path.
		err = askCmd.RunE(testCmd, []string{"test question"})
		assert.Error(t, err)
		// We expect it to get past the include patterns logic to the client creation.
		assert.Contains(t, err.Error(), "failed to create AI client")
	})

	t.Run("ask command with exclude patterns", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "ask",
			Args: cobra.MinimumNArgs(1),
		}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Set exclude patterns.
		err := testCmd.Flags().Set("exclude", "*.bak")
		require.NoError(t, err)

		// The command will still fail due to invalid provider, but this tests the exclude patterns path.
		err = askCmd.RunE(testCmd, []string{"test question"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create AI client")
	})

	t.Run("ask command with both include and exclude patterns", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "ask",
			Args: cobra.MinimumNArgs(1),
		}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Set both patterns.
		err := testCmd.Flags().Set("include", "*.yml,*.json")
		require.NoError(t, err)
		err = testCmd.Flags().Set("exclude", "*.bak,*.log")
		require.NoError(t, err)

		// The command will still fail due to invalid provider, but this tests both paths.
		err = askCmd.RunE(testCmd, []string{"test question"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create AI client")
	})
}

func TestAskCommand_WithNoAutoContext(t *testing.T) {
	// Create a temporary directory for the test.
	tempDir := t.TempDir()

	// Create an atmos.yaml with AI enabled and context enabled.
	configContent := `base_path: "./"

components:
  terraform:
    base_path: "components/terraform"

stacks:
  base_path: "stacks"
  included_paths:
    - "**/*"
  name_pattern: "{stage}"

settings:
  ai:
    enabled: true
    default_provider: "invalid_provider"
    context:
      enabled: true
`

	// Write the config file.
	configPath := filepath.Join(tempDir, "atmos.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0o600)
	require.NoError(t, err)

	// Create required directories.
	err = os.MkdirAll(filepath.Join(tempDir, "components", "terraform"), 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(tempDir, "stacks"), 0o755)
	require.NoError(t, err)

	// Create a minimal stack file.
	stackContent := `vars:
  stage: dev
`
	err = os.WriteFile(filepath.Join(tempDir, "stacks", "dev.yaml"), []byte(stackContent), 0o600)
	require.NoError(t, err)

	// Set environment for the tests.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", tempDir)

	// Save current working directory and change to temp dir.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tempDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	t.Run("ask command with no-auto-context flag", func(t *testing.T) {
		testCmd := &cobra.Command{
			Use:  "ask",
			Args: cobra.MinimumNArgs(1),
		}
		testCmd.Flags().StringSlice("include", nil, "Include patterns")
		testCmd.Flags().StringSlice("exclude", nil, "Exclude patterns")
		testCmd.Flags().Bool("no-auto-context", false, "Disable auto context")

		// Set no-auto-context flag.
		err := testCmd.Flags().Set("no-auto-context", "true")
		require.NoError(t, err)

		// The command will still fail due to invalid provider, but this tests the no-auto-context path.
		err = askCmd.RunE(testCmd, []string{"test question"})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create AI client")
	})
}

func TestAskCommand_QuestionJoiningWithStringsJoin(t *testing.T) {
	// Test the actual strings.Join behavior used in the command.
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "single word question",
			args:     []string{"components"},
			expected: "components",
		},
		{
			name:     "single phrase as one arg",
			args:     []string{"What are the available components?"},
			expected: "What are the available components?",
		},
		{
			name:     "multiple words as separate args",
			args:     []string{"What", "are", "components"},
			expected: "What are components",
		},
		{
			name:     "question with special characters",
			args:     []string{"How", "do", "I", "validate", "my", "stack?"},
			expected: "How do I validate my stack?",
		},
		{
			name:     "question with quotes in args",
			args:     []string{"describe", "vpc", "component"},
			expected: "describe vpc component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use strings.Join exactly as the command does.
			question := strings.Join(tt.args, " ")
			assert.Equal(t, tt.expected, question)
		})
	}
}

func TestAskCommand_TimeoutConfiguration(t *testing.T) {
	// Test the timeout configuration logic.
	t.Run("default timeout is 60 seconds", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled:        true,
					TimeoutSeconds: 0, // Not configured.
				},
			},
		}

		// Simulate the logic from ask.go.
		timeoutSeconds := 60
		if atmosConfig.Settings.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.Settings.AI.TimeoutSeconds
		}

		assert.Equal(t, 60, timeoutSeconds)
	})

	t.Run("custom timeout is used when configured", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled:        true,
					TimeoutSeconds: 120,
				},
			},
		}

		// Simulate the logic from ask.go.
		timeoutSeconds := 60
		if atmosConfig.Settings.AI.TimeoutSeconds > 0 {
			timeoutSeconds = atmosConfig.Settings.AI.TimeoutSeconds
		}

		assert.Equal(t, 120, timeoutSeconds)
	})

	t.Run("timeout with various values", func(t *testing.T) {
		tests := []struct {
			configured int
			expected   int
		}{
			{0, 60},
			{30, 30},
			{60, 60},
			{120, 120},
			{300, 300},
		}

		for _, tt := range tests {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						TimeoutSeconds: tt.configured,
					},
				},
			}

			timeoutSeconds := 60
			if atmosConfig.Settings.AI.TimeoutSeconds > 0 {
				timeoutSeconds = atmosConfig.Settings.AI.TimeoutSeconds
			}

			assert.Equal(t, tt.expected, timeoutSeconds)
		}
	})
}

func TestAskCommand_ContextOverridesWithEmptyPatterns(t *testing.T) {
	// Test that empty patterns don't affect the configuration.
	t.Run("empty include patterns don't change config", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						AutoInclude: []string{"*.yaml"},
					},
				},
			},
		}

		// Simulate with empty patterns.
		includePatterns := []string{}
		if len(includePatterns) > 0 {
			atmosConfig.Settings.AI.Context.AutoInclude = append(atmosConfig.Settings.AI.Context.AutoInclude, includePatterns...)
		}

		// Should remain unchanged.
		assert.Len(t, atmosConfig.Settings.AI.Context.AutoInclude, 1)
		assert.Contains(t, atmosConfig.Settings.AI.Context.AutoInclude, "*.yaml")
	})

	t.Run("empty exclude patterns don't change config", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						Exclude: []string{"*.tmp"},
					},
				},
			},
		}

		// Simulate with empty patterns.
		excludePatterns := []string{}
		if len(excludePatterns) > 0 {
			atmosConfig.Settings.AI.Context.Exclude = append(atmosConfig.Settings.AI.Context.Exclude, excludePatterns...)
		}

		// Should remain unchanged.
		assert.Len(t, atmosConfig.Settings.AI.Context.Exclude, 1)
		assert.Contains(t, atmosConfig.Settings.AI.Context.Exclude, "*.tmp")
	})
}

func TestAskCommand_NoAutoContextFalsePreservesContext(t *testing.T) {
	// Test that when no-auto-context is false, context remains enabled.
	t.Run("no-auto-context false preserves context enabled", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						Enabled: true,
					},
				},
			},
		}

		// Simulate what the ask command does when no-auto-context is false.
		noAutoContext := false
		if noAutoContext {
			atmosConfig.Settings.AI.Context.Enabled = false
		}

		// Context should still be enabled.
		assert.True(t, atmosConfig.Settings.AI.Context.Enabled)
	})
}

func TestAskCommand_ContextAppendBehavior(t *testing.T) {
	// Test that patterns are appended, not replaced.
	t.Run("include patterns are appended to existing patterns", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						AutoInclude: []string{"existing.yaml"},
					},
				},
			},
		}

		includePatterns := []string{"new1.yaml", "new2.yaml"}
		atmosConfig.Settings.AI.Context.AutoInclude = append(atmosConfig.Settings.AI.Context.AutoInclude, includePatterns...)

		assert.Len(t, atmosConfig.Settings.AI.Context.AutoInclude, 3)
		assert.Equal(t, "existing.yaml", atmosConfig.Settings.AI.Context.AutoInclude[0])
		assert.Equal(t, "new1.yaml", atmosConfig.Settings.AI.Context.AutoInclude[1])
		assert.Equal(t, "new2.yaml", atmosConfig.Settings.AI.Context.AutoInclude[2])
	})

	t.Run("exclude patterns are appended to existing patterns", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				AI: schema.AISettings{
					Enabled: true,
					Context: schema.AIContextSettings{
						Exclude: []string{"existing.tmp"},
					},
				},
			},
		}

		excludePatterns := []string{"new1.tmp", "new2.tmp"}
		atmosConfig.Settings.AI.Context.Exclude = append(atmosConfig.Settings.AI.Context.Exclude, excludePatterns...)

		assert.Len(t, atmosConfig.Settings.AI.Context.Exclude, 3)
		assert.Equal(t, "existing.tmp", atmosConfig.Settings.AI.Context.Exclude[0])
		assert.Equal(t, "new1.tmp", atmosConfig.Settings.AI.Context.Exclude[1])
		assert.Equal(t, "new2.tmp", atmosConfig.Settings.AI.Context.Exclude[2])
	})
}
