package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	l "github.com/cloudposse/atmos/pkg/list"
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
		checkAtmosConfig(WithStackValidation(cmd.Flag("stack").Changed))

		err := e.ExecuteVendorPullCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	vendorPullCmd.PersistentFlags().StringP("component", "c", "", "Only vendor the specified component: atmos vendor pull --component <component>")
	vendorPullCmd.PersistentFlags().StringP("stack", "s", "", "Only vendor the specified stack: atmos vendor pull --stack <stack>")
	vendorPullCmd.PersistentFlags().StringP("type", "t", "terraform", "atmos vendor pull --component <component> --type=terraform|helmfile")
	vendorPullCmd.PersistentFlags().Bool("dry-run", false, "atmos vendor pull --component <component> --dry-run")
	vendorPullCmd.PersistentFlags().String("tags", "", "Only vendor the components that have the specified tags: atmos vendor pull --tags=dev,test")

	// Autocompletion for stack flag
	vendorPullCmd.RegisterFlagCompletionFunc("stack", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		stacksList, err := l.FilterAndListStacks(toComplete)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}

		return stacksList, cobra.ShellCompDirectiveNoFileComp
	},
	)

	// Autocompletion for component flag
	vendorPullCmd.RegisterFlagCompletionFunc("component", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		componentList, err := l.FilterAndListComponents(toComplete)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}

		return componentList, cobra.ShellCompDirectiveNoFileComp
	},
	)

	vendorCmd.AddCommand(vendorPullCmd)
}
