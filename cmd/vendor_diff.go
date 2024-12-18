package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// vendorDiffCmd executes 'vendor diff' CLI commands
var vendorDiffCmd = &cobra.Command{
	Use:                "diff",
	Short:              "Execute 'vendor diff' commands",
	Long:               `This command executes 'atmos vendor diff' CLI commands`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteVendorDiffCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}
	},
}

func init() {
	vendorDiffCmd.PersistentFlags().StringP("component", "c", "", "atmos vendor diff --component <component>")
	vendorDiffCmd.PersistentFlags().StringP("stack", "s", "", "atmos vendor diff --stack <stack>")
	vendorDiffCmd.PersistentFlags().StringP("type", "t", "terraform", "atmos vendor diff --component <component> --type (terraform|helmfile)")
	vendorDiffCmd.PersistentFlags().Bool("dry-run", false, "atmos vendor diff --component <component> --dry-run")

	// Since this command is not implemented yet, exclude it from `atmos help`
	// vendorCmd.AddCommand(vendorDiffCmd)
}
