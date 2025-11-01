package flagparser

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPassThroughFlagParser_SplitAtDoubleDash(t *testing.T) {
	parser := NewPassThroughFlagParser()

	tests := []struct {
		name             string
		args             []string
		expectedBefore   []string
		expectedAfter    []string
		expectedAfterNil bool
	}{
		{
			name:           "with separator",
			args:           []string{"atmos", "terraform", "plan", "vpc", "-s", "dev", "--", "-var", "foo=bar"},
			expectedBefore: []string{"atmos", "terraform", "plan", "vpc", "-s", "dev"},
			expectedAfter:  []string{"-var", "foo=bar"},
		},
		{
			name:             "no separator",
			args:             []string{"atmos", "terraform", "plan", "vpc", "-s", "dev"},
			expectedBefore:   []string{"atmos", "terraform", "plan", "vpc", "-s", "dev"},
			expectedAfter:    nil,
			expectedAfterNil: true,
		},
		{
			name:           "separator at end with no trailing args",
			args:           []string{"atmos", "terraform", "plan", "--"},
			expectedBefore: []string{"atmos", "terraform", "plan"},
			expectedAfter:  []string{},
		},
		{
			name:           "separator at beginning",
			args:           []string{"--", "terraform", "plan"},
			expectedBefore: []string{},
			expectedAfter:  []string{"terraform", "plan"},
		},
		{
			name:           "only separator",
			args:           []string{"--"},
			expectedBefore: []string{},
			expectedAfter:  []string{},
		},
		{
			name:             "empty args",
			args:             []string{},
			expectedBefore:   []string{},
			expectedAfter:    nil,
			expectedAfterNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := parser.SplitAtDoubleDash(tt.args)

			assert.Equal(t, tt.expectedBefore, before)
			if tt.expectedAfterNil {
				assert.Nil(t, after)
			} else {
				assert.Equal(t, tt.expectedAfter, after)
			}
		})
	}
}

func TestPassThroughFlagParser_ExtractAtmosFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedFlags map[string]interface{}
		expectedArgs  []string
	}{
		{
			name: "extract stack flag (long form with equals)",
			args: []string{"plan", "vpc", "--stack=dev", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
			},
			expectedArgs: []string{"plan", "vpc", "-var", "foo=bar"},
		},
		{
			name: "extract stack flag (long form with space)",
			args: []string{"plan", "vpc", "--stack", "dev", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
			},
			expectedArgs: []string{"plan", "vpc", "-var", "foo=bar"},
		},
		{
			name: "extract stack flag (short form with equals)",
			args: []string{"plan", "vpc", "-s=dev", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"stack": "dev", // Shorthand normalized to full name
			},
			expectedArgs: []string{"plan", "vpc", "-var", "foo=bar"},
		},
		{
			name: "extract stack flag (short form with space)",
			args: []string{"plan", "vpc", "-s", "dev", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"stack": "dev", // Shorthand normalized to full name
			},
			expectedArgs: []string{"plan", "vpc", "-var", "foo=bar"},
		},
		{
			name: "extract multiple Atmos flags",
			args: []string{"plan", "vpc", "--stack", "dev", "--dry-run", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"stack":   "dev",
				"dry-run": true,
			},
			expectedArgs: []string{"plan", "vpc", "-var", "foo=bar"},
		},
		{
			name: "extract identity flag with value",
			args: []string{"plan", "vpc", "--identity", "admin", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"identity": "admin",
			},
			expectedArgs: []string{"plan", "vpc", "-var", "foo=bar"},
		},
		{
			name: "extract identity flag without value (NoOptDefVal)",
			args: []string{"plan", "vpc", "--identity", "--dry-run"},
			expectedFlags: map[string]interface{}{
				"identity": "__SELECT__",
				"dry-run":  true,
			},
			expectedArgs: []string{"plan", "vpc"},
		},
		{
			name:          "no Atmos flags",
			args:          []string{"plan", "vpc", "-var", "foo=bar", "-out=plan.tfplan"},
			expectedFlags: map[string]interface{}{},
			expectedArgs:  []string{"plan", "vpc", "-var", "foo=bar", "-out=plan.tfplan"},
		},
		{
			name: "mixed Atmos and tool flags",
			args: []string{"plan", "vpc", "-s", "dev", "-var", "foo=bar", "--dry-run", "-out=plan.tfplan"},
			expectedFlags: map[string]interface{}{
				"stack":   "dev", // Shorthand normalized to full name
				"dry-run": true,
			},
			expectedArgs: []string{"plan", "vpc", "-var", "foo=bar", "-out=plan.tfplan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewPassThroughFlagParser(WithCommonFlags())

			flags, args, err := parser.ExtractAtmosFlags(tt.args)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedFlags, flags)
			assert.Equal(t, tt.expectedArgs, args)
		})
	}
}

func TestPassThroughFlagParser_ExtractPositionalArgs(t *testing.T) {
	parser := NewPassThroughFlagParser()

	tests := []struct {
		name               string
		args               []string
		expectedCount      int
		expectedPositional []string
		expectedRemaining  []string
	}{
		{
			name:               "extract 2 positional args",
			args:               []string{"plan", "vpc", "-var", "foo=bar"},
			expectedCount:      2,
			expectedPositional: []string{"plan", "vpc"},
			expectedRemaining:  []string{"-var", "foo=bar"},
		},
		{
			name:               "extract 1 positional arg",
			args:               []string{"plan", "-var", "foo=bar"},
			expectedCount:      2,
			expectedPositional: []string{"plan"},
			expectedRemaining:  []string{"-var", "foo=bar"},
		},
		{
			name:               "no positional args (only flags)",
			args:               []string{"-var", "foo=bar", "-out=plan.tfplan"},
			expectedCount:      2,
			expectedPositional: []string{},
			expectedRemaining:  []string{"-var", "foo=bar", "-out=plan.tfplan"},
		},
		{
			name:               "more positional args than expected",
			args:               []string{"plan", "vpc", "extra", "-var", "foo=bar"},
			expectedCount:      2,
			expectedPositional: []string{"plan", "vpc"},
			expectedRemaining:  []string{"extra", "-var", "foo=bar"},
		},
		{
			name:               "empty args",
			args:               []string{},
			expectedCount:      2,
			expectedPositional: []string{},
			expectedRemaining:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			positional, remaining, err := parser.ExtractPositionalArgs(tt.args, tt.expectedCount)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedPositional, positional)
			assert.Equal(t, tt.expectedRemaining, remaining)
		})
	}
}

func TestPassThroughFlagParser_Parse(t *testing.T) {
	tests := []struct {
		name                    string
		args                    []string
		expectedAtmosFlags      map[string]interface{}
		expectedPassThroughArgs []string
		expectedSubCommand      string
		expectedComponentName   string
	}{
		{
			name: "explicit mode with separator",
			args: []string{"plan", "vpc", "-s", "dev", "--", "-var", "foo=bar", "-out=plan.tfplan"},
			expectedAtmosFlags: map[string]interface{}{
				"stack": "dev", // Shorthand normalized to full name
			},
			expectedPassThroughArgs: []string{"-var", "foo=bar", "-out=plan.tfplan"},
			expectedSubCommand:      "plan",
			expectedComponentName:   "vpc",
		},
		{
			name: "implicit mode without separator",
			args: []string{"plan", "vpc", "-s", "dev", "-var", "foo=bar"},
			expectedAtmosFlags: map[string]interface{}{
				"stack": "dev", // Shorthand normalized to full name
			},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			expectedSubCommand:      "plan",
			expectedComponentName:   "vpc",
		},
		{
			name: "with identity flag",
			args: []string{"plan", "vpc", "--identity", "admin", "--", "-var", "foo=bar"},
			expectedAtmosFlags: map[string]interface{}{
				"identity": "admin",
			},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			expectedSubCommand:      "plan",
			expectedComponentName:   "vpc",
		},
		{
			name: "identity flag without value (interactive)",
			args: []string{"plan", "vpc", "--identity", "--", "-var", "foo=bar"},
			expectedAtmosFlags: map[string]interface{}{
				"identity": "__SELECT__",
			},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			expectedSubCommand:      "plan",
			expectedComponentName:   "vpc",
		},
		{
			name: "multiple Atmos flags",
			args: []string{"plan", "vpc", "--stack", "dev", "--dry-run", "--", "-var", "foo=bar"},
			expectedAtmosFlags: map[string]interface{}{
				"stack":   "dev",
				"dry-run": true,
			},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			expectedSubCommand:      "plan",
			expectedComponentName:   "vpc",
		},
		{
			name:                    "no Atmos flags",
			args:                    []string{"plan", "vpc", "--", "-var", "foo=bar"},
			expectedAtmosFlags:      map[string]interface{}{},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			expectedSubCommand:      "plan",
			expectedComponentName:   "vpc",
		},
		{
			name: "complex real-world example",
			args: []string{
				"plan", "vpc",
				"--stack", "prod",
				"--identity", "admin",
				"--dry-run",
				"--",
				"-var", "region=us-east-1",
				"-var-file=common.tfvars",
				"-out=plan.tfplan",
			},
			expectedAtmosFlags: map[string]interface{}{
				"stack":    "prod",
				"identity": "admin",
				"dry-run":  true,
			},
			expectedPassThroughArgs: []string{
				"-var", "region=us-east-1",
				"-var-file=common.tfvars",
				"-out=plan.tfplan",
			},
			expectedSubCommand:    "plan",
			expectedComponentName: "vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewPassThroughFlagParser(WithCommonFlags())

			cfg, err := parser.Parse(context.Background(), tt.args)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedAtmosFlags, cfg.AtmosFlags)
			assert.Equal(t, tt.expectedPassThroughArgs, cfg.PassThroughArgs)
			assert.Equal(t, tt.expectedSubCommand, cfg.SubCommand)
			assert.Equal(t, tt.expectedComponentName, cfg.ComponentName)
		})
	}
}

func TestPassThroughFlagParser_Parse_EdgeCases(t *testing.T) {
	tests := []struct {
		name                    string
		args                    []string
		expectedAtmosFlags      map[string]interface{}
		expectedPassThroughArgs []string
		description             string
	}{
		{
			name:                    "duplicate flag (long and short form)",
			args:                    []string{"plan", "vpc", "-s", "dev", "--stack", "prod", "--", "-var", "foo=bar"},
			expectedAtmosFlags:      map[string]interface{}{"stack": "prod"},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			description:             "When both -s and --stack provided, last value wins",
		},
		{
			name:                    "flag value with equals sign",
			args:                    []string{"plan", "vpc", "--stack=prod/us-east-1", "--", "-var", "foo=bar"},
			expectedAtmosFlags:      map[string]interface{}{"stack": "prod/us-east-1"},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			description:             "Stack names with special characters should be preserved",
		},
		{
			name:                    "flag value with dashes",
			args:                    []string{"plan", "vpc", "--stack=my-stack-name", "--", "-var", "foo=bar"},
			expectedAtmosFlags:      map[string]interface{}{"stack": "my-stack-name"},
			expectedPassThroughArgs: []string{"-var", "foo=bar"},
			description:             "Stack names with dashes should be preserved",
		},
		{
			name:                    "empty pass-through args",
			args:                    []string{"plan", "vpc", "--stack", "dev", "--"},
			expectedAtmosFlags:      map[string]interface{}{"stack": "dev"},
			expectedPassThroughArgs: []string{},
			description:             "Empty pass-through args should work",
		},
		{
			name:                    "all flags before separator",
			args:                    []string{"--stack", "dev", "--dry-run", "--", "plan", "vpc"},
			expectedAtmosFlags:      map[string]interface{}{"stack": "dev", "dry-run": true},
			expectedPassThroughArgs: []string{},
			description:             "Atmos flags before separator, positional args after",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewPassThroughFlagParser(WithCommonFlags())

			cfg, err := parser.Parse(context.Background(), tt.args)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedAtmosFlags, cfg.AtmosFlags, tt.description)
			assert.Equal(t, tt.expectedPassThroughArgs, cfg.PassThroughArgs, tt.description)
		})
	}
}

func TestPassThroughFlagParser_RegisterFlags(t *testing.T) {
	parser := NewPassThroughFlagParser(WithTerraformFlags())
	cmd := &cobra.Command{Use: "terraform"}

	parser.RegisterFlags(cmd)

	// Verify flags were registered
	assert.NotNil(t, cmd.Flags().Lookup("stack"))
	assert.NotNil(t, cmd.Flags().Lookup("identity"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("upload-status"))
}

func TestPassThroughFlagParser_BindToViper(t *testing.T) {
	parser := NewPassThroughFlagParser(WithCommonFlags())
	v := viper.New()

	err := parser.BindToViper(v)

	require.NoError(t, err)
}

func TestPassThroughFlagParser_BindFlagsToViper(t *testing.T) {
	parser := NewPassThroughFlagParser(WithCommonFlags())
	cmd := &cobra.Command{Use: "terraform"}
	v := viper.New()

	parser.RegisterFlags(cmd)
	parser.BindToViper(v)

	err := parser.BindFlagsToViper(cmd, v)

	require.NoError(t, err)
}

func TestPassThroughFlagParser_GetIdentityFromCmd(t *testing.T) {
	parser := NewPassThroughFlagParser(WithIdentityFlag())
	cmd := &cobra.Command{Use: "terraform"}
	v := viper.New()

	parser.RegisterFlags(cmd)
	parser.BindToViper(v)
	parser.BindFlagsToViper(cmd, v)

	// Set identity flag
	cmd.Flags().Set("identity", "admin")

	identity, err := parser.GetIdentityFromCmd(cmd, v)

	require.NoError(t, err)
	assert.Equal(t, "admin", identity)
}
