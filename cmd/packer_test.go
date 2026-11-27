package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

func TestPackerRunReturnsConfigError(t *testing.T) {
	_ = NewTestKit(t)
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_CLI_CONFIG_PATH", t.TempDir())

	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().StringP("template", "t", "", "Packer template for building machine images")
	cmd.Flags().StringP("query", "q", "", "YQ expression to read an output from the Packer manifest")

	err := packerRun(cmd, "build", []string{"component", "-s", "stack"})
	require.ErrorIs(t, err, errUtils.ErrStacksDirectoryDoesNotExist)
}

func TestPackerSelectsOnlyInstance(t *testing.T) {
	_ = NewTestKit(t)
	root := componentCommandFixture(t, "packer")
	env := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(env) })
	t.Setenv("ATMOS_FORCE_TTY", "true")
	t.Setenv("ATMOS_CI_WORKER_MODE", "false")
	viper.Set("interactive", true) // TestKit resets the root flag binding.
	binary, err := os.Executable()
	require.NoError(t, err)
	marker := filepath.Join(root, "packer-args")
	t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
	setComponentCommandArgs(t, []string{"packer", "validate", "--command", binary, "--", "-syntax-only"})
	require.NoError(t, Execute())
	args, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Contains(t, string(args), "validate")
	require.Contains(t, string(args), "-syntax-only")
}
