package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/telemetry"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:                "dependents",
	Aliases:            []string{"dependants"},
	Short:              "List Atmos components that depend on a given component",
	Long:               "This command generates a list of Atmos components within stacks that depend on the specified Atmos component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  ComponentsArgCompletion,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()

		err := e.ExecuteDescribeDependentsCmd(cmd, args)
		if err != nil {
			telemetry.CaptureCmdFailure(cmd)
			u.PrintErrorMarkdownAndExit("", err, "")
		}
		telemetry.CaptureCmd(cmd)
	},
}

func init() {
	describeDependentsCmd.DisableFlagParsing = false

	AddStackCompletion(describeDependentsCmd)
	describeDependentsCmd.PersistentFlags().StringP("format", "f", "json", "The output format (`json` is default)")
	describeDependentsCmd.PersistentFlags().String("file", "", "Write the result to the file")

	err := describeDependentsCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		telemetry.CaptureCmdFailure(describeDependentsCmd)
		u.LogErrorAndExit(err)
	}

	describeCmd.AddCommand(describeDependentsCmd)
}
