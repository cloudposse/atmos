package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// vendorPullCmd executes 'vendor pull' CLI commands
var vendorPullCmd = &cobra.Command{
	Use:                "pull",
	Short:              "Execute 'vendor pull' commands",
	Long:               `This command executes 'atmos vendor pull' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// WithStackValidation is a functional option that enables/disables stack configuration validation
		// based on whether the --stack flag is provided
		atmosConfig := cmd.Context().Value(contextKey("atmos_config")).(schema.AtmosConfiguration)
		checkAtmosConfig(&atmosConfig, WithStackValidation(cmd.Flag("stack").Changed))

		err := e.ExecuteVendorPullCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	vendorPullCmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component: atmos vendor pull --component <component>")
	vendorPullCmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack: atmos vendor pull --stack <stack>")
	vendorPullCmd.PersistentFlags().StringP("type", "t", "terraform", "atmos vendor pull --component <component> --type=terraform|helmfile")
	vendorPullCmd.PersistentFlags().Bool("dry-run", false, "atmos vendor pull --component <component> --dry-run")
	vendorPullCmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags: atmos vendor pull --tags=dev,test")
	vendorPullCmd.PersistentFlags().Bool("everything", false, "Vendor all components: atmos vendor pull --everything")
	vendorCmd.AddCommand(vendorPullCmd)
}
