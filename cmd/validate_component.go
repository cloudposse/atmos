package cmd

import (
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// validateComponentCmd validates atmos components
var validateComponentCmd = &cobra.Command{
	Use:                "component",
	Short:              "Execute 'validate component' command",
	Long:               `This command validates an atmos component in a stack using Json Schema, OPA or CUE policies: atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteValidateComponentCmd(cmd, args)
		if err != nil {
			u.LogErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	validateComponentCmd.DisableFlagParsing = false

	validateComponentCmd.PersistentFlags().StringP("stack", "s", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")
	validateComponentCmd.PersistentFlags().String("schema-path", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")
	validateComponentCmd.PersistentFlags().String("schema-type", "jsonschema", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")

	err := validateComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorToStdErrorAndExit(err)
	}

	validateCmd.AddCommand(validateComponentCmd)
}
