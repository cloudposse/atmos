package stack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
)

func TestStackAddCommand(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	require.NoError(t, os.WriteFile("main.tf", nil, 0o600))
	exe, err := os.Executable()
	require.NoError(t, err)
	run := func(args ...string) (string, error) {
		c := newStackAddCommand()
		c.SetArgs(args)
		var buf bytes.Buffer
		c.SetOut(&buf)
		c.SetErr(&buf)
		err := c.Execute()
		return buf.String(), err
	}
	_, err = cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	_, err = run(".", "--command=")
	require.Error(t, err)
	out, err := run(".", "--command", exe, "-s", "dev", "--dry-run")
	require.NoError(t, err)
	require.Contains(t, out, "Preview only.")
	require.NoFileExists(t, "atmos.yaml")
	_, err = run(".", "--command", exe)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(root, "stacks", "dev.yaml"))
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	old := atmosConfigPtr
	SetAtmosConfig(&config)
	t.Cleanup(func() { SetAtmosConfig(old) })
	out, err = run(".")
	require.NoError(t, err)
	require.Contains(t, out, "Already configured.")
}
