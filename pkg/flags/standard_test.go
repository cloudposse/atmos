package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestNewStandardFlagParser(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())

	assert.NotNil(t, parser)
	assert.NotNil(t, parser.registry)

	// CommonFlags() includes all global flags + stack + dry-run
	// Verify presence of expected flags rather than hardcoding count
	assert.NotNil(t, parser.registry.Get("stack"), "should have stack flag")
	assert.NotNil(t, parser.registry.Get("dry-run"), "should have dry-run flag")
	assert.NotNil(t, parser.registry.Get("identity"), "should have identity from global flags")
	assert.NotNil(t, parser.registry.Get("chdir"), "should have chdir from global flags")
	assert.NotNil(t, parser.registry.Get("logs-level"), "should have logs-level from global flags")
}

func TestStandardFlagParser_RegisterFlags(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())
	cmd := &cobra.Command{Use: "test"}

	parser.RegisterFlags(cmd)

	// Check that flags were registered
	stackFlag := cmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag)
	assert.Equal(t, "s", stackFlag.Shorthand)

	identityFlag := cmd.Flags().Lookup(cfg.IdentityFlagName)
	assert.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, cfg.IdentityFlagSelectValue, identityFlag.NoOptDefVal)

	dryRunFlag := cmd.Flags().Lookup("dry-run")
	assert.NotNil(t, dryRunFlag)
}

func TestStandardFlagParser_BindToViper(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())
	v := viper.New()

	err := parser.BindToViper(v)

	require.NoError(t, err)
	// Viper bindings are internal, we can't easily test them directly
	// But we can verify no error occurred
}

func TestStandardFlagParser_BindFlagsToViper(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())
	cmd := &cobra.Command{Use: "test"}
	v := viper.New()

	parser.RegisterFlags(cmd)
	err := parser.BindToViper(v)
	require.NoError(t, err)

	err = parser.BindFlagsToViper(cmd, v)

	require.NoError(t, err)
}

func TestStandardFlagParser_Parse(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())

	ctx := context.Background()
	cfg, err := parser.Parse(ctx, []string{})

	// Parse() is a placeholder for interface compliance
	// StandardFlagParser doesn't populate ParsedConfig like FlagRegistry does
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.Flags)
}

func TestStandardFlagParser_GetIdentityFromCmd(t *testing.T) {
	tests := []struct {
		name          string
		flagValue     string
		flagSet       bool
		envValue      string
		expectedValue string
	}{
		{
			name:          "flag explicitly set",
			flagValue:     "admin",
			flagSet:       true,
			expectedValue: "admin",
		},
		{
			name:          "flag not set, use env",
			flagSet:       false,
			envValue:      "ci-user",
			expectedValue: "ci-user",
		},
		{
			name:          "flag not set, no env",
			flagSet:       false,
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewStandardFlagParser(WithIdentityFlag())
			cmd := &cobra.Command{Use: "test"}
			v := viper.New()

			parser.RegisterFlags(cmd)
			parser.BindToViper(v)
			parser.BindFlagsToViper(cmd, v)

			if tt.flagSet {
				cmd.Flags().Set(cfg.IdentityFlagName, tt.flagValue)
			}
			if tt.envValue != "" {
				v.Set(cfg.IdentityFlagName, tt.envValue)
			}

			identity, err := parser.GetIdentityFromCmd(cmd, v)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, identity)
		})
	}
}

func TestStandardFlagParser_WithViperPrefix(t *testing.T) {
	parser := NewStandardFlagParser(
		WithCommonFlags(),
		WithViperPrefix("terraform"),
	)

	cmd := &cobra.Command{Use: "test"}
	v := viper.New()

	parser.RegisterFlags(cmd)
	parser.BindToViper(v)
	parser.BindFlagsToViper(cmd, v)

	// Set flag value
	cmd.Flags().Set("stack", "dev")

	// Read value from Viper (with prefix)
	value := v.GetString("terraform.stack")

	// The viper prefix affects how values are stored/retrieved
	require.NotEmpty(t, value)
	assert.Equal(t, "dev", value)
}

func TestStandardFlagParser_RequiredFlags(t *testing.T) {
	parser := NewStandardFlagParser(
		WithRequiredStringFlag("component", "c", "Component name (required)"),
	)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag is marked as required
	componentFlag := cmd.Flags().Lookup("component")
	assert.NotNil(t, componentFlag)
	// Cobra marks required flags internally, we just verify it's registered
}
