package cmd

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
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
			u.LogErrorAndExit(err)
		}

		cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			u.LogErrorAndExit(err)
		}

		u.LogInfo(cliConfig, "all stacks validated successfully\n")
	},
}

func init() {
	ValidateStacksCmd.DisableFlagParsing = false

	ValidateStacksCmd.PersistentFlags().String("schemas-atmos-manifest", "", "atmos validate stacks --schemas-atmos-manifest <path-to-atmos-json-schema>")

	validateCmd.AddCommand(ValidateStacksCmd)
}
