package terraform

import (
	"github.com/spf13/cobra"
)

// varfileCmd represents the terraform varfile command (custom Atmos command).
var varfileCmd = &cobra.Command{
	Use:                "varfile",
	Short:              "Load variables from a file",
	Long:               `Load variable definitions from a specified file and use them in the configuration.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

// writeVarfileCmd represents the terraform write varfile command (custom Atmos command).
var writeVarfileCmd = &cobra.Command{
	Use:                "write varfile",
	Short:              "Write variables to a file",
	Long:               `Write the variables used in the configuration to a specified file for later use or modification.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return terraformRun(terraformCmd, cmd, args)
	},
}

func init() {
	// Register completions for varfileCmd.
	RegisterTerraformCompletions(varfileCmd)

	// Attach to parent terraform command.
	terraformCmd.AddCommand(varfileCmd)
	terraformCmd.AddCommand(writeVarfileCmd)
}
