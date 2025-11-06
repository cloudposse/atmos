package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestParser creates a PassThroughFlagParser with common flags + identity flag for testing.
// WithCommonFlags() now includes GlobalFlags which has identity, so no need to add it separately.
func createTestParser() *PassThroughFlagParser {
	parser := NewPassThroughFlagParser(
		WithCommonFlags(),
	)

	// Set NoOptDefVal for identity flag to support --identity without value.
	// Identity is included in GlobalFlags which is part of WithCommonFlags().
	identityFlag := parser.GetRegistry().Get("identity")
	if identityFlag != nil {
		if strFlag, ok := identityFlag.(*StringFlag); ok {
			strFlag.NoOptDefVal = "__SELECT__"
		}
	}

	return parser
}

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
			parser := createTestParser()

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
		expectedFlags           map[string]interface{}
		expectedSeparatedArgs []string
		expectedPositionalArgs  []string
	}{
		{
			name: "explicit mode with separator",
			args: []string{"plan", "vpc", "-s", "dev", "--", "-var", "foo=bar", "-out=plan.tfplan"},
			expectedFlags: map[string]interface{}{
				"stack": "dev", // Shorthand normalized to full name
			},
			expectedSeparatedArgs: []string{"-var", "foo=bar", "-out=plan.tfplan"},
			expectedPositionalArgs:  []string{"plan", "vpc"},
		},
		{
			name: "implicit mode without separator",
			args: []string{"plan", "vpc", "-s", "dev", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"stack": "dev", // Shorthand normalized to full name
			},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			expectedPositionalArgs:  []string{"plan", "vpc"},
		},
		{
			name: "with identity flag",
			args: []string{"plan", "vpc", "--identity", "admin", "--", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"identity": "admin",
			},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			expectedPositionalArgs:  []string{"plan", "vpc"},
		},
		{
			name: "identity flag without value (interactive)",
			args: []string{"plan", "vpc", "--identity", "--", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"identity": "__SELECT__",
			},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			expectedPositionalArgs:  []string{"plan", "vpc"},
		},
		{
			name: "multiple Atmos flags",
			args: []string{"plan", "vpc", "--stack", "dev", "--dry-run", "--", "-var", "foo=bar"},
			expectedFlags: map[string]interface{}{
				"stack":   "dev",
				"dry-run": true,
			},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			expectedPositionalArgs:  []string{"plan", "vpc"},
		},
		{
			name:                    "no Atmos flags",
			args:                    []string{"plan", "vpc", "--", "-var", "foo=bar"},
			expectedFlags:           map[string]interface{}{},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			expectedPositionalArgs:  []string{"plan", "vpc"},
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
			expectedFlags: map[string]interface{}{
				"stack":    "prod",
				"identity": "admin",
				"dry-run":  true,
			},
			expectedSeparatedArgs: []string{
				"-var", "region=us-east-1",
				"-var-file=common.tfvars",
				"-out=plan.tfplan",
			},
			expectedPositionalArgs: []string{"plan", "vpc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := createTestParser()

			cfg, err := parser.Parse(context.Background(), tt.args)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedFlags, cfg.Flags)
			assert.Equal(t, tt.expectedSeparatedArgs, cfg.SeparatedArgs)
			assert.Equal(t, tt.expectedPositionalArgs, cfg.PositionalArgs)
		})
	}
}

func TestPassThroughFlagParser_Parse_EdgeCases(t *testing.T) {
	tests := []struct {
		name                    string
		args                    []string
		expectedFlags           map[string]interface{}
		expectedSeparatedArgs []string
		description             string
	}{
		{
			name:                    "duplicate flag (long and short form)",
			args:                    []string{"plan", "vpc", "-s", "dev", "--stack", "prod", "--", "-var", "foo=bar"},
			expectedFlags:           map[string]interface{}{"stack": "prod"},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			description:             "When both -s and --stack provided, last value wins",
		},
		{
			name:                    "flag value with equals sign",
			args:                    []string{"plan", "vpc", "--stack=prod/us-east-1", "--", "-var", "foo=bar"},
			expectedFlags:           map[string]interface{}{"stack": "prod/us-east-1"},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			description:             "Stack names with special characters should be preserved",
		},
		{
			name:                    "flag value with dashes",
			args:                    []string{"plan", "vpc", "--stack=my-stack-name", "--", "-var", "foo=bar"},
			expectedFlags:           map[string]interface{}{"stack": "my-stack-name"},
			expectedSeparatedArgs: []string{"-var", "foo=bar"},
			description:             "Stack names with dashes should be preserved",
		},
		{
			name:                    "empty pass-through args",
			args:                    []string{"plan", "vpc", "--stack", "dev", "--"},
			expectedFlags:           map[string]interface{}{"stack": "dev"},
			expectedSeparatedArgs: []string{},
			description:             "Empty pass-through args should work",
		},
		{
			name:                    "all flags before separator",
			args:                    []string{"--stack", "dev", "--dry-run", "--", "plan", "vpc"},
			expectedFlags:           map[string]interface{}{"stack": "dev", "dry-run": true},
			expectedSeparatedArgs: []string{},
			description:             "Atmos flags before separator, positional args after",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := createTestParser()

			cfg, err := parser.Parse(context.Background(), tt.args)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedFlags, cfg.Flags, tt.description)
			assert.Equal(t, tt.expectedSeparatedArgs, cfg.SeparatedArgs, tt.description)
		})
	}
}

func TestPassThroughFlagParser_RegisterFlags(t *testing.T) {
	parser := NewPassThroughFlagParser(WithTerraformFlags())
	cmd := &cobra.Command{Use: "terraform"}

	parser.RegisterFlags(cmd)

	// Verify flags were registered (only local flags, not inherited global flags)
	assert.NotNil(t, cmd.Flags().Lookup("stack"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("upload-status"))

	// Identity flag should NOT be in local flags - it's inherited from RootCmd
	assert.Nil(t, cmd.Flags().Lookup("identity"), "identity should be inherited from RootCmd, not local")
}

func TestPassThroughFlagParser_BindToViper(t *testing.T) {
	parser := createTestParser()
	v := viper.New()

	err := parser.BindToViper(v)

	require.NoError(t, err)
}

func TestPassThroughFlagParser_BindFlagsToViper(t *testing.T) {
	parser := createTestParser()
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

func TestPassThroughFlagParser_DisablePositionalExtraction(t *testing.T) {
	// Test cases for DisablePositionalExtraction feature used by auth exec/shell commands.
	tests := []struct {
		name                        string
		args                        []string
		disablePositionalExtraction bool
		expectedFlags               map[string]interface{}
		expectedSeparatedArgs     []string
		expectedPositionalArgs      []string
	}{
		{
			name:                        "with positional extraction enabled (default)",
			args:                        []string{"--identity=test-user", "--", "echo", "hello"},
			disablePositionalExtraction: false,
			expectedFlags:               map[string]interface{}{"identity": "test-user"},
			expectedSeparatedArgs:     []string{}, // "echo" and "hello" are extracted as positionals
			expectedPositionalArgs:      []string{"echo", "hello"},
		},
		{
			name:                        "with positional extraction disabled (auth commands)",
			args:                        []string{"--identity=test-user", "--", "echo", "hello"},
			disablePositionalExtraction: true,
			expectedFlags:               map[string]interface{}{"identity": "test-user"},
			expectedSeparatedArgs:     []string{"echo", "hello"}, // All args after -- are passed through
			expectedPositionalArgs:      []string{},                // Not extracted
		},
		{
			name:                        "auth exec with multiple command args",
			args:                        []string{"--identity=admin", "--", "aws", "s3", "ls", "s3://bucket"},
			disablePositionalExtraction: true,
			expectedFlags:               map[string]interface{}{"identity": "admin"},
			expectedSeparatedArgs:     []string{"aws", "s3", "ls", "s3://bucket"},
			expectedPositionalArgs:      []string{},
		},
		{
			name:                        "auth shell with shell args",
			args:                        []string{"--identity=test-user", "--", "-c", "echo $HOME"},
			disablePositionalExtraction: true,
			expectedFlags:               map[string]interface{}{"identity": "test-user"},
			expectedSeparatedArgs:     []string{"-c", "echo $HOME"},
			expectedPositionalArgs:      []string{},
		},
		{
			name:                        "no separator with disabled positional extraction",
			args:                        []string{"--identity=test-user", "echo", "hello"},
			disablePositionalExtraction: true,
			expectedFlags:               map[string]interface{}{"identity": "test-user"},
			expectedSeparatedArgs:     []string{"echo", "hello"}, // All non-flag args passed through
			expectedPositionalArgs:      []string{},
		},
		{
			name:                        "identity flag without value (NoOptDefVal)",
			args:                        []string{"--identity", "--", "echo", "test"},
			disablePositionalExtraction: true,
			expectedFlags:               map[string]interface{}{"identity": "__SELECT__"},
			expectedSeparatedArgs:     []string{"echo", "test"},
			expectedPositionalArgs:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewPassThroughFlagParser(
				WithStringFlag("identity", "i", "", "Identity flag"),
			)

			// Set NoOptDefVal for identity flag (like auth commands do).
			registry := parser.GetRegistry()
			if identityFlag := registry.Get("identity"); identityFlag != nil {
				if sf, ok := identityFlag.(*StringFlag); ok {
					sf.NoOptDefVal = "__SELECT__"
				}
			}

			if tt.disablePositionalExtraction {
				parser.DisablePositionalExtraction()
			}

			result, err := parser.Parse(context.Background(), tt.args)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedFlags, result.Flags, "AtmosFlags mismatch")
			assert.Equal(t, tt.expectedSeparatedArgs, result.SeparatedArgs, "SeparatedArgs mismatch")
			assert.Equal(t, tt.expectedPositionalArgs, result.PositionalArgs, "PositionalArgs mismatch")
		})
	}
}

// TestPassThroughFlagParser_EdgeCases tests edge cases that were previously tested
// in the deleted extractIdentityFlag function to ensure we maintain test coverage.
func TestPassThroughFlagParser_EdgeCases(t *testing.T) {
	tests := []struct {
		name                        string
		args                        []string
		expectedIdentity            interface{}
		expectedSeparatedArgs     []string
		expectedPositionalArgs      []string
		disablePositionalExtraction bool
	}{
		{
			name:                        "short flag -i with space-separated value and --",
			args:                        []string{"-i", "test-user", "--", "aws", "s3", "ls"},
			expectedIdentity:            "test-user",
			expectedSeparatedArgs:     []string{"aws", "s3", "ls"},
			expectedPositionalArgs:      []string{},
			disablePositionalExtraction: true,
		},
		{
			name:                        "short flag -i without value before --",
			args:                        []string{"-i", "--", "aws", "s3", "ls"},
			expectedIdentity:            "__SELECT__", // NoOptDefVal triggers
			expectedSeparatedArgs:     []string{"aws", "s3", "ls"},
			expectedPositionalArgs:      []string{},
			disablePositionalExtraction: true,
		},
		{
			name:                        "identity equals empty string",
			args:                        []string{"--identity=", "--", "echo", "hello"},
			expectedIdentity:            "__SELECT__", // Empty value triggers NoOptDefVal (interactive selection)
			expectedSeparatedArgs:     []string{"echo", "hello"},
			expectedPositionalArgs:      []string{},
			disablePositionalExtraction: true,
		},
		{
			name:                        "no double dash with identity equals value",
			args:                        []string{"--identity=test-user", "terraform", "plan"},
			expectedIdentity:            "test-user",
			expectedSeparatedArgs:     []string{"terraform", "plan"},
			expectedPositionalArgs:      []string{},
			disablePositionalExtraction: true,
		},
		{
			name:                        "identity flag at end without value",
			args:                        []string{"echo", "hello", "--identity"},
			expectedIdentity:            "__SELECT__",
			expectedSeparatedArgs:     []string{"echo", "hello"},
			expectedPositionalArgs:      []string{},
			disablePositionalExtraction: true,
		},
		{
			name:                        "only identity flag without command",
			args:                        []string{"--identity"},
			expectedIdentity:            "__SELECT__",
			expectedSeparatedArgs:     []string{}, // Parser returns empty slice, not nil
			expectedPositionalArgs:      []string{},
			disablePositionalExtraction: true,
		},
		{
			name:                        "short flag -i with equals syntax",
			args:                        []string{"-i=test-user", "--", "aws", "s3", "ls"},
			expectedIdentity:            "test-user",
			expectedSeparatedArgs:     []string{"aws", "s3", "ls"},
			expectedPositionalArgs:      []string{},
			disablePositionalExtraction: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewPassThroughFlagParser(
				WithStringFlag("identity", "i", "", "Identity flag"),
			)

			// Set NoOptDefVal for identity flag (like auth commands do).
			registry := parser.GetRegistry()
			if identityFlag := registry.Get("identity"); identityFlag != nil {
				if sf, ok := identityFlag.(*StringFlag); ok {
					sf.NoOptDefVal = "__SELECT__"
				}
			}

			if tt.disablePositionalExtraction {
				parser.DisablePositionalExtraction()
			}

			result, err := parser.Parse(context.Background(), tt.args)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedIdentity, result.Flags["identity"], "identity flag mismatch")
			assert.Equal(t, tt.expectedSeparatedArgs, result.SeparatedArgs, "SeparatedArgs mismatch")
			assert.Equal(t, tt.expectedPositionalArgs, result.PositionalArgs, "PositionalArgs mismatch")
		})
	}
}

// TestPassThroughFlagParser_ResolveNoOptDefValForEmptyValue tests the helper function
// that resolves NoOptDefVal for flags with empty values.
func TestPassThroughFlagParser_ResolveNoOptDefValForEmptyValue(t *testing.T) {
	tests := []struct {
		name           string
		flagName       string
		flagShorthand  string
		value          string
		noOptDefVal    string
		expectedResult interface{}
		description    string
	}{
		{
			name:           "empty value with NoOptDefVal",
			flagName:       "identity",
			flagShorthand:  "i",
			value:          "",
			noOptDefVal:    "__SELECT__",
			expectedResult: "__SELECT__",
			description:    "Empty value should return NoOptDefVal",
		},
		{
			name:           "empty value with NoOptDefVal (shorthand)",
			flagName:       "identity",
			flagShorthand:  "i",
			value:          "",
			noOptDefVal:    "__SELECT__",
			expectedResult: "__SELECT__",
			description:    "Shorthand with empty value should return NoOptDefVal",
		},
		{
			name:           "non-empty value with NoOptDefVal",
			flagName:       "identity",
			flagShorthand:  "i",
			value:          "prod",
			noOptDefVal:    "__SELECT__",
			expectedResult: "prod",
			description:    "Non-empty value should be returned as-is",
		},
		{
			name:           "empty value without NoOptDefVal",
			flagName:       "stack",
			flagShorthand:  "s",
			value:          "",
			noOptDefVal:    "",
			expectedResult: "",
			description:    "Empty value without NoOptDefVal should remain empty",
		},
		{
			name:           "pager flag empty value",
			flagName:       "pager",
			flagShorthand:  "",
			value:          "",
			noOptDefVal:    "true",
			expectedResult: "true",
			description:    "Pager flag with empty value should use NoOptDefVal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create parser with empty registry.
			parser := NewPassThroughFlagParser()

			// Register flag with NoOptDefVal.
			registry := parser.GetRegistry()
			flag := &StringFlag{
				Name:        tt.flagName,
				Shorthand:   tt.flagShorthand,
				NoOptDefVal: tt.noOptDefVal,
			}
			registry.Register(flag)

			// Manually populate shorthandToFull map since we registered flag after parser creation.
			// In normal usage, this is done automatically in NewPassThroughFlagParser constructor.
			if tt.flagShorthand != "" {
				parser.shorthandToFull[tt.flagShorthand] = tt.flagName
			}

			// Test with full name.
			result := parser.resolveNoOptDefValForEmptyValue(tt.flagName, tt.value)
			assert.Equal(t, tt.expectedResult, result, tt.description)

			// Test with shorthand if it exists.
			if tt.flagShorthand != "" {
				result = parser.resolveNoOptDefValForEmptyValue(tt.flagShorthand, tt.value)
				assert.Equal(t, tt.expectedResult, result, tt.description+" (shorthand)")
			}
		})
	}
}

// TestPassThroughFlagParser_EmptyValueHandling tests the parser's ability to handle
// --flag= (empty value after equals sign) for NoOptDefVal flags.
// This is an integration test that verifies the complete parsing flow.
func TestPassThroughFlagParser_EmptyValueHandling(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedValue string
		description   string
	}{
		{
			name:          "--identity= (empty value should use NoOptDefVal)",
			args:          []string{"plan", "vpc", "--identity="},
			expectedValue: "__SELECT__",
			description:   "Empty value after = should trigger interactive selection (use NoOptDefVal)",
		},
		{
			name:          "-i= (shorthand empty value should use NoOptDefVal)",
			args:          []string{"plan", "vpc", "-i="},
			expectedValue: "__SELECT__",
			description:   "Shorthand empty value should also trigger interactive selection",
		},
		{
			name:          "--identity (alone should use NoOptDefVal)",
			args:          []string{"plan", "vpc", "--identity"},
			expectedValue: "__SELECT__",
			description:   "Flag without value should use NoOptDefVal (existing behavior)",
		},
		{
			name:          "--identity=prod (explicit value)",
			args:          []string{"plan", "vpc", "--identity=prod"},
			expectedValue: "prod",
			description:   "Explicit value should be used as-is",
		},
		{
			name:          "no identity flag",
			args:          []string{"plan", "vpc"},
			expectedValue: "",
			description:   "No identity flag means no value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewPassThroughFlagParser(WithCommonFlags())

			// Set NoOptDefVal for identity flag
			registry := parser.GetRegistry()
			identityFlag := registry.Get("identity")
			require.NotNil(t, identityFlag, "identity flag should exist in registry")

			if sf, ok := identityFlag.(*StringFlag); ok {
				sf.NoOptDefVal = "__SELECT__"
			}

			// Parse args
			result, err := parser.Parse(context.Background(), tt.args)
			require.NoError(t, err)

			// Check identity value
			actualValue, exists := result.Flags["identity"]
			if tt.expectedValue == "" {
				// No identity flag provided
				assert.False(t, exists, "identity flag should not exist when not provided")
			} else {
				require.True(t, exists, "identity flag should exist in result")
				assert.Equal(t, tt.expectedValue, actualValue, tt.description)
			}
		})
	}
}
