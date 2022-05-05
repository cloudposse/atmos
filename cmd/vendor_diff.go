package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// vendorDiffCmd executes 'vendor diff' CLI commands
var vendorDiffCmd = &cobra.Command{
	Use:                "diff",
	Short:              "Execute 'vendor diff' commands",
	Long:               `This command executes 'atmos vendor diff' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteVendorDiff(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	vendorCmd.AddCommand(vendorDiffCmd)
	vendorDiffCmd.PersistentFlags().StringP("component", "c", "", "atmos vendor diff --component <component>")
	vendorDiffCmd.PersistentFlags().StringP("stack", "s", "", "atmos vendor diff --stack <stack>")
	vendorDiffCmd.PersistentFlags().StringP("type", "t", "terraform", "atmos vendor diff --component <component> --type (terraform|helmfile)")
	vendorDiffCmd.PersistentFlags().Bool("dry-run", false, "atmos vendor diff --component <component> --dry-run")
}
