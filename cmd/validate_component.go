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
	Long:               `This command validates atmos components using JsonSchema, OPA or CUE schemas: atmos validate component <component> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Run: func(cmd *cobra.Command, args []string) {
		err := e.ExecuteValidateStacks(cmd, args)
		if err != nil {
			u.PrintErrorToStdErrorAndExit(err)
		}
	},
}

func init() {
	validateComponentCmd.DisableFlagParsing = false

	validateComponentCmd.PersistentFlags().String("schema-path", "", "atmos validate component <component> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")
	validateComponentCmd.PersistentFlags().String("schema-type", "", "atmos validate component <component> --schema-path <schema_path> --schema-type <jsonschema|opa|cue>")

	err := validateComponentCmd.MarkPersistentFlagRequired("schema-path")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	err = validateComponentCmd.MarkPersistentFlagRequired("schema-type")
	if err != nil {
		u.PrintErrorToStdErrorAndExit(err)
	}

	validateCmd.AddCommand(validateComponentCmd)
}
