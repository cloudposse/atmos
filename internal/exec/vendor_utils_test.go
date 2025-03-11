package exec

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestExecuteVendorPullCommand(t *testing.T) {
	stacksPath := "../../tests/fixtures/scenarios/vendor2"

	err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

	err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
	assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

	var cmd = &cobra.Command{
		Use:                "pull",
		Short:              "Pull the latest vendor configurations or dependencies",
		Long:               "Pull and update vendor-specific configurations or dependencies to ensure the project has the latest required resources.",
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
		Args:               cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := ExecuteVendorPullCmd(cmd, args)
			if err != nil {
				return err
			}
			return nil
		},
	}

	cmd.DisableFlagParsing = false
	cmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component")
	cmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack")
	cmd.PersistentFlags().StringP("type", "t", "terraform", "The type of the vendor (terraform or helmfile).")
	cmd.PersistentFlags().Bool("dry-run", false, "Simulate pulling the latest version of the specified component from the remote repository without making any changes.")
	cmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags")
	cmd.PersistentFlags().Bool("everything", false, "Vendor all components")

	// Execute the command
	err = cmd.RunE(cmd, []string{})
	assert.NoError(t, err, "'atmos vendor pull' command should execute without error")
}
