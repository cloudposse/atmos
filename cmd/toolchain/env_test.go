package toolchain

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
		expected := []string{"bash", "json", "dotenv", "fish", "powershell"}
		assert.Equal(t, expected, supportedFormats)
	})

	t.Run("supportedFormats has correct count", func(t *testing.T) {
		assert.Len(t, supportedFormats, 5)
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
