package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFlagParser_TerraformScenarios tests the unified parser with realistic Terraform command scenarios.
func TestFlagParser_TerraformScenarios(t *testing.T) {
	tests := []struct {
		name                string
		args                []string
		compatibilityAliases map[string]CompatibilityAlias
		expectedFlags        map[string]interface{}
		expectedPositional   []string
		expectedPassThrough  []string
		expectError          bool
		errorContains        string
	}{
		// ====================================================================
		// Category 1: Atmos Shorthand Flags (should be translated)
		// ====================================================================
		{
			name: "atmos shorthand stack flag",
			args: []string{"plan", "vpc", "-s", "dev"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
			},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "atmos shorthand identity flag",
			args: []string{"plan", "vpc", "-i", "prod"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-i": {Behavior: MapToAtmosFlag, Target: "--identity"},
			},
			expectedFlags: map[string]interface{}{
				"identity": "prod",
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},

		// ====================================================================
		// Category 2: Terraform Atmos-Managed Flags (Atmos processes these)
		// ====================================================================
		{
			name: "terraform var flag (atmos-managed)",
			args: []string{"plan", "vpc", "-var", "region=us-east-1"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-var": {Behavior: MapToAtmosFlag, Target: "--var"},
			},
			expectedFlags: map[string]interface{}{
				"var": []string{"region=us-east-1"},
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "multiple terraform var flags",
			args: []string{"plan", "vpc", "-var", "region=us-east-1", "-var", "env=prod"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-var": {Behavior: MapToAtmosFlag, Target: "--var"},
			},
			expectedFlags: map[string]interface{}{
				"var": []string{"region=us-east-1", "env=prod"},
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "terraform out flag (atmos-managed)",
			args: []string{"plan", "vpc", "-out", "tfplan"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-out": {Behavior: MapToAtmosFlag, Target: "--out"},
			},
			expectedFlags: map[string]interface{}{
				"out": "tfplan",
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "terraform auto-approve flag (atmos-managed)",
			args: []string{"apply", "vpc", "-auto-approve"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-auto-approve": {Behavior: MapToAtmosFlag, Target: "--auto-approve"},
			},
			expectedFlags: map[string]interface{}{
				"auto-approve": true,
			},
			expectedPositional:  []string{"apply", "vpc"},
			expectedPassThrough:  []string{},
		},

		// ====================================================================
		// Category 3: Terraform Pass-Through Flags (Atmos doesn't process)
		// ====================================================================
		{
			name: "terraform var-file flag (pass-through)",
			args: []string{"plan", "vpc", "-var-file", "common.tfvars"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags:       map[string]interface{}{},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough:  []string{"-var-file", "common.tfvars"},
		},
		{
			name: "terraform target flag (pass-through)",
			args: []string{"plan", "vpc", "-target", "aws_instance.app"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-target": {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags:       map[string]interface{}{},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough:  []string{"-target", "aws_instance.app"},
		},
		{
			name: "terraform replace flag (pass-through)",
			args: []string{"apply", "vpc", "-replace", "aws_instance.app"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-replace": {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags:      map[string]interface{}{},
			expectedPositional:  []string{"apply", "vpc"},
			expectedPassThrough:  []string{"-replace", "aws_instance.app"},
		},

		// ====================================================================
		// Category 4: Mixed Scenarios (Atmos + Terraform flags)
		// ====================================================================
		{
			name: "realistic terraform plan command",
			args: []string{
				"plan", "vpc",
				"-s", "dev",
				"-var", "region=us-east-1",
				"-var", "env=prod",
				"-var-file", "common.tfvars",
				"-target", "aws_instance.app",
			},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s":        {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-var":      {Behavior: MapToAtmosFlag, Target: "--var"},
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
				"-target":   {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
				"var":   []string{"region=us-east-1", "env=prod"},
			},
			expectedPositional: []string{"plan", "vpc"},
			expectedPassThrough:  []string{"-var-file", "common.tfvars", "-target", "aws_instance.app"},
		},
		{
			name: "terraform apply with auto-approve and pass-through flags",
			args: []string{
				"apply", "vpc",
				"-s", "prod",
				"-auto-approve",
				"-var-file", "prod.tfvars",
				"-target", "aws_instance.app",
				"-replace", "aws_instance.db",
			},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s":            {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-auto-approve": {Behavior: MapToAtmosFlag, Target: "--auto-approve"},
				"-var-file":     {Behavior: AppendToSeparated, Target: ""},
				"-target":       {Behavior: AppendToSeparated, Target: ""},
				"-replace":      {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags: map[string]interface{}{
				"stack":        "prod",
				"auto-approve": true,
			},
			expectedPositional:  []string{"apply", "vpc"},
			expectedPassThrough:  []string{"-var-file", "prod.tfvars", "-target", "aws_instance.app", "-replace", "aws_instance.db"},
		},

		// ====================================================================
		// Category 5: Modern Syntax (already double-dash)
		// ====================================================================
		{
			name: "modern double-dash syntax (no translation needed)",
			args: []string{"plan", "vpc", "--stack", "dev", "--var", "region=us-east-1"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s":   {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-var": {Behavior: MapToAtmosFlag, Target: "--var"},
			},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
				"var":   []string{"region=us-east-1"},
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "mixed modern and legacy syntax",
			args: []string{
				"plan", "vpc",
				"--stack", "dev",
				"-var", "region=us-east-1",
				"-var-file", "common.tfvars",
			},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-var":      {Behavior: MapToAtmosFlag, Target: "--var"},
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
				"var":   []string{"region=us-east-1"},
			},
			expectedPositional: []string{"plan", "vpc"},
			expectedPassThrough:  []string{"-var-file", "common.tfvars"},
		},

		// ====================================================================
		// Category 6: Double-Dash Separator
		// ====================================================================
		{
			name: "double-dash separator with legacy flags after",
			args: []string{
				"plan", "vpc",
				"-s", "dev",
				"--",
				"-var-file", "common.tfvars",
				"-target", "aws_instance.app",
			},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s":        {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
				"-target":   {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
			},
			expectedPositional: []string{"plan", "vpc"},
			expectedPassThrough:  []string{"-var-file", "common.tfvars", "-target", "aws_instance.app"},
		},
		{
			name: "double-dash separator with modern flags before",
			args: []string{
				"plan", "vpc",
				"--stack", "dev",
				"--var", "region=us-east-1",
				"--",
				"-var-file", "common.tfvars",
			},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-var-file": {Behavior: AppendToSeparated, Target: ""},
			},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
				"var":   []string{"region=us-east-1"},
			},
			expectedPositional: []string{"plan", "vpc"},
			expectedPassThrough:  []string{"-var-file", "common.tfvars"},
		},

		// ====================================================================
		// Category 7: Cobra Validation (unknown flags should error)
		// ====================================================================
		{
			name: "unknown flag triggers cobra error",
			args: []string{"plan", "vpc", "--unknown-flag", "value"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
			},
			expectError:   true,
			errorContains: "unknown flag",
		},
		{
			name: "unknown shorthand triggers cobra error",
			args: []string{"plan", "vpc", "-z", "value"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
			},
			expectError:   true,
			errorContains: "unknown shorthand flag",
		},

		// ====================================================================
		// Category 8: Edge Cases
		// ====================================================================
		{
			name: "flag value with equals sign",
			args: []string{"plan", "vpc", "-var", "key=value=with=equals"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-var": {Behavior: MapToAtmosFlag, Target: "--var"},
			},
			expectedFlags: map[string]interface{}{
				"var": []string{"key=value=with=equals"},
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "flag value with special characters",
			args: []string{"plan", "vpc", "-var", "key=value_with_underscores"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-var": {Behavior: MapToAtmosFlag, Target: "--var"},
			},
			expectedFlags: map[string]interface{}{
				"var": []string{"key=value_with_underscores"},
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "equals form for compatibility alias",
			args: []string{"plan", "vpc", "-s=dev", "-var=region=us-east-1"},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s":   {Behavior: MapToAtmosFlag, Target: "--stack"},
				"-var": {Behavior: MapToAtmosFlag, Target: "--var"},
			},
			expectedFlags: map[string]interface{}{
				"stack": "dev",
				"var":   []string{"region=us-east-1"},
			},
			expectedPositional:  []string{"plan", "vpc"},
			expectedPassThrough: []string{},
		},
		{
			name: "empty args array",
			args: []string{},
			compatibilityAliases: map[string]CompatibilityAlias{
				"-s": {Behavior: MapToAtmosFlag, Target: "--stack"},
			},
			expectedFlags:       map[string]interface{}{},
			expectedPositional:  []string{},
			expectedPassThrough: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh command for each test.
			cmd := &cobra.Command{
				Use: "test",
				RunE: func(cmd *cobra.Command, args []string) error {
					return nil
				},
			}

			// Register flags that the test expects.
			if _, ok := tt.expectedFlags["stack"]; ok {
				cmd.Flags().String("stack", "", "Stack name")
			}
			if _, ok := tt.expectedFlags["identity"]; ok {
				cmd.Flags().String("identity", "", "Identity selector")
			}
			if _, ok := tt.expectedFlags["var"]; ok {
				cmd.Flags().StringSlice("var", []string{}, "Set variables")
			}
			if _, ok := tt.expectedFlags["out"]; ok {
				cmd.Flags().String("out", "", "Output plan file")
			}
			if _, ok := tt.expectedFlags["auto-approve"]; ok {
				cmd.Flags().Bool("auto-approve", false, "Auto approve changes")
			}

			// Create viper instance.
			v := viper.New()

			// Create unified parser with compatibility aliases.
			translator := NewCompatibilityAliasTranslator(tt.compatibilityAliases)
			parser := NewAtmosFlagParser(cmd, v, translator)

			// Parse the args.
			result, err := parser.Parse(tt.args)

			// Check error expectations.
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			// No error expected.
			require.NoError(t, err)

			// Verify positional args.
			assert.Equal(t, tt.expectedPositional, result.PositionalArgs, "positional args mismatch")

			// Verify pass-through args.
			assert.Equal(t, tt.expectedPassThrough, result.SeparatedArgs, "pass-through args mismatch")

			// Verify flags.
			for key, expectedValue := range tt.expectedFlags {
				actualValue := v.Get(key)
				assert.Equal(t, expectedValue, actualValue, "flag %q mismatch", key)
			}
		})
	}
}

// TestFlagParser_NoOptDefVal tests flags with empty value handling for interactive selection.
// Note: Due to Cobra's design, --flag value syntax with NoOptDefVal treats "value" as positional arg.
// Therefore, users must use --flag=value or --flag= (empty) to trigger proper handling.
func TestFlagParser_NoOptDefVal(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		compatibilityAlias map[string]CompatibilityAlias
		expectedValue      string
		expectedPositional []string
		description        string
	}{
		{
			name: "identity flag with equals value",
			args: []string{"plan", "vpc", "--identity=prod"},
			compatibilityAlias: map[string]CompatibilityAlias{
				// NOTE: -i is NOT a compatibility alias - it's a Cobra native shorthand.
				// Don't add it here or ValidateNoConflicts will panic.
			},
			expectedValue:      "prod",
			expectedPositional: []string{"plan", "vpc"},
			description:        "Explicit value should be used",
		},
		{
			name: "identity flag with equals empty",
			args: []string{"plan", "vpc", "--identity="},
			compatibilityAlias: map[string]CompatibilityAlias{
				// NOTE: -i is NOT a compatibility alias - it's a Cobra native shorthand.
			},
			expectedValue:      "__SELECT__",
			expectedPositional: []string{"plan", "vpc"},
			description:        "Empty value should trigger interactive selection",
		},
		{
			name: "identity shorthand with equals value",
			args: []string{"plan", "vpc", "-i=prod"},
			compatibilityAlias: map[string]CompatibilityAlias{
				// NOTE: -i is NOT a compatibility alias - it's a Cobra native shorthand.
				// Cobra handles -i → --identity automatically.
			},
			expectedValue:      "prod",
			expectedPositional: []string{"plan", "vpc"},
			description:        "Cobra shorthand with equals value should work",
		},
		{
			name: "identity shorthand with equals empty",
			args: []string{"plan", "vpc", "-i="},
			compatibilityAlias: map[string]CompatibilityAlias{
				// NOTE: -i is NOT a compatibility alias - it's a Cobra native shorthand.
			},
			expectedValue:      "__SELECT__",
			expectedPositional: []string{"plan", "vpc"},
			description:        "Cobra shorthand -i= normalizes to --identity= (empty value triggers NoOptDefVal)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create command with identity flag.
			cmd := &cobra.Command{
				Use: "test",
				RunE: func(cmd *cobra.Command, args []string) error {
					return nil
				},
			}
			// Register flag with shorthand (NO NoOptDefVal - we handle empty values manually).
			cmd.Flags().StringP("identity", "i", "", "Identity selector")

			// Create viper instance.
			v := viper.New()

			// Create parser with compatibility aliases.
			translator := NewCompatibilityAliasTranslator(tt.compatibilityAlias)
			parser := NewAtmosFlagParser(cmd, v, translator)

			// Parse the args.
			result, err := parser.Parse(tt.args)

			// No error expected.
			require.NoError(t, err, "parse should succeed")

			// Verify the identity value.
			actualValue := v.GetString("identity")
			assert.Equal(t, tt.expectedValue, actualValue, tt.description)

			// Verify positional args.
			assert.Equal(t, tt.expectedPositional, result.PositionalArgs)
		})
	}
}
