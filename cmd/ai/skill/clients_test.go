package skill

import (
	"os"
	"path/filepath"
	"testing"

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

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?")

	require.NoError(t, err)
	assert.Empty(t, clients, "auto mode must not fall back to installing into every supported client")
}

func TestResolveSkillClients_AutoUsesExactlyWhatWasDetected(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".vscode"), 0o755))
	v := viper.New()

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?")

	require.NoError(t, err)
	assert.Equal(t, []string{marketplace.ClientVSCode}, clients)
}

func TestResolveSkillClients_ExplicitClientBypassesDetection(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("client", []string{"vscode"})

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?")

	require.NoError(t, err)
	assert.Equal(t, []string{"vscode"}, clients)
}

func TestResolveSkillClients_ExplicitClientWinsEvenInteractively(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("client", []string{"claude-code"})

	// skipPrompt is false here, but the explicit --client flag must still win
	// without ever driving the interactive picker.
	clients, err := resolveSkillClients(base, v, false, "Install skill into which clients?")

	require.NoError(t, err)
	assert.Equal(t, []string{"claude-code"}, clients)
}

func TestResolveSkillClients_AllClientsBypassesDetectionEvenWhenEmpty(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("all-clients", true)

	clients, err := resolveSkillClients(base, v, true, "Install skill into which clients?")

	require.NoError(t, err)
	assert.ElementsMatch(t, marketplace.SupportedClients, clients)
}

func TestResolveSkillClients_AllClientsWinsEvenInteractively(t *testing.T) {
	base := t.TempDir()
	v := viper.New()
	v.Set("all-clients", true)

	clients, err := resolveSkillClients(base, v, false, "Install skill into which clients?")

	require.NoError(t, err)
	assert.ElementsMatch(t, marketplace.SupportedClients, clients)
}
