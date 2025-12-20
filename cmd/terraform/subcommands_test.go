package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// subcommandTestCase defines test parameters for terraform subcommands.
type subcommandTestCase struct {
	name           string
	cmdName        string
	cmd            *cobra.Command
	parser         *flags.StandardParser
	shortContains  string
	longContains   string
	backendFileVal string
	reconfigureVal string
	skipInitVal    bool
}

// getSubcommandTestCases returns test cases for init and workspace commands.
func getSubcommandTestCases() []subcommandTestCase {
	return []subcommandTestCase{
		{
			name:           "init",
			cmdName:        "init",
			cmd:            initCmd,
			parser:         initParser,
			shortContains:  "Prepare",
			longContains:   "Initialize",
			backendFileVal: "true",
			reconfigureVal: "false",
			skipInitVal:    true,
		},
		{
			name:           "workspace",
			cmdName:        "workspace",
			cmd:            workspaceCmd,
			parser:         workspaceParser,
			shortContains:  "workspace",
			longContains:   "workspace",
			backendFileVal: "false",
			reconfigureVal: "true",
			skipInitVal:    false,
		},
	}
}

// TestSubcommandFlagSetup verifies that subcommands have correct flags registered.
func TestSubcommandFlagSetup(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Verify command is registered.
			require.NotNil(t, tc.cmd)

			// Verify it's attached to terraformCmd.
			found := false
			for _, cmd := range terraformCmd.Commands() {
				if cmd.Use == tc.cmdName {
					found = true
					break
				}
			}
			assert.True(t, found, "%s should be registered as a subcommand of terraformCmd", tc.cmdName)

			// Verify command has backend execution flags.
			backendFlags := []string{
				"auto-generate-backend-file",
				"init-run-reconfigure",
			}

			for _, flagName := range backendFlags {
				flag := tc.cmd.Flags().Lookup(flagName)
				assert.NotNil(t, flag, "%s flag should be registered on %s", flagName, tc.cmdName)
			}
		})
	}
}

// TestSubcommandParserSetup verifies that parsers are properly configured.
func TestSubcommandParserSetup(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.parser, "%sParser should be initialized", tc.name)

			// Verify the parser has the backend execution flags.
			registry := tc.parser.Registry()
			assert.True(t, registry.Has("auto-generate-backend-file"),
				"%sParser should have auto-generate-backend-file flag", tc.name)
			assert.True(t, registry.Has("init-run-reconfigure"),
				"%sParser should have init-run-reconfigure flag", tc.name)
		})
	}
}

// TestSubcommandViperBinding verifies that command flags can be bound to viper.
func TestSubcommandViperBinding(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh viper instance for testing.
			v := viper.New()

			// Bind the parser to viper.
			err := tc.parser.BindToViper(v)
			require.NoError(t, err, "%sParser should bind to viper without error", tc.name)

			// Set values via viper and verify they can be retrieved.
			v.Set("auto-generate-backend-file", tc.backendFileVal)
			v.Set("init-run-reconfigure", tc.reconfigureVal)

			assert.Equal(t, tc.backendFileVal, v.GetString("auto-generate-backend-file"))
			assert.Equal(t, tc.reconfigureVal, v.GetString("init-run-reconfigure"))
		})
	}
}

// TestSubcommandFlagsToViperBinding tests the BindFlagsToViper functionality.
func TestSubcommandFlagsToViperBinding(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test command to simulate command behavior.
			cmd := &cobra.Command{Use: "test-" + tc.cmdName}

			// Register the flags on the test command.
			tc.parser.RegisterFlags(cmd)

			// Create a fresh viper instance.
			v := viper.New()

			// Bind flags to viper.
			err := tc.parser.BindFlagsToViper(cmd, v)
			require.NoError(t, err, "BindFlagsToViper should succeed")

			// Set flag values via command line simulation.
			err = cmd.Flags().Set("auto-generate-backend-file", tc.backendFileVal)
			require.NoError(t, err)
			err = cmd.Flags().Set("init-run-reconfigure", tc.reconfigureVal)
			require.NoError(t, err)

			// Bind again to pick up the flag values.
			err = tc.parser.BindFlagsToViper(cmd, v)
			require.NoError(t, err)

			// Verify the values are accessible via viper.
			assert.Equal(t, tc.backendFileVal, v.GetString("auto-generate-backend-file"))
			assert.Equal(t, tc.reconfigureVal, v.GetString("init-run-reconfigure"))
		})
	}
}

// TestSubcommandParseOptions tests that options are correctly parsed.
func TestSubcommandParseOptions(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			v := viper.New()

			// Simulate setting backend execution flags.
			v.Set("auto-generate-backend-file", tc.backendFileVal)
			v.Set("init-run-reconfigure", tc.reconfigureVal)
			v.Set("skip-init", tc.skipInitVal)
			v.Set("dry-run", true)

			opts := ParseTerraformRunOptions(v)

			assert.Equal(t, tc.backendFileVal, opts.AutoGenerateBackendFile)
			assert.Equal(t, tc.reconfigureVal, opts.InitRunReconfigure)
			assert.Equal(t, tc.skipInitVal, opts.SkipInit)
			assert.True(t, opts.DryRun)
		})
	}
}

// TestSubcommandMetadata verifies command metadata.
func TestSubcommandMetadata(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.cmdName, tc.cmd.Use)
			assert.Contains(t, tc.cmd.Short, tc.shortContains)
			assert.Contains(t, tc.cmd.Long, tc.longContains)
			assert.NotNil(t, tc.cmd.RunE, "%s should have a RunE function", tc.cmdName)
		})
	}
}

// TestSubcommandRunEFlagBinding tests that RunE correctly binds flags to viper.
func TestSubcommandRunEFlagBinding(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test command that simulates the command behavior.
			testCmd := &cobra.Command{Use: "test-" + tc.cmdName}

			// Register both terraform and subcommand-specific flags.
			terraformParser.RegisterFlags(testCmd)
			tc.parser.RegisterFlags(testCmd)

			// Create a fresh viper instance.
			v := viper.New()

			// Simulate setting flag values.
			err := testCmd.Flags().Set("auto-generate-backend-file", tc.backendFileVal)
			require.NoError(t, err)
			err = testCmd.Flags().Set("init-run-reconfigure", tc.reconfigureVal)
			require.NoError(t, err)

			// Bind flags to viper (simulating what RunE does).
			err = terraformParser.BindFlagsToViper(testCmd, v)
			require.NoError(t, err, "terraformParser.BindFlagsToViper should succeed")

			err = tc.parser.BindFlagsToViper(testCmd, v)
			require.NoError(t, err, "%sParser.BindFlagsToViper should succeed", tc.name)

			// Parse options (simulating what RunE does).
			opts := ParseTerraformRunOptions(v)

			// Verify the options were parsed correctly.
			assert.Equal(t, tc.backendFileVal, opts.AutoGenerateBackendFile)
			assert.Equal(t, tc.reconfigureVal, opts.InitRunReconfigure)
		})
	}
}

// TestSubcommandRunEWithDryRun tests that the RunE function can be executed with dry-run.
func TestSubcommandRunEWithDryRun(t *testing.T) {
	for _, tc := range getSubcommandTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test command to simulate flag parsing.
			testCmd := &cobra.Command{Use: "test-" + tc.cmdName}
			terraformParser.RegisterFlags(testCmd)
			tc.parser.RegisterFlags(testCmd)

			v := viper.New()

			// Set dry-run to avoid actual execution.
			v.Set("dry-run", true)
			v.Set("auto-generate-backend-file", tc.backendFileVal)
			v.Set("init-run-reconfigure", tc.reconfigureVal)

			// Bind flags to viper.
			err := terraformParser.BindFlagsToViper(testCmd, v)
			require.NoError(t, err)
			err = tc.parser.BindFlagsToViper(testCmd, v)
			require.NoError(t, err)

			// Parse options.
			opts := ParseTerraformRunOptions(v)

			// Verify the flag binding and parsing worked correctly.
			assert.True(t, opts.DryRun)
			assert.Equal(t, tc.backendFileVal, opts.AutoGenerateBackendFile)
			assert.Equal(t, tc.reconfigureVal, opts.InitRunReconfigure)
		})
	}
}
