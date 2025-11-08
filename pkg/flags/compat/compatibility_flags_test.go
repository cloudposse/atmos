package compat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// translationResult holds the result of compatibility flag translation.
type translationResult struct {
	atmosArgs     []string
	separatedArgs []string
}

// normalizeShorthandWithEquals is a test helper that normalizes shorthand flags with = syntax to longhand format.
// This is a copy of normalizeShorthandWithEquals to avoid import cycles.
func normalizeShorthandWithEquals(cmd *cobra.Command, arg string) (normalized string, wasNormalized bool) {
	// Only process single-dash flags with = syntax.
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return arg, false
	}

	// Check if arg has = syntax.
	idx := strings.Index(arg, "=")
	if idx <= 0 {
		return arg, false
	}

	// Extract shorthand (e.g., "-i=" → "i").
	shorthand := arg[1:idx]
	valuePart := arg[idx:] // "=value" or just "="

	// Look up the longhand flag name for this shorthand.
	var longhand string
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if flag.Shorthand == shorthand {
			longhand = flag.Name
		}
	})

	// If no longhand found, this is not a registered Cobra shorthand.
	// Return original arg unchanged.
	if longhand == "" {
		return arg, false
	}

	// Normalize to longhand format: -i=value → --identity=value.
	normalized = "--" + longhand + valuePart
	return normalized, true
}

// assertPanic is a test helper that asserts a panic occurred with an optional expected message.
func assertPanic(t *testing.T, expectedMsg string) {
	t.Helper()
	r := recover()
	if r == nil {
		t.Errorf("Expected panic but got none")
		return
	}
	if expectedMsg != "" {
		panicStr := fmt.Sprintf("%v", r)
		if !strings.Contains(panicStr, expectedMsg) {
			t.Errorf("Panic message %q does not contain expected %q", panicStr, expectedMsg)
		}
	}
}

// NOTE: Tests for -s and -i removed because these are Cobra native shorthands, NOT compatibility flags.
// Cobra handles -s → --stack automatically when you register flags with StringP("stack", "s", ...).
// The compatibility translator should ONLY handle terraform-specific pass-through flags.

func TestCompatibilityFlagTranslator_TerraformPassThroughCommonFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected translationResult
	}{
		{
			name:  "terraform -var → separated args (pass-through)",
			input: []string{"-var", "foo=bar"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var", "foo=bar"},
			},
		},
		{
			name:  "terraform -var=foo=bar → separated args (equals syntax)",
			input: []string{"-var=foo=bar"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var=foo=bar"},
			},
		},
		{
			name:  "multiple -var flags → separated args",
			input: []string{"-var", "foo=bar", "-var", "baz=qux"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var", "foo=bar", "-var", "baz=qux"},
			},
		},
		{
			name:  "terraform -out → separated args (Atmos may add this automatically)",
			input: []string{"-out", "plan.tfplan"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-out", "plan.tfplan"},
			},
		},
		{
			name:  "terraform -out=plan.tfplan → separated args (equals syntax)",
			input: []string{"-out=plan.tfplan"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-out=plan.tfplan"},
			},
		},
		{
			name:  "terraform -auto-approve → separated args",
			input: []string{"-auto-approve"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-auto-approve"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := buildTestCompatibilityTranslator()
			atmosArgs, separatedArgs := translator.Translate(tt.input)

			assert.Equal(t, tt.expected.atmosArgs, atmosArgs, "atmosArgs mismatch")
			assert.Equal(t, tt.expected.separatedArgs, separatedArgs, "separatedArgs mismatch")
		})
	}
}

func TestCompatibilityFlagTranslator_TerraformPassThroughFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected translationResult
	}{
		{
			name:  "terraform -var-file → separated args",
			input: []string{"-var-file", "prod.tfvars"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var-file", "prod.tfvars"},
			},
		},
		{
			name:  "terraform -var-file=prod.tfvars → separated args",
			input: []string{"-var-file=prod.tfvars"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var-file=prod.tfvars"},
			},
		},
		{
			name:  "terraform -target → separated args",
			input: []string{"-target", "aws_instance.app"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-target", "aws_instance.app"},
			},
		},
		{
			name:  "terraform -target=aws_instance.app → separated args",
			input: []string{"-target=aws_instance.app"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-target=aws_instance.app"},
			},
		},
		{
			name:  "terraform -replace → separated args",
			input: []string{"-replace=aws_instance.app"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-replace=aws_instance.app"},
			},
		},
		{
			name:  "terraform -destroy → separated args",
			input: []string{"-destroy"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-destroy"},
			},
		},
		{
			name:  "terraform -refresh-only → separated args",
			input: []string{"-refresh-only"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-refresh-only"},
			},
		},
		{
			name:  "terraform -lock=false → separated args",
			input: []string{"-lock=false"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-lock=false"},
			},
		},
		{
			name:  "terraform -lock-timeout → separated args",
			input: []string{"-lock-timeout", "30s"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-lock-timeout", "30s"},
			},
		},
		{
			name:  "terraform -parallelism → separated args",
			input: []string{"-parallelism", "10"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-parallelism", "10"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := buildTestCompatibilityTranslator()
			atmosArgs, separatedArgs := translator.Translate(tt.input)

			assert.Equal(t, tt.expected.atmosArgs, atmosArgs, "atmosArgs mismatch")
			assert.Equal(t, tt.expected.separatedArgs, separatedArgs, "separatedArgs mismatch")
		})
	}
}

func TestCompatibilityFlagTranslator_MixedScenarios(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected translationResult
	}{
		{
			name:  "cobra shorthand + terraform pass-through",
			input: []string{"-s", "dev", "-var", "foo=bar", "-var-file", "prod.tfvars"},
			expected: translationResult{
				// -s is Cobra shorthand, passed through as-is (Cobra will handle it).
				atmosArgs:     []string{"-s", "dev"},
				separatedArgs: []string{"-var", "foo=bar", "-var-file", "prod.tfvars"},
			},
		},
		{
			name:  "cobra shorthand + terraform pass-through with equals syntax",
			input: []string{"-s=dev", "-var=foo=bar", "-var-file=prod.tfvars"},
			expected: translationResult{
				// -s is Cobra shorthand, passed through as-is (Cobra will handle it).
				atmosArgs:     []string{"-s=dev"},
				separatedArgs: []string{"-var=foo=bar", "-var-file=prod.tfvars"},
			},
		},
		{
			name: "realistic terraform plan command",
			input: []string{
				"plan", "vpc",
				"-s", "dev",
				"-var", "region=us-east-1",
				"-var", "env=prod",
				"-var-file", "common.tfvars",
				"-target", "aws_instance.app",
			},
			expected: translationResult{
				atmosArgs: []string{
					"plan", "vpc",
					// -s is Cobra shorthand, passed through as-is (Cobra will handle it).
					"-s", "dev",
				},
				separatedArgs: []string{
					"-var", "region=us-east-1",
					"-var", "env=prod",
					"-var-file", "common.tfvars",
					"-target", "aws_instance.app",
				},
			},
		},
		{
			name:  "all pass-through flags",
			input: []string{"-var-file", "a.tfvars", "-target", "x", "-replace", "y"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var-file", "a.tfvars", "-target", "x", "-replace", "y"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := buildTestCompatibilityTranslator()
			atmosArgs, separatedArgs := translator.Translate(tt.input)

			assert.Equal(t, tt.expected.atmosArgs, atmosArgs, "atmosArgs mismatch")
			assert.Equal(t, tt.expected.separatedArgs, separatedArgs, "separatedArgs mismatch")
		})
	}
}

func TestCompatibilityFlagTranslator_ModernSyntax(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected translationResult
	}{
		{
			name:  "already modern --stack flag",
			input: []string{"--stack", "dev"},
			expected: translationResult{
				atmosArgs:     []string{"--stack", "dev"},
				separatedArgs: []string{},
			},
		},
		{
			name:  "already modern --var flag",
			input: []string{"--var", "foo=bar"},
			expected: translationResult{
				atmosArgs:     []string{"--var", "foo=bar"},
				separatedArgs: []string{},
			},
		},
		{
			name:  "modern syntax with equals",
			input: []string{"--stack=dev", "--var=foo=bar"},
			expected: translationResult{
				atmosArgs:     []string{"--stack=dev", "--var=foo=bar"},
				separatedArgs: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := buildTestCompatibilityTranslator()
			atmosArgs, separatedArgs := translator.Translate(tt.input)

			assert.Equal(t, tt.expected.atmosArgs, atmosArgs, "atmosArgs mismatch")
			assert.Equal(t, tt.expected.separatedArgs, separatedArgs, "separatedArgs mismatch")
		})
	}
}

func TestCompatibilityFlagTranslator_PositionalArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected translationResult
	}{
		{
			name:  "positional args not prefixed with dash",
			input: []string{"plan", "vpc"},
			expected: translationResult{
				atmosArgs:     []string{"plan", "vpc"},
				separatedArgs: []string{},
			},
		},
		{
			name:  "positional args with cobra shorthand",
			input: []string{"plan", "vpc", "-s", "dev"},
			expected: translationResult{
				// -s is Cobra shorthand, passed through as-is (Cobra will handle it).
				atmosArgs:     []string{"plan", "vpc", "-s", "dev"},
				separatedArgs: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := buildTestCompatibilityTranslator()
			atmosArgs, separatedArgs := translator.Translate(tt.input)

			assert.Equal(t, tt.expected.atmosArgs, atmosArgs, "atmosArgs mismatch")
			assert.Equal(t, tt.expected.separatedArgs, separatedArgs, "separatedArgs mismatch")
		})
	}
}

func TestCompatibilityFlagTranslator_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected translationResult
	}{
		{
			name:  "flag value that looks like flag",
			input: []string{"-var", "flag=-target"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var", "flag=-target"},
			},
		},
		{
			name:  "flag value with special characters",
			input: []string{"-var", "tags={\"env\":\"prod\",\"team\":\"devops\"}"},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{"-var", "tags={\"env\":\"prod\",\"team\":\"devops\"}"},
			},
		},
		{
			name:  "empty args",
			input: []string{},
			expected: translationResult{
				atmosArgs:     []string{},
				separatedArgs: []string{},
			},
		},
		{
			name:  "unknown single-dash flag (pass to Atmos for Cobra validation)",
			input: []string{"-x"},
			expected: translationResult{
				atmosArgs:     []string{"-x"}, // Let Cobra error on unknown flag
				separatedArgs: []string{},
			},
		},
		{
			name:  "unknown single-dash flag with value",
			input: []string{"-x", "value"},
			expected: translationResult{
				atmosArgs:     []string{"-x", "value"}, // Let Cobra handle/error
				separatedArgs: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := buildTestCompatibilityTranslator()
			atmosArgs, separatedArgs := translator.Translate(tt.input)

			assert.Equal(t, tt.expected.atmosArgs, atmosArgs, "atmosArgs mismatch")
			assert.Equal(t, tt.expected.separatedArgs, separatedArgs, "separatedArgs mismatch")
		})
	}
}

func TestCompatibilityFlagTranslator_ValidateTargets(t *testing.T) {
	tests := []struct {
		name        string
		aliases     map[string]CompatibilityFlag
		setupFlags  func(*cobra.Command)
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid targets - all exist",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-i": {Behavior: MapToAtmosFlag, Target: "--identity"},
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "stack name")
				cmd.Flags().String("identity", "", "identity name")
			},
			expectError: false,
		},
		{
			name: "invalid target - flag does not exist",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-x": {Behavior: MapToAtmosFlag, Target: "--nonexistent"},
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "stack name")
			},
			expectError: true,
			errorMsg:    `compatibility flag "-x" references non-existent flag "--nonexistent"`,
		},
		{
			name: "AppendToSeparated aliases - no validation needed",
			aliases: map[string]CompatibilityFlag{
				"-var":      {Behavior: AppendToSeparated, Target: ""},
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
			},
			setupFlags: func(cmd *cobra.Command) {
				// No flags registered - AppendToSeparated doesn't need validation.
			},
			expectError: false,
		},
		{
			name: "mixed - valid MapToAtmosFlag and AppendToSeparated",
			aliases: map[string]CompatibilityFlag{
				"-s":        {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-var":      {Behavior: AppendToSeparated, Target: ""},
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "stack name")
			},
			expectError: false,
		},
		{
			name: "empty target - no validation needed",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: ""},
			},
			setupFlags: func(cmd *cobra.Command) {
				// No flags registered - empty target doesn't need validation.
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			tt.setupFlags(cmd)

			translator := NewCompatibilityFlagTranslator(tt.aliases)
			err := translator.ValidateTargets(cmd)

			if tt.expectError {
				require.Error(t, err)
				// All validation errors should wrap ErrCompatibilityFlagMissingTarget
				assert.ErrorIs(t, err, errUtils.ErrCompatibilityFlagMissingTarget)
				// Also check the error message contains helpful context
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCompatibilityFlagTranslator_ValidateNoConflicts(t *testing.T) {
	tests := []struct {
		name        string
		aliases     map[string]CompatibilityFlag
		setupFlags  func(*cobra.Command)
		expectPanic bool
		panicMsg    string
	}{
		{
			name: "no conflicts - terraform pass-through flags only",
			aliases: map[string]CompatibilityFlag{
				"-var":      {Behavior: AppendToSeparated, Target: ""},
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().StringP("stack", "s", "", "stack name")
			},
			expectPanic: false,
		},
		{
			name: "conflict - compatibility flag -s conflicts with Cobra shorthand",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().StringP("stack", "s", "", "stack name")
			},
			expectPanic: true,
			panicMsg:    `compatibility flag "-s" conflicts with Cobra native shorthand for flag "stack"`,
		},
		{
			name: "conflict - compatibility flag -i conflicts with Cobra shorthand",
			aliases: map[string]CompatibilityFlag{
				"-i": {Behavior: MapToAtmosFlag, Target: "--identity"},
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().StringP("identity", "i", "", "identity name")
			},
			expectPanic: true,
			panicMsg:    `compatibility flag "-i" conflicts with Cobra native shorthand for flag "identity"`,
		},
		{
			name: "no conflict - different shorthand",
			aliases: map[string]CompatibilityFlag{
				"-x": {Behavior: MapToAtmosFlag, Target: "--xtra"},
			},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().StringP("stack", "s", "", "stack name")
			},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			tt.setupFlags(cmd)

			translator := NewCompatibilityFlagTranslator(tt.aliases)

			if tt.expectPanic {
				defer assertPanic(t, tt.panicMsg)
			}

			err := translator.ValidateNoConflicts(cmd)
			if !tt.expectPanic {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCompatibilityFlagTranslator_ValidateTargetsInArgs(t *testing.T) {
	tests := []struct {
		name        string
		aliases     map[string]CompatibilityFlag
		args        []string
		setupFlags  func(*cobra.Command)
		expectError bool
		errorMsg    string
	}{
		{
			name: "validates only used aliases - valid",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-i": {Behavior: MapToAtmosFlag, Target: "--identity"},
			},
			args: []string{"-s", "dev"}, // Only -s used, -i not validated.
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "stack name")
				// identity flag NOT registered, but that's OK since -i not used.
			},
			expectError: false,
		},
		{
			name: "validates only used aliases - invalid",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-i": {Behavior: MapToAtmosFlag, Target: "--identity"},
			},
			args: []string{"-i", "admin"}, // -i used but identity flag not registered.
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "stack name")
				// identity flag NOT registered.
			},
			expectError: true,
			errorMsg:    `compatibility flag "-i" references non-existent flag "--identity"`,
		},
		{
			name: "handles equals syntax",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
			},
			args: []string{"-s=dev"},
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "stack name")
			},
			expectError: false,
		},
		{
			name: "ignores AppendToSeparated aliases",
			aliases: map[string]CompatibilityFlag{
				"-var": {Behavior: AppendToSeparated, Target: ""},
			},
			args: []string{"-var", "foo=bar"},
			setupFlags: func(cmd *cobra.Command) {
				// No flags registered - AppendToSeparated doesn't need validation.
			},
			expectError: false,
		},
		{
			name: "ignores unknown aliases",
			aliases: map[string]CompatibilityFlag{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
			},
			args: []string{"-x", "value"}, // -x not in aliases map.
			setupFlags: func(cmd *cobra.Command) {
				cmd.Flags().String("stack", "", "stack name")
			},
			expectError: false, // Unknown alias ignored, Cobra will handle.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			tt.setupFlags(cmd)

			translator := NewCompatibilityFlagTranslator(tt.aliases)
			err := translator.ValidateTargetsInArgs(cmd, tt.args)

			if tt.expectError {
				require.Error(t, err)
				// All validation errors should wrap ErrCompatibilityFlagMissingTarget
				assert.ErrorIs(t, err, errUtils.ErrCompatibilityFlagMissingTarget)
				// Also check the error message contains helpful context
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// buildTestCompatibilityTranslator creates a translator with test configuration.
func buildTestCompatibilityTranslator() *CompatibilityFlagTranslator {
	// Use actual terraform compatibility flags to ensure tests stay in sync.
	// Import is: "github.com/cloudposse/atmos/pkg/flags/terraform"
	// NOTE: -s and -i are NOT compatibility flags - they are Cobra native shorthands.
	return NewCompatibilityFlagTranslator(map[string]CompatibilityFlag{
		// Pass-through terraform flags.
		"-var":          {Behavior: AppendToSeparated, Target: ""},
		"-var-file":     {Behavior: AppendToSeparated, Target: ""},
		"-out":          {Behavior: AppendToSeparated, Target: ""},
		"-auto-approve": {Behavior: AppendToSeparated, Target: ""},
		"-target":       {Behavior: AppendToSeparated, Target: ""},
		"-replace":      {Behavior: AppendToSeparated, Target: ""},
		"-destroy":      {Behavior: AppendToSeparated, Target: ""},
		"-refresh-only": {Behavior: AppendToSeparated, Target: ""},
		"-lock":         {Behavior: AppendToSeparated, Target: ""},
		"-lock-timeout": {Behavior: AppendToSeparated, Target: ""},
		"-parallelism":  {Behavior: AppendToSeparated, Target: ""},
	})
}

// TestCompatibilityFlagTranslator_ShorthandNormalization tests that shorthand flags
// with = syntax are normalized to longhand format BEFORE Cobra sees them.
// This ensures -i=value behaves the same as --identity=value.
func TestCompatibilityFlagTranslator_ShorthandNormalization(t *testing.T) {
	tests := []struct {
		name              string
		args              []string
		expectedAtmosArgs []string
		expectedSeparated []string
		description       string
	}{
		{
			name:              "-i=value normalizes to --identity=value",
			args:              []string{"plan", "vpc", "-i=prod"},
			expectedAtmosArgs: []string{"plan", "vpc", "--identity=prod"},
			expectedSeparated: []string{},
			description:       "shorthand with = and value normalizes to longhand",
		},
		{
			name:              "-i= normalizes to --identity=",
			args:              []string{"plan", "vpc", "-i="},
			expectedAtmosArgs: []string{"plan", "vpc", "--identity="},
			expectedSeparated: []string{},
			description:       "shorthand with = and empty value normalizes to longhand",
		},
		{
			name:              "-s=dev normalizes to --stack=dev",
			args:              []string{"plan", "vpc", "-s=dev"},
			expectedAtmosArgs: []string{"plan", "vpc", "--stack=dev"},
			expectedSeparated: []string{},
			description:       "stack shorthand with = normalizes to longhand",
		},
		{
			name:              "-s= normalizes to --stack=",
			args:              []string{"plan", "vpc", "-s="},
			expectedAtmosArgs: []string{"plan", "vpc", "--stack="},
			expectedSeparated: []string{},
			description:       "stack shorthand with = and empty value normalizes to longhand",
		},
		{
			name:              "mixed: -i=prod with -var (compatibility flag)",
			args:              []string{"plan", "vpc", "-i=prod", "-var", "region=us-east-1"},
			expectedAtmosArgs: []string{"plan", "vpc", "--identity=prod"},
			expectedSeparated: []string{"-var", "region=us-east-1"},
			description:       "shorthand normalization + compatibility flag translation",
		},
		{
			name:              "unknown shorthand with = passes through unchanged",
			args:              []string{"plan", "vpc", "-z=value"},
			expectedAtmosArgs: []string{"plan", "vpc", "-z=value"},
			expectedSeparated: []string{},
			description:       "unknown shorthand is not normalized (Cobra will error)",
		},
		{
			name:              "-i prod (space syntax) passes through unchanged",
			args:              []string{"plan", "vpc", "-i", "prod"},
			expectedAtmosArgs: []string{"plan", "vpc", "-i", "prod"},
			expectedSeparated: []string{},
			description:       "shorthand with space syntax does not need normalization (Cobra handles it)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create command with identity and stack shorthands.
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().StringP("identity", "i", "", "identity")
			cmd.Flags().StringP("stack", "s", "", "stack")

			// Create translator with compatibility flags.
			translator := NewCompatibilityFlagTranslator(map[string]CompatibilityFlag{
				"-var": {Behavior: AppendToSeparated, Target: ""},
			})

			// STEP 1: Normalize Cobra shorthands (this happens in AtmosFlagParser.Parse()).
			normalizedArgs := make([]string, len(tt.args))
			for i, arg := range tt.args {
				if normalized, wasNormalized := normalizeShorthandWithEquals(cmd, arg); wasNormalized {
					normalizedArgs[i] = normalized
				} else {
					normalizedArgs[i] = arg
				}
			}

			// STEP 2: Translate compatibility flags.
			atmosArgs, separatedArgs := translator.Translate(normalizedArgs)

			// Verify results.
			assert.Equal(t, tt.expectedAtmosArgs, atmosArgs, "AtmosArgs mismatch: %s", tt.description)
			assert.Equal(t, tt.expectedSeparated, separatedArgs, "SeparatedArgs mismatch: %s", tt.description)
		})
	}
}
