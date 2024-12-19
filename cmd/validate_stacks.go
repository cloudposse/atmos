package cmd

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ValidateStacksCmd validates stacks
var ValidateStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Execute 'validate stacks' command",
	Long:               `This command validates stack manifest configurations: atmos validate stacks`,
	Example:            "validate stacks",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteValidateStacksCmd(cmd, args)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}

		u.PrintMessageInColor("all stacks validated successfully\n", color.New(color.FgGreen))
	},
}

func init() {
	ValidateStacksCmd.DisableFlagParsing = false

	ValidateStacksCmd.PersistentFlags().String("schemas-atmos-manifest", "", "atmos validate stacks --schemas-atmos-manifest <path-to-atmos-json-schema>")

	validateCmd.AddCommand(ValidateStacksCmd)
}
