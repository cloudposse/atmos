package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestPackerRunReturnsConfigError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATMOS_CLI_CONFIG_PATH", t.TempDir())

	cmd := &cobra.Command{Use: "build"}
	cmd.Flags().StringP("template", "t", "", "Packer template for building machine images")
	cmd.Flags().StringP("query", "q", "", "YQ expression to read an output from the Packer manifest")

	err := packerRun(cmd, "build", []string{"component", "-s", "stack"})
	require.Error(t, err)
}
