package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// vendorPullCmd executes 'vendor pull' CLI commands.
var vendorPullCmd = &cobra.Command{
	Use:                "pull",
	Short:              "Pull the latest vendor configurations or dependencies",
	Long:               "Pull and update vendor-specific configurations or dependencies to ensure the project has the latest required resources.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// WithStackValidation is a functional option that enables/disables stack configuration validation
		// based on whether the --stack flag is provided
		checkAtmosConfig(WithStackValidation(cmd.Flag("stack").Changed))

		err := e.ExecuteVendorPullCmd(cmd, args)
		return err
	},
}

func init() {
	vendorPullCmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component")
	_ = vendorPullCmd.RegisterFlagCompletionFunc("component", ComponentsArgCompletion)
	vendorPullCmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack")
	AddStackCompletion(vendorPullCmd)
	vendorPullCmd.PersistentFlags().StringP("type", "t", "terraform", "The type of the vendor (terraform or helmfile).")
	vendorPullCmd.PersistentFlags().Bool("dry-run", false, "Simulate pulling the latest version of the specified component from the remote repository without making any changes.")
	vendorPullCmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags")
	vendorPullCmd.PersistentFlags().Bool("everything", false, "Vendor all components")
	vendorCmd.AddCommand(vendorPullCmd)
}
