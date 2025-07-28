package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/list"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// listDeploymentsCmd lists atmos deployments.
var listDeploymentsCmd = &cobra.Command{
	Use:                "deployments",
	Short:              "List all Atmos deployments",
	Long:               "This command lists all Atmos deployments or is used to upload deployments to the pro API.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()
		err := ExecuteListDeploymentsCmd(cmd, args)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	// Add common list flags
	fl.AddCommonListFlags(listDeploymentsCmd)

	// Add deployment-specific flags
	listDeploymentsCmd.Flags().Bool("upload", false, "Upload deployments to pro API")

	// Add the command to the list command
	listCmd.AddCommand(listDeploymentsCmd)
}

func ExecuteListDeploymentsCmd(cmd *cobra.Command, args []string) error {
	info := &schema.ConfigAndStacksInfo{}
	info.Command = "list"
	info.SubCommand = "deployments"

	// Process and validate command line arguments
	_, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return err
	}

	return list.ExecuteListDeploymentsCmd(info, cmd, args)
}
