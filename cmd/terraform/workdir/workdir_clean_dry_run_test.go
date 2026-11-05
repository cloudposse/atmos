package workdir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
)

func TestCleanComponentDryRunPreservesFiles(t *testing.T) {
	base := t.TempDir()
	createTestAtmosConfig(t, base)
	t.Chdir(base)
	t.Setenv("ATMOS_BASE_PATH", base)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", base)
	resetViperForTest(t)
	t.Cleanup(func() { resetViperForTest(t) })
	v := viper.GetViper()
	v.Set("stack", "dev")
	v.Set("dry-run", true)

	path, err := provWorkdir.BuildPath(base, "terraform", "vpc", "dev", nil)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(path, 0o755))
	sentinel := filepath.Join(path, "main.tf")
	require.NoError(t, os.WriteFile(sentinel, []byte("# preserve on dry run\n"), 0o644))

	require.NoError(t, cleanCmd.RunE(cleanCmd, []string{"vpc"}))
	require.FileExists(t, sentinel)

	// A real cleanup must still remove the same workdir.
	v.Set("dry-run", false)
	require.NoError(t, cleanCmd.RunE(cleanCmd, []string{"vpc"}))
	require.NoDirExists(t, path)
}
