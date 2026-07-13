package helmfile

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfHelmfileNotInstalled skips the test if helmfile is not installed.
func skipIfHelmfileNotInstalled(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("helmfile")
	if err != nil {
		t.Skip("helmfile not installed:", err)
	}
}

func TestHelmfileCommands_Error(t *testing.T) {
	skipIfHelmfileNotInstalled(t)
	stacksPath := "../../tests/fixtures/scenarios/stack-templates"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	testCases := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"apply", helmfileApplyCmd},
		{"destroy", helmfileDestroyCmd},
		{"diff", helmfileDiffCmd},
		{"sync", helmfileSyncCmd},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cmd.RunE(tc.cmd, []string{})
			assert.Error(t, err, "helmfile %s should error with no parameters", tc.name)
		})
	}
}

func TestHelmfileCommandProvider_GetCommand(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	cmd := provider.GetCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "helmfile", cmd.Use)
	assert.Contains(t, cmd.Aliases, "hf")
	assert.Equal(t, "Manage Helmfile-based Kubernetes deployments", cmd.Short)
}

func TestHelmfileCommandProvider_GetName(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	name := provider.GetName()

	assert.Equal(t, "helmfile", name)
}

func TestHelmfileCommandProvider_GetGroup(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	group := provider.GetGroup()

	assert.Equal(t, "Core Stack Commands", group)
}

func TestHelmfileCommandProvider_GetAliases(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	aliases := provider.GetAliases()

	assert.Nil(t, aliases)
}

func TestHelmfileCommandProvider_GetFlagsBuilder(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	builder := provider.GetFlagsBuilder()

	// Helmfile uses pass-through flag parsing, so no flags builder.
	assert.Nil(t, builder)
}

func TestHelmfileCommandProvider_GetPositionalArgsBuilder(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	builder := provider.GetPositionalArgsBuilder()

	// Helmfile command has subcommands, not positional args.
	assert.Nil(t, builder)
}

func TestHelmfileCommandProvider_GetCompatibilityFlags(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	flags := provider.GetCompatibilityFlags()

	// Helmfile uses pass-through flag parsing.
	assert.Nil(t, flags)
}

func TestHelmfileCommandProvider_IsExperimental(t *testing.T) {
	provider := &HelmfileCommandProvider{}
	isExperimental := provider.IsExperimental()

	assert.False(t, isExperimental)
}

func TestHelmfileCmd_HasSubcommands(t *testing.T) {
	cmd := helmfileCmd

	// Verify helmfile command has expected subcommands.
	subcommands := cmd.Commands()
	subcommandNames := make([]string, len(subcommands))
	for i, sub := range subcommands {
		subcommandNames[i] = sub.Name()
	}

	assert.Contains(t, subcommandNames, "apply")
	assert.Contains(t, subcommandNames, "destroy")
	assert.Contains(t, subcommandNames, "diff")
	assert.Contains(t, subcommandNames, "sync")
	assert.Contains(t, subcommandNames, "version")
	assert.Contains(t, subcommandNames, "generate")
	assert.Contains(t, subcommandNames, "source")
}

func TestHelmfileCmd_FParseErrWhitelist(t *testing.T) {
	// Verify unknown flags are whitelisted for pass-through to helmfile.
	assert.True(t, helmfileCmd.FParseErrWhitelist.UnknownFlags)
}

func TestHelmfileRun_HelpRequest(t *testing.T) {
	// Test that help request returns nil (handled by handleHelpRequest).
	err := helmfileRun(helmfileCmd, "sync", []string{"--help"})
	assert.NoError(t, err)

	err = helmfileRun(helmfileCmd, "diff", []string{"-h"})
	assert.NoError(t, err)

	err = helmfileRun(helmfileCmd, "apply", []string{"help"})
	assert.NoError(t, err)
}

// newHelmfileRunTestCmd builds a minimal command carrying the global flags
// getConfigAndStacksInfo/ProcessCommandLineArgs expect (base-path, config,
// stack, ...) — mirroring newHookTestCmd in cmd/terraform's hook tests. These
// flags are normally inherited from RootCmd; helmfileRun is exercised here
// without going through RootCmd.Execute(), so they must be registered
// directly.
func newHelmfileRunTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "diff"}
	cmd.Flags().String("base-path", "", "base path")
	cmd.Flags().StringSlice("config", nil, "config")
	cmd.Flags().StringSlice("config-path", nil, "config path")
	cmd.Flags().StringSlice("profile", nil, "profile")
	cmd.Flags().StringP("stack", "s", "", "stack flag")
	cmd.Flags().Bool("ci", false, "ci flag")
	return cmd
}

// TestHelmfileRun_NodeHooksFallbackOnEarlyFailure is a regression test for
// the helmfileRun fallback (err != nil && !nodeHooks.called): when
// ExecuteHelmfile fails before ever reaching info.NodeHooks.Before/After
// (e.g., a component/stack whose config resolves but whose Helm auth profile
// doesn't exist), helmfileRun must still fire the after-hook itself so CI/user
// hooks see the failure. This also exercises helmfileNodeHooks.After's
// execErr != nil branch through the real call path (not just the direct-call
// unit test), since the underlying error is non-nil here.
func TestHelmfileRun_NodeHooksFallbackOnEarlyFailure(t *testing.T) {
	skipIfHelmfileNotInstalled(t)
	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "complete"))

	err := helmfileRun(newHelmfileRunTestCmd(), "diff", []string{"echo-server", "-s", "tenant1-ue2-dev"})
	require.Error(t, err, "diff should fail: the fixture's helm auth profile does not exist locally")
}
