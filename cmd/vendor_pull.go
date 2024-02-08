package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// vendorPullCmd executes 'vendor pull' CLI commands
var vendorPullCmd = &cobra.Command{
	Use:                "pull",
	Short:              "Execute 'vendor pull' commands",
	Long:               `This command executes 'atmos vendor pull' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteVendorPullCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	vendorPullCmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component: atmos vendor pull --component <component>")
	vendorPullCmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack: atmos vendor pull --stack <stack>")
	vendorPullCmd.PersistentFlags().StringP("type", "t", "terraform", "atmos vendor pull --component <component> --type=terraform|helmfile")
	vendorPullCmd.PersistentFlags().Bool("dry-run", false, "atmos vendor pull --component <component> --dry-run")
	vendorPullCmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags: atmos vendor pull --tags=dev,test")

	vendorCmd.AddCommand(vendorPullCmd)
}
