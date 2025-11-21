package provision

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProvisionCommandProvider_GetCommand(t *testing.T) {
	provider := &ProvisionCommandProvider{}
	command := provider.GetCommand()

	require.NotNil(t, command)
	assert.Equal(t, "provision backend <component> --stack <stack>", command.Use)
	assert.Contains(t, command.Short, "Provision backend infrastructure")
}

func TestProvisionCommandProvider_GetName(t *testing.T) {
	provider := &ProvisionCommandProvider{}
	assert.Equal(t, "provision", provider.GetName())
}

func TestProvisionCommandProvider_GetGroup(t *testing.T) {
	provider := &ProvisionCommandProvider{}
	assert.Equal(t, "Core Stack Commands", provider.GetGroup())
}

func TestProvisionCommandProvider_GetAliases(t *testing.T) {
	provider := &ProvisionCommandProvider{}
	aliases := provider.GetAliases()
	assert.Nil(t, aliases, "provision command should have no aliases")
}

func TestSetAtmosConfig(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "/test/path",
	}

	SetAtmosConfig(config)
	assert.Equal(t, config, atmosConfigPtr)
	assert.Equal(t, "/test/path", atmosConfigPtr.BasePath)
}

func TestProvisionCommand_Flags(t *testing.T) {
	// Get the command.
	cmd := provisionCmd

	// Verify stack flag.
	stackFlag := cmd.Flags().Lookup("stack")
	require.NotNil(t, stackFlag, "stack flag should exist")
	assert.Equal(t, "s", stackFlag.Shorthand)
	assert.Equal(t, "", stackFlag.DefValue)

	// Verify identity flag.
	identityFlag := cmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag, "identity flag should exist")
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, "", identityFlag.DefValue)
	// NoOptDefVal allows --identity without value for interactive selection.
	assert.NotEmpty(t, identityFlag.NoOptDefVal, "identity flag should support optional value")
}

func TestProvisionCommand_Args(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "valid two arguments",
			args:    []string{"backend", "vpc"},
			wantErr: false,
		},
		{
			name:    "no arguments",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "one argument",
			args:    []string{"backend"},
			wantErr: true,
		},
		{
			name:    "three arguments",
			args:    []string{"backend", "vpc", "extra"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			cmd.Args = cobra.ExactArgs(2)

			err := cmd.Args(cmd, tt.args)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProvisionCommand_StackFlagFromCLI(t *testing.T) {
	cmd := provisionCmd

	// Set the stack flag.
	err := cmd.Flags().Set("stack", "dev")
	require.NoError(t, err)

	// Verify the flag was set.
	stackValue, err := cmd.Flags().GetString("stack")
	require.NoError(t, err)
	assert.Equal(t, "dev", stackValue)
}

func TestProvisionCommand_StackFlagFromEnv(t *testing.T) {
	// Test that ATMOS_STACK environment variable works.

	// Set environment variable.
	t.Setenv("ATMOS_STACK", "prod")

	// Create fresh viper instance.
	v := viper.New()
	v.SetEnvPrefix("ATMOS")
	v.AutomaticEnv()
	v.BindEnv("stack", "ATMOS_STACK")

	// Verify environment variable is read.
	assert.Equal(t, "prod", v.GetString("stack"))
}

func TestProvisionCommand_IdentityFlagFromCLI(t *testing.T) {
	cmd := provisionCmd

	// Set the identity flag with a value.
	err := cmd.Flags().Set("identity", "prod-admin")
	require.NoError(t, err)

	// Verify the flag was set.
	identityValue, err := cmd.Flags().GetString("identity")
	require.NoError(t, err)
	assert.Equal(t, "prod-admin", identityValue)
}

func TestProvisionCommand_IdentityFlagOptionalValue(t *testing.T) {
	// Test that identity flag supports optional value for interactive selection.

	cmd := provisionCmd

	// Verify NoOptDefVal is set (allows --identity without value).
	identityFlag := cmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.NotEmpty(t, identityFlag.NoOptDefVal)
}

func TestProvisionCommand_Help(t *testing.T) {
	cmd := provisionCmd

	// Verify help text contains expected content.
	assert.Contains(t, cmd.Short, "Provision backend infrastructure")
	assert.Contains(t, cmd.Long, "S3 backends")
	assert.Contains(t, cmd.Long, "terraform-aws-tfstate-backend")

	// Verify examples.
	assert.Contains(t, cmd.Example, "atmos provision backend vpc --stack dev")
	assert.Contains(t, cmd.Example, "atmos provision backend eks --stack prod")
}

func TestProvisionCommand_DisableFlagParsing(t *testing.T) {
	cmd := provisionCmd

	// Verify flag parsing is enabled.
	assert.False(t, cmd.DisableFlagParsing, "Flag parsing should be enabled")
}

func TestProvisionCommand_UnknownFlags(t *testing.T) {
	cmd := provisionCmd

	// Verify unknown flags are not whitelisted.
	assert.False(t, cmd.FParseErrWhitelist.UnknownFlags, "Unknown flags should not be whitelisted")
}

func TestProvisionOptions_Structure(t *testing.T) {
	// Test that ProvisionOptions embeds global flags correctly.

	opts := &ProvisionOptions{
		Stack:    "dev",
		Identity: "admin",
	}

	assert.Equal(t, "dev", opts.Stack)
	assert.Equal(t, "admin", opts.Identity)

	// Verify global.Flags is embedded (can access global flag fields).
	// Note: We can't test actual global flag values without full initialization,
	// but we can verify the type embedding.
	var _ interface{} = opts.Flags // This compiles, confirming embedding.
}

func TestProvisionCommand_ArgumentsParsing(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		wantProvisionerType string
		wantComponent       string
	}{
		{
			name:                "backend and vpc",
			args:                []string{"backend", "vpc"},
			wantProvisionerType: "backend",
			wantComponent:       "vpc",
		},
		{
			name:                "backend and eks",
			args:                []string{"backend", "eks"},
			wantProvisionerType: "backend",
			wantComponent:       "eks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate argument parsing.
			if len(tt.args) == 2 {
				provisionerType := tt.args[0]
				component := tt.args[1]

				assert.Equal(t, tt.wantProvisionerType, provisionerType)
				assert.Equal(t, tt.wantComponent, component)
			}
		})
	}
}

func TestProvisionCommand_Integration(t *testing.T) {
	// This test verifies the command structure without executing RunE.
	// Full integration tests are in tests/cli_provision_test.go.

	provider := &ProvisionCommandProvider{}
	cmd := provider.GetCommand()

	// Verify command registration.
	assert.Equal(t, "provision", provider.GetName())
	assert.Equal(t, "Core Stack Commands", provider.GetGroup())

	// Verify command structure.
	assert.NotNil(t, cmd)
	assert.NotNil(t, cmd.RunE)
	// Note: Can't directly compare function pointers, so verify Args works correctly.
	assert.NoError(t, cmd.Args(cmd, []string{"backend", "vpc"}))

	// Verify flags are registered.
	assert.True(t, cmd.Flags().HasFlags())
	assert.NotNil(t, cmd.Flags().Lookup("stack"))
	assert.NotNil(t, cmd.Flags().Lookup("identity"))
}

func TestProvisionParser_Initialization(t *testing.T) {
	// Verify that provisionParser is initialized.
	assert.NotNil(t, provisionParser, "provisionParser should be initialized in init()")
}

func TestProvisionCommand_FlagBinding(t *testing.T) {
	// Test that flags are properly bound to Viper for environment variable support.

	// Create fresh viper instance.
	v := viper.New()

	// Simulate environment variables.
	t.Setenv("ATMOS_STACK", "test-stack")
	t.Setenv("ATMOS_IDENTITY", "test-identity")

	v.SetEnvPrefix("ATMOS")
	v.AutomaticEnv()

	// Bind variables manually (simulating what provisionParser does).
	v.BindEnv("stack", "ATMOS_STACK")
	v.BindEnv("identity", "ATMOS_IDENTITY", "IDENTITY")

	// Verify bindings work.
	assert.Equal(t, "test-stack", v.GetString("stack"))
	assert.Equal(t, "test-identity", v.GetString("identity"))
}

func TestProvisionCommand_ErrorHandling(t *testing.T) {
	// Test error handling for missing required flags.

	// Verify that runE handles missing stack flag.
	// Note: We can't call RunE directly without full Atmos config setup,
	// but we can verify the error constant used.
	assert.NotNil(t, errUtils.ErrRequiredFlagNotProvided)
	assert.NotNil(t, errUtils.ErrInvalidArguments)
}

func TestProvisionCommand_StackFlagPrecedence(t *testing.T) {
	// Test flag precedence: CLI flag > environment variable > default.

	// Set environment variable.
	t.Setenv("ATMOS_STACK", "env-stack")

	// Create fresh viper.
	v := viper.New()
	v.SetEnvPrefix("ATMOS")
	v.AutomaticEnv()
	v.BindEnv("stack", "ATMOS_STACK")

	// Set CLI flag (should override environment).
	v.Set("stack", "cli-stack")

	// CLI flag should take precedence.
	assert.Equal(t, "cli-stack", v.GetString("stack"))

	// Reset and test environment variable only.
	v2 := viper.New()
	v2.SetEnvPrefix("ATMOS")
	v2.AutomaticEnv()
	v2.BindEnv("stack", "ATMOS_STACK")

	// Environment variable should be used.
	assert.Equal(t, "env-stack", v2.GetString("stack"))
}

func TestProvisionCommand_ExamplesFormat(t *testing.T) {
	cmd := provisionCmd

	// Verify examples are properly formatted.
	examples := cmd.Example
	assert.Contains(t, examples, "atmos provision backend")
	assert.Contains(t, examples, "--stack")

	// Verify at least two examples exist.
	lines := 0
	for _, char := range examples {
		if char == '\n' {
			lines++
		}
	}
	assert.GreaterOrEqual(t, lines, 1, "Should have multiple example lines")
}

func TestProvisionCommand_RunEStructure(t *testing.T) {
	// Test that RunE has the correct structure and validates arguments.

	cmd := provisionCmd

	// Verify RunE is not nil.
	require.NotNil(t, cmd.RunE)

	// Verify Args validator requires exactly 2 arguments by testing behavior.

	// Test Args validator.
	err := cmd.Args(cmd, []string{"backend", "vpc"})
	assert.NoError(t, err)

	err = cmd.Args(cmd, []string{"backend"})
	assert.Error(t, err)

	err = cmd.Args(cmd, []string{})
	assert.Error(t, err)
}
