package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRemoveCmd_Registration(t *testing.T) {
	assert.Equal(t, "remove <name>", removeCmd.Use)
	assert.NotEmpty(t, removeCmd.Short)
	assert.NotEmpty(t, removeCmd.Long)
	assert.NotNil(t, removeCmd.RunE)

	for _, name := range []string{"yes"} {
		assert.NotNil(t, removeCmd.Flags().Lookup(name), "expected %s flag", name)
	}
}

// writeMinimalAtmosYAMLForRemove writes an atmos.yaml with one configured
// mcp.server ("existing") and chdir's into its directory so executeMCPRemove
// loads it via the standard cfg.InitCliConfig discovery path.
func writeMinimalAtmosYAMLForRemove(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	body := "base_path: \".\"\nmcp:\n  servers:\n    existing:\n      command: uvx\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(body), 0o644))
	t.Chdir(tempDir)
	return tempDir
}

func newRemoveTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "remove"}
	removeParser.RegisterFlags(cmd)
	return cmd
}

func TestExecuteMCPRemove_RemovesExistingServer(t *testing.T) {
	tempDir := writeMinimalAtmosYAMLForRemove(t)
	cmd := newRemoveTestCmd(t)
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	err := executeMCPRemove(cmd, []string{"existing"})
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(tempDir, "atmos.yaml"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "existing")
}

func TestExecuteMCPRemove_UnknownServerErrors(t *testing.T) {
	writeMinimalAtmosYAMLForRemove(t)
	cmd := newRemoveTestCmd(t)
	require.NoError(t, cmd.Flags().Set("yes", "true"))

	err := executeMCPRemove(cmd, []string{"nonexistent"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPServerNotFound)
}

func TestExecuteMCPRemove_NonInteractiveWithoutYesErrors(t *testing.T) {
	writeMinimalAtmosYAMLForRemove(t)
	cmd := newRemoveTestCmd(t)

	// PromptForConfirmation returns ErrInteractiveNotAvailable when stdin
	// isn't a TTY (always true under `go test`), so omitting --yes here must
	// surface that error rather than hang.
	err := executeMCPRemove(cmd, []string{"existing"})
	require.Error(t, err)
}
