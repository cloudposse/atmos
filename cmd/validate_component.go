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
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	validateComponentCmd.DisableFlagParsing = false

	validateComponentCmd.PersistentFlags().StringP("stack", "s", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")
	validateComponentCmd.PersistentFlags().String("schema-path", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")
	validateComponentCmd.PersistentFlags().String("schema-type", "", "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")
	validateComponentCmd.PersistentFlags().StringSlice("module-paths", nil, "atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type opa --module-paths constants,helpers")
	validateComponentCmd.PersistentFlags().Int("timeout", 0, "Validation timeout in seconds: atmos validate component <component> -s <stack> --timeout 15")

	err := validateComponentCmd.MarkPersistentFlagRequired("stack")
	if err != nil {
		u.LogErrorAndExit(err)
	}

	validateCmd.AddCommand(validateComponentCmd)
}
