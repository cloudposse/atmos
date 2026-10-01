package skill

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
)

// These tests exercise resolveSkillClients directly, bypassing the
// interactive huh form (skipPrompt is always true here), mirroring
// cmd/mcp/client/install_test.go's TestResolveInstallClients_* suite.

func TestResolveSkillClients_AutoNoFallbackWhenNothingDetected(t *testing.T) {
	base := t.TempDir()
	v := viper.New()

	clients, err := resolveSkillClients(base, v, true, marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Empty(t, clients, "auto mode must not fall back to installing into every supported client")
}

func TestResolveSkillClients_AutoUsesExactlyWhatWasDetected(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
	v := viper.New()

	clients, err := resolveSkillClients(base, v, true, marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Equal(t, []string{marketplace.ClientVSCode}, clients)
}

func TestResolveSkillClients_ExplicitClientBypassesDetection(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("client", []string{"vscode"})

	clients, err := resolveSkillClients(base, v, true, marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Equal(t, []string{"vscode"}, clients)
}

func TestResolveSkillClients_ExplicitClientWinsEvenInteractively(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("client", []string{"claude-code"})

	// skipPrompt is false here, but the explicit --client flag must still win
	// without ever driving the interactive picker.
	clients, err := resolveSkillClients(base, v, false, marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Equal(t, []string{"claude-code"}, clients)
}

func TestResolveSkillClients_AllClientsBypassesDetectionEvenWhenEmpty(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("all-clients", true)

	clients, err := resolveSkillClients(base, v, true, marketplace.ScopeProject)

	require.NoError(t, err)
	assert.ElementsMatch(t, marketplace.SupportedClients, clients)
}

func TestResolveSkillClients_AllClientsWinsEvenInteractively(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("all-clients", true)

	clients, err := resolveSkillClients(base, v, false, marketplace.ScopeProject)

	require.NoError(t, err)
	assert.ElementsMatch(t, marketplace.SupportedClients, clients)
}

// These tests exercise resolveSkillScope directly, bypassing the interactive
// huh form, mirroring cmd/mcp/client/install_test.go's TestResolveInstallScope
// suite. newSkillInstallTestCmd/newSkillUninstallTestCmd register the same
// scope/global flags installCmd/uninstallCmd register in their own init(),
// via the package-level installParser/uninstallParser.

func newSkillInstallTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "install"}
	installParser.RegisterFlags(cmd)
	return cmd
}

func newSkillUninstallTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "uninstall"}
	uninstallParser.RegisterFlags(cmd)
	return cmd
}

func TestResolveSkillScope_ExplicitScopeWins(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("scope", marketplace.ScopeUser))
	v := viper.New()
	v.Set("scope", marketplace.ScopeUser)

	// skipPrompt is false here, but the explicit --scope flag must still win
	// without ever driving the interactive picker.
	scope, err := resolveSkillScope(cmd, v, false)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeUser, scope)
}

func TestResolveSkillScope_ExplicitGlobalSelectsUserScope(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("global", "true"))
	v := viper.New()
	v.Set("scope", marketplace.ScopeProject)
	v.Set("global", true)

	scope, err := resolveSkillScope(cmd, v, false)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeUser, scope)
}

func TestResolveSkillScope_ExplicitGlobalFalseKeepsConfiguredScope(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("global", "false"))
	v := viper.New()
	v.Set("scope", marketplace.ScopeProject)
	v.Set("global", false)

	scope, err := resolveSkillScope(cmd, v, false)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeProject, scope)
}

// TestResolveSkillScope_SkipPromptDefaultsToConfiguredScope covers the caller
// contract directly: resolveSkillScope no longer reads "yes"/"force" from
// viper itself -- each caller decides what "skip the prompt" means for its
// own flags (install.go passes its --yes value; uninstall.go passes --force,
// since it has no separate --yes). Passing skipPrompt=true here stands in
// for either caller.
func TestResolveSkillScope_SkipPromptDefaultsToConfiguredScope(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	v := viper.New()
	v.Set("scope", marketplace.ScopeProject)

	scope, err := resolveSkillScope(cmd, v, true)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeProject, scope)
}

// TestResolveSkillScope_InstallForceAloneIsNotASkipSignal is the regression
// test for the bug where `atmos ai skill install --force` (with no --yes)
// silently skipped the scope prompt: resolveSkillScope must not special-case
// "force" internally -- it only ever sees skipPrompt, which install.go's
// RunE must derive from --yes alone, never from --force ("--force" there
// means "reinstall", not "skip prompts"). Passing skipPrompt=false here
// (mirroring what install.go now does when --force is set but --yes is not)
// must NOT hit the "flag was explicitly set" short-circuit.
func TestResolveSkillScope_InstallForceAloneIsNotASkipSignal(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	v := viper.New()
	v.Set("scope", marketplace.ScopeProject)
	v.Set("force", true) // Present in viper, but resolveSkillScope must never read it.

	// skipPrompt=false: since neither --scope nor --global was Changed(), and
	// skipPrompt is false, only the non-TTY fallback (unavoidable in this
	// test process) causes a return here -- not "force".
	scope, err := resolveSkillScope(cmd, v, false)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeProject, scope)
}

func TestResolveSkillScope_NonTTYDefaultsToConfiguredScope(t *testing.T) {
	// The test environment has no real TTY attached to stdin, so
	// term.IsTTYSupportForStdin() is false and the non-interactive branch is
	// taken even with skipPrompt=false -- mirroring
	// cmd/mcp/client.TestResolveInstallScope's equivalent case.
	cmd := newSkillUninstallTestCmd(t)
	v := viper.New()
	v.Set("scope", marketplace.ScopeUser)

	scope, err := resolveSkillScope(cmd, v, false)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeUser, scope)
}
