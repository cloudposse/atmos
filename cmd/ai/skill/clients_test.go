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

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?", marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Empty(t, clients, "auto mode must not fall back to installing into every supported client")
}

func TestResolveSkillClients_AutoUsesExactlyWhatWasDetected(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
	v := viper.New()

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?", marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Equal(t, []string{marketplace.ClientVSCode}, clients)
}

func TestResolveSkillClients_ExplicitClientBypassesDetection(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("client", []string{"vscode"})

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?", marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Equal(t, []string{"vscode"}, clients)
}

func TestResolveSkillClients_ExplicitClientWinsEvenInteractively(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("client", []string{"claude-code"})

	// skipPrompt is false here, but the explicit --client flag must still win
	// without ever driving the interactive picker.
	clients, err := resolveSkillClients(base, v, false, "Install skill into which clients?", marketplace.ScopeProject)

	require.NoError(t, err)
	assert.Equal(t, []string{"claude-code"}, clients)
}

func TestResolveSkillClients_AllClientsBypassesDetectionEvenWhenEmpty(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("all-clients", true)

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?", marketplace.ScopeProject)

	require.NoError(t, err)
	assert.ElementsMatch(t, marketplace.SupportedClients, clients)
}

func TestResolveSkillClients_AllClientsWinsEvenInteractively(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("all-clients", true)

	clients, err := resolveSkillClients(base, v, false, "Install skill into which clients?", marketplace.ScopeProject)

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

	scope, err := resolveSkillScope(cmd, v)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeUser, scope)
}

func TestResolveSkillScope_ExplicitGlobalSelectsUserScope(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("global", "true"))
	v := viper.New()
	v.Set("scope", marketplace.ScopeProject)
	v.Set("global", true)

	scope, err := resolveSkillScope(cmd, v)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeUser, scope)
}

func TestResolveSkillScope_ExplicitGlobalFalseKeepsConfiguredScope(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	require.NoError(t, cmd.Flags().Set("global", "false"))
	v := viper.New()
	v.Set("scope", marketplace.ScopeProject)
	v.Set("global", false)

	scope, err := resolveSkillScope(cmd, v)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeProject, scope)
}

func TestResolveSkillScope_YesSkipsPromptAndDefaultsToConfiguredScope(t *testing.T) {
	cmd := newSkillInstallTestCmd(t)
	v := viper.New()
	v.Set("scope", marketplace.ScopeProject)
	v.Set("yes", true)

	scope, err := resolveSkillScope(cmd, v)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeProject, scope)
}

func TestResolveSkillScope_ForceSkipsPromptAndDefaultsToConfiguredScope(t *testing.T) {
	// uninstall.go has no separate --yes flag, so --force must also skip the
	// scope prompt (matching resolveSkillClients's existing use of --force as
	// its skip-prompt signal for uninstall).
	cmd := newSkillUninstallTestCmd(t)
	v := viper.New()
	v.Set("scope", marketplace.ScopeUser)
	v.Set("force", true)

	scope, err := resolveSkillScope(cmd, v)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeUser, scope)
}

func TestResolveSkillScope_NonTTYDefaultsToConfiguredScope(t *testing.T) {
	// The test environment has no real TTY attached to stdin, so
	// term.IsTTYSupportForStdin() is false and the non-interactive branch is
	// taken even without --yes/--force -- mirroring
	// cmd/mcp/client.TestResolveInstallScope's equivalent case.
	cmd := newSkillUninstallTestCmd(t)
	v := viper.New()
	v.Set("scope", marketplace.ScopeUser)

	scope, err := resolveSkillScope(cmd, v)
	require.NoError(t, err)
	assert.Equal(t, marketplace.ScopeUser, scope)
}
