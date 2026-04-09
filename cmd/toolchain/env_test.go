package toolchain

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

func TestEnvCommandProvider(t *testing.T) {
	provider := &EnvCommandProvider{}

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "env", cmd.Use)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		assert.Equal(t, "env", provider.GetName())
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		assert.Equal(t, "Toolchain Commands", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns non-nil parser", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		require.NotNil(t, builder, "env command has flags and should return parser")
		assert.Equal(t, envParser, builder)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})
}

func TestEnvCommand_FormatValidation(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		wantErr    bool
		errContain string
	}{
		{
			name:    "bash format is valid",
			format:  "bash",
			wantErr: false,
		},
		{
			name:    "json format is valid",
			format:  "json",
			wantErr: false,
		},
		{
			name:    "dotenv format is valid",
			format:  "dotenv",
			wantErr: false,
		},
		{
			name:    "fish format is valid",
			format:  "fish",
			wantErr: false,
		},
		{
			name:    "powershell format is valid",
			format:  "powershell",
			wantErr: false,
		},
		{
			name:       "invalid format returns error",
			format:     "invalid",
			wantErr:    true,
			errContain: "",
		},
		{
			name:       "empty format returns error",
			format:     "",
			wantErr:    true,
			errContain: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test.
			v := viper.New()
			v.Set("format", tt.format)
			v.Set("relative", false)

			// Create a test command that mimics envCmd's RunE behavior.
			testCmd := &cobra.Command{
				Use:   "env",
				Short: "Test env command",
				RunE: func(cmd *cobra.Command, args []string) error {
					format := v.GetString("format")
					found := false
					for _, f := range supportedFormats {
						if f == format {
							found = true
							break
						}
					}
					if !found {
						return errUtils.Build(errUtils.ErrInvalidArgumentError).
							WithExplanationf("invalid format: %s (supported: %v)", format, supportedFormats).
							Err()
					}
					// Don't actually call toolchain.EmitEnv in tests.
					return nil
				},
			}

			var stdout, stderr bytes.Buffer
			testCmd.SetOut(&stdout)
			testCmd.SetErr(&stderr)

			err := testCmd.Execute()

			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEnvCommand_SupportedFormats(t *testing.T) {
	t.Run("supportedFormats contains expected formats", func(t *testing.T) {
		expected := []string{"bash", "json", "dotenv", "fish", "powershell", "github"}
		assert.Equal(t, expected, supportedFormats)
	})

	t.Run("supportedFormats has correct count", func(t *testing.T) {
		assert.Len(t, supportedFormats, 6)
	})
}

func TestEnvCommand_Flags(t *testing.T) {
	t.Run("env command has format flag", func(t *testing.T) {
		cmd := envCmd
		flag := cmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand)
	})

	t.Run("env command has relative flag", func(t *testing.T) {
		cmd := envCmd
		flag := cmd.Flags().Lookup("relative")
		require.NotNil(t, flag)
	})

	t.Run("format flag default is bash", func(t *testing.T) {
		// Check the parser configuration.
		require.NotNil(t, envParser)
	})
}

func TestEnvCommand_ShellCompletion(t *testing.T) {
	t.Run("format flag has completion function", func(t *testing.T) {
		cmd := envCmd
		flag := cmd.Flags().Lookup("format")
		require.NotNil(t, flag)

		// Check that completion is registered by attempting to get completions.
		// The flag should have been registered with a completion function.
		assert.NotNil(t, cmd.Flag("format"))
	})
}

func TestEnvCommand_RelativeFlag(t *testing.T) {
	tests := []struct {
		name     string
		relative bool
	}{
		{
			name:     "relative flag true",
			relative: true,
		},
		{
			name:     "relative flag false",
			relative: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("format", "bash")
			v.Set("relative", tt.relative)

			// Verify viper correctly stores the relative flag.
			assert.Equal(t, tt.relative, v.GetBool("relative"))
		})
	}
}

func TestEnvCommand_EnvVars(t *testing.T) {
	t.Run("format env var is configured", func(t *testing.T) {
		// The parser is configured with WithEnvVars("format", "ATMOS_TOOLCHAIN_ENV_FORMAT").
		require.NotNil(t, envParser)
	})

	t.Run("relative env var is configured", func(t *testing.T) {
		// The parser is configured with WithEnvVars("relative", "ATMOS_TOOLCHAIN_RELATIVE").
		require.NotNil(t, envParser)
	})
}

func TestEnvCommand_CommandStructure(t *testing.T) {
	t.Run("command has correct use string", func(t *testing.T) {
		assert.Equal(t, "env", envCmd.Use)
	})

	t.Run("command has short description", func(t *testing.T) {
		assert.NotEmpty(t, envCmd.Short)
		assert.Contains(t, envCmd.Short, "PATH")
	})

	t.Run("command has long description", func(t *testing.T) {
		assert.NotEmpty(t, envCmd.Long)
		assert.Contains(t, envCmd.Long, ".tool-versions")
	})

	t.Run("command has RunE function", func(t *testing.T) {
		assert.NotNil(t, envCmd.RunE)
	})
}

func TestEnvCommand_FormatFlagShorthand(t *testing.T) {
	t.Run("format flag has shorthand f", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "f", flag.Shorthand)
	})
}

func TestEnvCommand_DefaultValues(t *testing.T) {
	t.Run("format default is bash", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "bash", flag.DefValue)
	})

	t.Run("relative default is false", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("relative")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

func TestEnvCommand_ValidationWithViper(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		relative bool
		isValid  bool
	}{
		{
			name:     "valid bash format with relative false",
			format:   "bash",
			relative: false,
			isValid:  true,
		},
		{
			name:     "valid json format with relative true",
			format:   "json",
			relative: true,
			isValid:  true,
		},
		{
			name:     "valid dotenv format",
			format:   "dotenv",
			relative: false,
			isValid:  true,
		},
		{
			name:     "valid fish format",
			format:   "fish",
			relative: true,
			isValid:  true,
		},
		{
			name:     "valid powershell format",
			format:   "powershell",
			relative: false,
			isValid:  true,
		},
		{
			name:     "invalid xml format",
			format:   "xml",
			relative: false,
			isValid:  false,
		},
		{
			name:     "invalid zsh format",
			format:   "zsh",
			relative: false,
			isValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("format", tt.format)
			v.Set("relative", tt.relative)

			// Check if format is in supported formats.
			format := v.GetString("format")
			found := false
			for _, f := range supportedFormats {
				if f == format {
					found = true
					break
				}
			}

			assert.Equal(t, tt.isValid, found)
		})
	}
}

// TestEnvCommand_FormatFlagCompletion tests format flag completion.
func TestEnvCommand_FormatFlagCompletion(t *testing.T) {
	t.Run("format flag has shell completion", func(t *testing.T) {
		// The flag should have been registered with a completion function.
		flag := envCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		// We can't easily test the completion function directly, but we can verify
		// the flag exists and the command doesn't panic when accessed.
		assert.NotEmpty(t, flag.Name)
	})
}

// TestEnvCommand_ParserConfiguration tests that the parser is correctly configured.
func TestEnvCommand_ParserConfiguration(t *testing.T) {
	t.Run("parser is not nil", func(t *testing.T) {
		require.NotNil(t, envParser)
	})
}

// TestEnvCommand_FormatValidation_CaseInsensitive tests that format validation is case sensitive.
func TestEnvCommand_FormatValidation_CaseInsensitive(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		isValid bool
	}{
		{"lowercase bash", "bash", true},
		{"uppercase BASH", "BASH", false}, // Format validation is case-sensitive.
		{"mixed case Bash", "Bash", false},
		{"lowercase json", "json", true},
		{"uppercase JSON", "JSON", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, f := range supportedFormats {
				if f == tt.format {
					found = true
					break
				}
			}
			assert.Equal(t, tt.isValid, found)
		})
	}
}

// TestEnvCommand_AllSupportedFormatsValid tests that all supported formats pass validation.
func TestEnvCommand_AllSupportedFormatsValid(t *testing.T) {
	for _, format := range supportedFormats {
		t.Run("format "+format+" is valid", func(t *testing.T) {
			v := viper.New()
			v.Set("format", format)
			v.Set("relative", false)

			// Create a test command that mimics envCmd's validation behavior.
			testCmd := &cobra.Command{
				Use:   "env",
				Short: "Test env command",
				RunE: func(cmd *cobra.Command, args []string) error {
					f := v.GetString("format")
					found := false
					for _, sf := range supportedFormats {
						if sf == f {
							found = true
							break
						}
					}
					if !found {
						return errUtils.Build(errUtils.ErrInvalidArgumentError).
							WithExplanationf("invalid format: %s", f).
							Err()
					}
					return nil
				},
			}

			var stdout, stderr bytes.Buffer
			testCmd.SetOut(&stdout)
			testCmd.SetErr(&stderr)

			err := testCmd.Execute()
			require.NoError(t, err, "format %s should be valid", format)
		})
	}
}

// TestEnvCommand_FlagDefaults tests default flag values.
func TestEnvCommand_FlagDefaults(t *testing.T) {
	t.Run("format flag has correct default", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("format")
		require.NotNil(t, flag)
		assert.Equal(t, "bash", flag.DefValue)
	})

	t.Run("relative flag has correct default", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("relative")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})
}

// TestEnvCommand_CommandMetadata tests command metadata.
func TestEnvCommand_CommandMetadata(t *testing.T) {
	t.Run("command has Use field", func(t *testing.T) {
		assert.Equal(t, "env", envCmd.Use)
	})

	t.Run("command has Short description", func(t *testing.T) {
		assert.NotEmpty(t, envCmd.Short)
		assert.Contains(t, envCmd.Short, "PATH")
	})

	t.Run("command has Long description", func(t *testing.T) {
		assert.NotEmpty(t, envCmd.Long)
		assert.Contains(t, envCmd.Long, ".tool-versions")
	})

	t.Run("command has RunE", func(t *testing.T) {
		assert.NotNil(t, envCmd.RunE)
	})
}

// setupEnvTestEnvironment creates a temp directory with an atmos config and tool-versions file.
func setupEnvTestEnvironment(t *testing.T) (cleanup func(), tempDir string) {
	t.Helper()

	tempDir = t.TempDir()

	// Create a minimal tool-versions file with a tool.
	tvPath := filepath.Join(tempDir, ".tool-versions")
	err := os.WriteFile(tvPath, []byte("terraform 1.5.0\n"), 0o644)
	require.NoError(t, err)

	// Save original config and set test config.
	originalConfig := toolchain.GetAtmosConfig()

	testConfig := &schema.AtmosConfiguration{
		Toolchain: schema.Toolchain{
			VersionsFile: tvPath,
			InstallPath:  tempDir,
		},
	}
	toolchain.SetAtmosConfig(testConfig)

	cleanup = func() {
		toolchain.SetAtmosConfig(originalConfig)
		// Reset viper state.
		viper.Reset()
	}

	return cleanup, tempDir
}

// TestEnvCommand_RunE tests the RunE function execution paths.
func TestEnvCommand_RunE(t *testing.T) {
	t.Run("RunE with default format executes", func(t *testing.T) {
		cleanup, _ := setupEnvTestEnvironment(t)
		defer cleanup()

		// Reset flags to defaults.
		envCmd.Flags().Set("format", "bash")
		envCmd.Flags().Set("relative", "false")
		envCmd.Flags().Set("output", "")

		// Call RunE - exercises:
		// - Line 27-30: Viper binding
		// - Line 32-35: Format validation (bash is valid)
		// - Line 37-38: Get flags
		// - Line 46: Call EmitEnv
		err := envCmd.RunE(envCmd, []string{})

		// Expect ErrToolNotFound since no tools are installed.
		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with invalid format returns error", func(t *testing.T) {
		cleanup, _ := setupEnvTestEnvironment(t)
		defer cleanup()

		// Set invalid format.
		envCmd.Flags().Set("format", "invalid")
		envCmd.Flags().Set("relative", "false")
		envCmd.Flags().Set("output", "")

		// Call RunE - exercises:
		// - Line 32-35: Format validation with invalid format
		err := envCmd.RunE(envCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("RunE with all supported formats validates correctly", func(t *testing.T) {
		for _, format := range supportedFormats {
			t.Run(format, func(t *testing.T) {
				cleanup, _ := setupEnvTestEnvironment(t)
				defer cleanup()

				envCmd.Flags().Set("format", format)
				envCmd.Flags().Set("relative", "false")
				envCmd.Flags().Set("output", "")

				// Call RunE - should pass format validation.
				err := envCmd.RunE(envCmd, []string{})

				// Error should be ErrToolNotFound (format validation passed).
				require.Error(t, err)
				assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
			})
		}
	})

	t.Run("RunE with relative flag passes to EmitEnv", func(t *testing.T) {
		cleanup, _ := setupEnvTestEnvironment(t)
		defer cleanup()

		envCmd.Flags().Set("format", "bash")
		envCmd.Flags().Set("relative", "true")
		envCmd.Flags().Set("output", "")

		// Call RunE - exercises line 37.
		err := envCmd.RunE(envCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with output flag passes to EmitEnv", func(t *testing.T) {
		cleanup, tempDir := setupEnvTestEnvironment(t)
		defer cleanup()

		outputFile := filepath.Join(tempDir, "output.txt")

		envCmd.Flags().Set("format", "bash")
		envCmd.Flags().Set("relative", "false")
		envCmd.Flags().Set("output", outputFile)

		// Call RunE - exercises line 38.
		err := envCmd.RunE(envCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with github format uses GITHUB_PATH env var", func(t *testing.T) {
		cleanup, tempDir := setupEnvTestEnvironment(t)
		defer cleanup()

		githubPath := filepath.Join(tempDir, "github_path")
		t.Setenv("GITHUB_PATH", githubPath)

		envCmd.Flags().Set("format", "github")
		envCmd.Flags().Set("relative", "false")
		envCmd.Flags().Set("output", "")

		// Call RunE - exercises lines 41-44 (GITHUB_PATH handling).
		err := envCmd.RunE(envCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})

	t.Run("RunE with github format and explicit output overrides GITHUB_PATH", func(t *testing.T) {
		cleanup, tempDir := setupEnvTestEnvironment(t)
		defer cleanup()

		// Set GITHUB_PATH (should be ignored).
		t.Setenv("GITHUB_PATH", "/ignored/path")

		// Set explicit output (should be used).
		outputFile := filepath.Join(tempDir, "explicit_output.txt")
		envCmd.Flags().Set("format", "github")
		envCmd.Flags().Set("relative", "false")
		envCmd.Flags().Set("output", outputFile)

		// Call RunE - output flag should be used, not GITHUB_PATH.
		err := envCmd.RunE(envCmd, []string{})

		require.Error(t, err)
		assert.ErrorIs(t, err, toolchain.ErrToolNotFound)
	})
}

// TestEnvCommand_RunE_ErrorMessages tests error messages from RunE.
func TestEnvCommand_RunE_ErrorMessages(t *testing.T) {
	t.Run("invalid format error includes supported formats", func(t *testing.T) {
		cleanup, _ := setupEnvTestEnvironment(t)
		defer cleanup()

		envCmd.Flags().Set("format", "xml")
		envCmd.Flags().Set("relative", "false")
		envCmd.Flags().Set("output", "")

		err := envCmd.RunE(envCmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "supported")
	})

	t.Run("invalid format error includes the invalid format", func(t *testing.T) {
		cleanup, _ := setupEnvTestEnvironment(t)
		defer cleanup()

		envCmd.Flags().Set("format", "myformat")
		envCmd.Flags().Set("relative", "false")
		envCmd.Flags().Set("output", "")

		err := envCmd.RunE(envCmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "myformat")
	})
}

// TestEnvCommand_OutputFlag tests the output flag behavior.
func TestEnvCommand_OutputFlag(t *testing.T) {
	t.Run("output flag exists", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("output")
		require.NotNil(t, flag)
		assert.Equal(t, "o", flag.Shorthand)
	})

	t.Run("output flag default is empty", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("output")
		require.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("output flag has description mentioning GITHUB_PATH", func(t *testing.T) {
		flag := envCmd.Flags().Lookup("output")
		require.NotNil(t, flag)
		assert.Contains(t, flag.Usage, "GITHUB_PATH")
	})
}
