package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	fl "github.com/cloudposse/atmos/pkg/list/flags"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// listDeploymentsCmd lists atmos deployments
var listDeploymentsCmd = &cobra.Command{
	Use:                "deployments",
	Short:              "List all Atmos deployments",
	Long:               "This command lists all Atmos deployments, or filters the list to show only the deployments with drift detection enabled.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		err := e.ExecuteListDeploymentsCmd(cmd, args)
		if err != nil {
			u.PrintErrorMarkdownAndExit("Error listing deployments", err, "")
			return
		}
	},
}

func init() {
	// Add common list flags
	fl.AddCommonListFlags(listDeploymentsCmd)

	// Add deployment-specific flags
	listDeploymentsCmd.Flags().StringP("stack", "s", "", "Filter deployments by stack")
	listDeploymentsCmd.Flags().Bool("drift-enabled", false, "Filter deployments with drift detection enabled")
	listDeploymentsCmd.Flags().Bool("upload", false, "Upload deployments to pro API")

	// Add the command to the list command
	listCmd.AddCommand(listDeploymentsCmd)
}
