package terraform

import (
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

// Note: These tests use viper.New() to create fresh, isolated viper instances
// rather than cmd.TestKit because:
// 1. TestKit is in cmd package (not exported to cmd/terraform)
// 2. TestKit is designed for RootCmd state cleanup
// 3. These tests only read global parsers (don't modify them)
// 4. Fresh viper instances ensure complete test isolation

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

// TestCompoundSubcommandRegistration verifies that compound sub-subcommands are registered
// as proper Cobra child commands of their parent commands (state, providers, workspace).
func TestCompoundSubcommandRegistration(t *testing.T) {
	tests := []struct {
		name             string
		parentCmd        *cobra.Command
		expectedChildren []string
		parentUse        string
		parentAttachedTo *cobra.Command
	}{
		{
			name:      "state sub-subcommands",
			parentCmd: stateCmd,
			expectedChildren: []string{
				"list", "mv", "pull", "push", "replace-provider", "rm", "show",
			},
			parentUse:        "state",
			parentAttachedTo: terraformCmd,
		},
		{
			name:      "providers sub-subcommands",
			parentCmd: providersCmd,
			expectedChildren: []string{
				"lock", "mirror", "schema",
			},
			parentUse:        "providers",
			parentAttachedTo: terraformCmd,
		},
		{
			name:      "workspace sub-subcommands",
			parentCmd: workspaceCmd,
			expectedChildren: []string{
				"list", "select", "new", "delete", "show",
			},
			parentUse:        "workspace",
			parentAttachedTo: terraformCmd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify parent command exists and has correct Use field.
			require.NotNil(t, tt.parentCmd, "parent command should not be nil")
			assert.Equal(t, tt.parentUse, tt.parentCmd.Use)

			// Verify parent is attached to terraformCmd.
			found := false
			for _, cmd := range tt.parentAttachedTo.Commands() {
				if cmd == tt.parentCmd {
					found = true
					break
				}
			}
			assert.True(t, found, "%s should be registered as a child of terraformCmd", tt.parentUse)

			// Collect actual child command names.
			var actualChildren []string
			for _, cmd := range tt.parentCmd.Commands() {
				actualChildren = append(actualChildren, cmd.Name())
			}
			sort.Strings(actualChildren)

			expectedSorted := make([]string, len(tt.expectedChildren))
			copy(expectedSorted, tt.expectedChildren)
			sort.Strings(expectedSorted)

			assert.Equal(t, expectedSorted, actualChildren,
				"%s should have the expected sub-subcommands registered", tt.parentUse)

			// Verify each child has correct properties.
			for _, cmd := range tt.parentCmd.Commands() {
				assert.NotEmpty(t, cmd.Short, "child %s should have a Short description", cmd.Name())
				assert.NotNil(t, cmd.RunE, "child %s should have a RunE function", cmd.Name())
				assert.True(t, cmd.FParseErrWhitelist.UnknownFlags,
					"child %s should whitelist unknown flags", cmd.Name())
				assert.Contains(t, cmd.Use, "[component] -s [stack]",
					"child %s Use should include component/stack hint", cmd.Name())
			}
		})
	}
}

// TestCompoundSubcommandHasRunE verifies that parent compound commands have RunE set.
func TestCompoundSubcommandHasRunE(t *testing.T) {
	cmds := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"state", stateCmd},
		{"providers", providersCmd},
		{"workspace", workspaceCmd},
	}

	for _, tc := range cmds {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.cmd.RunE, "%s command should have RunE", tc.name)
			assert.True(t, tc.cmd.FParseErrWhitelist.UnknownFlags,
				"%s command should whitelist unknown flags", tc.name)
		})
	}
}

// TestWorkspaceSubcommandsPersistentFlags verifies that workspace sub-subcommands
// inherit persistent flags from the workspace parent command.
func TestWorkspaceSubcommandsPersistentFlags(t *testing.T) {
	// Workspace should have persistent flags (backend execution flags).
	backendFlags := []string{
		"auto-generate-backend-file",
		"init-run-reconfigure",
	}

	for _, flagName := range backendFlags {
		flag := workspaceCmd.PersistentFlags().Lookup(flagName)
		assert.NotNil(t, flag, "workspace should have persistent flag %s", flagName)
	}

	// Verify that sub-subcommands can see inherited persistent flags.
	for _, cmd := range workspaceCmd.Commands() {
		for _, flagName := range backendFlags {
			flag := cmd.InheritedFlags().Lookup(flagName)
			assert.NotNil(t, flag, "workspace %s should inherit persistent flag %s",
				cmd.Name(), flagName)
		}
	}
}

// TestNewTerraformPassthroughSubcommand tests the helper function that creates
// Cobra child commands for compound subcommands.
func TestNewTerraformPassthroughSubcommand(t *testing.T) {
	parent := &cobra.Command{Use: "testparent"}
	cmd := newTerraformPassthroughSubcommand(parent, "testchild", "Test child command")

	assert.Equal(t, "testchild [component] -s [stack]", cmd.Use)
	assert.Equal(t, "Test child command", cmd.Short)
	assert.NotNil(t, cmd.RunE, "passthrough subcommand should have RunE")
	assert.True(t, cmd.FParseErrWhitelist.UnknownFlags,
		"passthrough subcommand should whitelist unknown flags")
}

// TestNewWorkspacePassthroughSubcommand tests the workspace-specific helper function
// that creates Cobra child commands with workspace parser binding.
func TestNewWorkspacePassthroughSubcommand(t *testing.T) {
	cmd := newWorkspacePassthroughSubcommand("testchild", "Test workspace child")

	assert.Equal(t, "testchild [component] -s [stack]", cmd.Use)
	assert.Equal(t, "Test workspace child", cmd.Short)
	assert.NotNil(t, cmd.RunE, "workspace passthrough subcommand should have RunE")
	assert.True(t, cmd.FParseErrWhitelist.UnknownFlags,
		"workspace passthrough subcommand should whitelist unknown flags")
}

// TestCompoundSubcommandCompatFlags verifies that per-subcommand compat flags are
// registered in the command registry and contain the expected flags.
func TestCompoundSubcommandCompatFlags(t *testing.T) {
	tests := []struct {
		name          string
		registryKey   string
		compatFunc    func() map[string]compat.CompatibilityFlag
		expectedFlags []string
		emptyExpected bool
	}{
		// State sub-subcommands.
		{
			name:          "state-list",
			registryKey:   "state-list",
			compatFunc:    StateListCompatFlags,
			expectedFlags: []string{"-state", "-id"},
		},
		{
			name:          "state-mv",
			registryKey:   "state-mv",
			compatFunc:    StateMvCompatFlags,
			expectedFlags: []string{"-lock", "-lock-timeout", "-ignore-remote-version"},
		},
		{
			name:          "state-pull",
			registryKey:   "state-pull",
			compatFunc:    StatePullCompatFlags,
			emptyExpected: true,
		},
		{
			name:          "state-push",
			registryKey:   "state-push",
			compatFunc:    StatePushCompatFlags,
			expectedFlags: []string{"-force", "-lock", "-lock-timeout", "-ignore-remote-version"},
		},
		{
			name:          "state-replace-provider",
			registryKey:   "state-replace-provider",
			compatFunc:    StateReplaceProviderCompatFlags,
			expectedFlags: []string{"-auto-approve", "-lock", "-lock-timeout", "-ignore-remote-version"},
		},
		{
			name:          "state-rm",
			registryKey:   "state-rm",
			compatFunc:    StateRmCompatFlags,
			expectedFlags: []string{"-lock", "-lock-timeout", "-ignore-remote-version"},
		},
		{
			name:          "state-show",
			registryKey:   "state-show",
			compatFunc:    StateShowCompatFlags,
			expectedFlags: []string{"-state"},
		},
		// Providers sub-subcommands.
		{
			name:          "providers-lock",
			registryKey:   "providers-lock",
			compatFunc:    ProvidersLockCompatFlags,
			expectedFlags: []string{"-platform", "-fs-mirror", "-net-mirror", "-enable-plugin-cache"},
		},
		{
			name:          "providers-mirror",
			registryKey:   "providers-mirror",
			compatFunc:    ProvidersMirrorCompatFlags,
			expectedFlags: []string{"-platform"},
		},
		{
			name:          "providers-schema",
			registryKey:   "providers-schema",
			compatFunc:    ProvidersSchemaCompatFlags,
			expectedFlags: []string{"-json"},
		},
		// Workspace sub-subcommands.
		{
			name:          "workspace-list",
			registryKey:   "workspace-list",
			compatFunc:    WorkspaceListCompatFlags,
			emptyExpected: true,
		},
		{
			name:          "workspace-select",
			registryKey:   "workspace-select",
			compatFunc:    WorkspaceSelectCompatFlags,
			expectedFlags: []string{"-or-create"},
		},
		{
			name:          "workspace-new",
			registryKey:   "workspace-new",
			compatFunc:    WorkspaceNewCompatFlags,
			expectedFlags: []string{"-lock", "-lock-timeout", "-state"},
		},
		{
			name:          "workspace-delete",
			registryKey:   "workspace-delete",
			compatFunc:    WorkspaceDeleteCompatFlags,
			expectedFlags: []string{"-force", "-lock", "-lock-timeout"},
		},
		{
			name:          "workspace-show",
			registryKey:   "workspace-show",
			compatFunc:    WorkspaceShowCompatFlags,
			emptyExpected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify the function returns expected flags.
			flags := tt.compatFunc()
			require.NotNil(t, flags, "%s compat flags should not be nil", tt.name)

			if tt.emptyExpected {
				assert.Empty(t, flags, "%s should have no compat flags", tt.name)
			} else {
				for _, expectedFlag := range tt.expectedFlags {
					_, ok := flags[expectedFlag]
					assert.True(t, ok, "%s should contain flag %s", tt.name, expectedFlag)
				}
				assert.Len(t, flags, len(tt.expectedFlags),
					"%s should have exactly %d flags", tt.name, len(tt.expectedFlags))
			}

			// Verify all flags have AppendToSeparated behavior.
			for flagName, flag := range flags {
				assert.Equal(t, compat.AppendToSeparated, flag.Behavior,
					"%s flag %s should have AppendToSeparated behavior", tt.name, flagName)
				assert.NotEmpty(t, flag.Description,
					"%s flag %s should have a description", tt.name, flagName)
			}

			// Verify the flags are registered in the command registry.
			registeredFlags := internal.GetSubcommandCompatFlags("terraform", tt.registryKey)
			require.NotNil(t, registeredFlags,
				"%s should be registered in the command registry", tt.registryKey)
			assert.Equal(t, len(flags), len(registeredFlags),
				"%s registry flags should match function output", tt.registryKey)
		})
	}
}

// TestCompoundSubcommandCompatFlags_NoDryRunConflict verifies that terraform's -dry-run
// is excluded from state mv and state rm compat flags to avoid conflict with Atmos --dry-run.
func TestCompoundSubcommandCompatFlags_NoDryRunConflict(t *testing.T) {
	conflictingSubcommands := []struct {
		name       string
		compatFunc func() map[string]compat.CompatibilityFlag
	}{
		{"state-mv", StateMvCompatFlags},
		{"state-rm", StateRmCompatFlags},
	}

	for _, tt := range conflictingSubcommands {
		t.Run(tt.name, func(t *testing.T) {
			flags := tt.compatFunc()
			_, hasDryRun := flags["-dry-run"]
			assert.False(t, hasDryRun,
				"%s should NOT include -dry-run to avoid conflict with Atmos --dry-run", tt.name)
		})
	}
}
