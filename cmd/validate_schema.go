package cmd

import (
	"github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// ValidateStacksCmd validates stacks
var ValidateSchemaCmd = &cobra.Command{
	Use:                "schema [optional-key]",
	Short:              "Validate schema manifest configurations",
	Long:               "This command validates the configuration of stack manifests in Atmos to ensure proper setup and compliance.",
	Example:            "validate schema [optional key]",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		arg := ""
		if len(args) > 0 {
			arg = args[0] // Use provided argument
		}
		schema, err := cmd.Flags().GetString("schema")
		if err != nil {
			u.PrintErrorMarkdown("", err, "")
		}
		if err := exec.NewAtmosValidatorExecuter(&atmosConfig).ExecuteAtmosValidateSchemaCmd(arg, schema); err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	ValidateSchemaCmd.PersistentFlags().String("schema", "", "If you want to provide schema from external")
	validateCmd.AddCommand(ValidateSchemaCmd)
}
