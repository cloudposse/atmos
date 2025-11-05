package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPassThroughFlagParser_InheritedPersistentFlags tests that PassThroughFlagParser
// correctly parses persistent flags from parent commands when DisableFlagParsing=true.
//
// Regression test for: "Packer commands failing with 'component is required' error"
// Root cause: PassThroughFlagParser.Parse() was parsing PersistentFlags() and Flags()
// separately, missing InheritedFlags() from parent commands (like RootCmd).
func TestPassThroughFlagParser_InheritedPersistentFlags(t *testing.T) {
	ctx := context.Background()
	v := viper.New()

	// Create parent command with persistent flags (simulates RootCmd)
	parentCmd := &cobra.Command{
		Use: "atmos",
	}
	parentCmd.PersistentFlags().String("logs-level", "", "Log level")
	parentCmd.PersistentFlags().String("config-path", "", "Config path")

	// Create child command with PassThroughFlagParser (simulates packer/terraform commands)
	parser := NewPassThroughFlagParser(
		WithStackFlag(),
		WithIdentityFlag(),
	)
	parser.SetPositionalArgsCount(1) // Extract 1 positional arg (component)

	childCmd := &cobra.Command{
		Use:  "packer",
		Args: cobra.ArbitraryArgs,
	}
	parser.RegisterPersistentFlags(childCmd)
	require.NoError(t, parser.BindToViper(v))

	// Add child to parent
	parentCmd.AddCommand(childCmd)

	// Simulate args with persistent flag from parent + local flag
	args := []string{"--logs-level=Debug", "--stack=prod", "component-name"}

	result, err := parser.Parse(ctx, args)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify persistent flag from parent was parsed (check Viper, not result.Flags)
	// PassThroughFlagParser binds to Viper during Parse()
	assert.Equal(t, "Debug", v.GetString("logs-level"), "persistent flag from parent should be parsed")

	// Verify local flag was parsed
	assert.Equal(t, "prod", result.Flags["stack"], "local flag should be parsed")

	// Verify positional arg was extracted
	assert.Equal(t, []string{"component-name"}, result.PositionalArgs)
}

// TestPassThroughFlagParser_MultipleInheritedFlags tests parsing multiple inherited flags
// from different levels of command hierarchy.
//
// Regression test for: "Global persistent flag --logs-level not working"
func TestPassThroughFlagParser_MultipleInheritedFlags(t *testing.T) {
	ctx := context.Background()
	v := viper.New()

	// Root command with global persistent flags
	rootCmd := &cobra.Command{
		Use: "atmos",
	}
	rootCmd.PersistentFlags().String("logs-level", "", "Log level")
	rootCmd.PersistentFlags().String("base-path", "", "Base path")

	// Middle command with its own persistent flags
	middleCmd := &cobra.Command{
		Use: "packer",
	}
	middleCmd.PersistentFlags().String("dry-run", "", "Dry run")
	rootCmd.AddCommand(middleCmd)

	// Leaf command with PassThroughFlagParser
	parser := NewPassThroughFlagParser(
		WithStackFlag(),
	)
	parser.SetPositionalArgsCount(1) // Extract 1 positional arg (component)

	leafCmd := &cobra.Command{
		Use:  "init",
		Args: cobra.ArbitraryArgs,
	}
	parser.RegisterFlags(leafCmd)
	require.NoError(t, parser.BindToViper(v))
	middleCmd.AddCommand(leafCmd)

	// Args with flags from all levels
	args := []string{
		"--logs-level=Trace", // From rootCmd
		"--base-path=/tmp",   // From rootCmd
		"--dry-run=true",     // From middleCmd
		"--stack=dev",        // From leafCmd
		"component",          // Positional arg
	}

	result, err := parser.Parse(ctx, args)
	require.NoError(t, err)

	// All inherited flags should be available in Viper
	assert.Equal(t, "Trace", v.GetString("logs-level"))
	assert.Equal(t, "/tmp", v.GetString("base-path"))
	assert.Equal(t, "true", v.GetString("dry-run"))
	assert.Equal(t, "dev", result.Flags["stack"])
	assert.Equal(t, []string{"component"}, result.PositionalArgs)
}

// TestPassThroughFlagParser_InheritedFlagPrecedence tests that CLI flags take precedence
// over environment variables and config file values.
//
// Regression test for: "Console duration flag precedence not working"
func TestPassThroughFlagParser_InheritedFlagPrecedence(t *testing.T) {
	ctx := context.Background()
	v := viper.New()

	// Set ENV var (lower precedence than CLI flag)
	v.Set("logs-level", "Info")

	parentCmd := &cobra.Command{Use: "atmos"}
	parentCmd.PersistentFlags().String("logs-level", "", "Log level")

	parser := NewPassThroughFlagParser(WithStackFlag())
	parser.SetPositionalArgsCount(1) // Extract 1 positional arg (component)
	childCmd := &cobra.Command{Use: "packer", Args: cobra.ArbitraryArgs}
	parser.RegisterPersistentFlags(childCmd)
	require.NoError(t, parser.BindToViper(v))
	parentCmd.AddCommand(childCmd)

	// CLI flag should override Viper value
	args := []string{"--logs-level=Debug", "--stack=prod", "component"}

	result, err := parser.Parse(ctx, args)
	require.NoError(t, err)

	// CLI flag should take precedence over env/config
	assert.Equal(t, "Debug", v.GetString("logs-level"), "CLI flag should override Viper value")
	assert.Equal(t, "prod", result.Flags["stack"])
}

// TestPassThroughFlagParser_ComponentExtraction tests that positional args
// (component names) are correctly extracted when persistent flags are present.
//
// Regression test for: "component is required error with packer commands"
func TestPassThroughFlagParser_ComponentExtraction(t *testing.T) {
	ctx := context.Background()
	v := viper.New()

	parentCmd := &cobra.Command{Use: "atmos"}
	parentCmd.PersistentFlags().String("logs-level", "", "Log level")

	parser := NewPassThroughFlagParser(WithStackFlag())
	parser.SetPositionalArgsCount(1) // Packer extracts 1 positional arg (component)

	childCmd := &cobra.Command{Use: "packer", Args: cobra.ArbitraryArgs}
	parser.RegisterPersistentFlags(childCmd)
	require.NoError(t, parser.BindToViper(v))
	parentCmd.AddCommand(childCmd)

	tests := []struct {
		name              string
		args              []string
		expectedComponent string
		expectedFlags     map[string]interface{}
	}{
		{
			name:              "component after persistent flag",
			args:              []string{"--logs-level=Debug", "aws-bastion", "--stack=prod"},
			expectedComponent: "aws-bastion",
			expectedFlags:     map[string]interface{}{"stack": "prod"},
		},
		{
			name:              "component before persistent flag",
			args:              []string{"aws-bastion", "--logs-level=Debug", "--stack=prod"},
			expectedComponent: "aws-bastion",
			expectedFlags:     map[string]interface{}{"stack": "prod"},
		},
		{
			name:              "component between flags",
			args:              []string{"--logs-level=Debug", "aws-bastion", "--stack=prod"},
			expectedComponent: "aws-bastion",
			expectedFlags:     map[string]interface{}{"stack": "prod"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test
			v = viper.New()
			require.NoError(t, parser.BindToViper(v))

			result, err := parser.Parse(ctx, tt.args)
			require.NoError(t, err)

			// Verify component was extracted
			require.Len(t, result.PositionalArgs, 1, "should extract exactly 1 positional arg (component)")
			assert.Equal(t, tt.expectedComponent, result.PositionalArgs[0])

			// Verify flags were parsed
			for key, expected := range tt.expectedFlags {
				assert.Equal(t, expected, result.Flags[key])
			}

			// Verify persistent flag was parsed
			assert.Equal(t, "Debug", v.GetString("logs-level"))
		})
	}
}

// TestPassThroughFlagParser_WithDoubleDashSeparator tests that the -- separator
// works correctly with inherited persistent flags.
func TestPassThroughFlagParser_WithDoubleDashSeparator(t *testing.T) {
	ctx := context.Background()
	v := viper.New()

	parentCmd := &cobra.Command{Use: "atmos"}
	parentCmd.PersistentFlags().String("logs-level", "", "Log level")

	parser := NewPassThroughFlagParser(WithStackFlag())
	parser.SetPositionalArgsCount(1) // Extract 1 positional arg (component)

	childCmd := &cobra.Command{Use: "packer", Args: cobra.ArbitraryArgs}
	parser.RegisterPersistentFlags(childCmd)
	require.NoError(t, parser.BindToViper(v))
	parentCmd.AddCommand(childCmd)

	// Args: persistent flag, component, atmos flag, --, pass-through flags
	args := []string{
		"--logs-level=Debug",
		"aws-bastion",
		"--stack=prod",
		"--",
		"-var", "foo=bar",
	}

	result, err := parser.Parse(ctx, args)
	require.NoError(t, err)

	// Verify component extracted
	assert.Equal(t, []string{"aws-bastion"}, result.PositionalArgs)

	// Verify Atmos flags parsed
	assert.Equal(t, "prod", result.Flags["stack"])
	assert.Equal(t, "Debug", v.GetString("logs-level"))

	// Verify pass-through args
	assert.Equal(t, []string{"-var", "foo=bar"}, result.PassThroughArgs)
}

// TestPassThroughFlagParser_NoInheritedFlags tests that the parser still works
// when there are no inherited flags (command has no parent).
func TestPassThroughFlagParser_NoInheritedFlags(t *testing.T) {
	ctx := context.Background()
	v := viper.New()

	parser := NewPassThroughFlagParser(WithStackFlag())
	parser.SetPositionalArgsCount(1)

	// Command with no parent (no inherited flags)
	cmd := &cobra.Command{Use: "standalone", Args: cobra.ArbitraryArgs}
	parser.RegisterPersistentFlags(cmd)
	require.NoError(t, parser.BindToViper(v))

	args := []string{"--stack=prod", "component"}

	result, err := parser.Parse(ctx, args)
	require.NoError(t, err)

	assert.Equal(t, "prod", result.Flags["stack"])
	assert.Equal(t, []string{"component"}, result.PositionalArgs)
}
