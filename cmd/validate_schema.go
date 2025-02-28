package cmd

import (
	"os"

	"github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
)

// ValidateStacksCmd validates schema.
var ValidateSchemaCmd = &cobra.Command{
	Use:                "schema",
	Short:              "Validate schema manifest configurations",
	Long:               "This command validates the configuration of stack manifests in Atmos to ensure proper setup and compliance.",
	Example:            "validate schema [optional key]",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		fileName := ""
		schema := ""
		key := ""
		if len(args) > 0 {
			key = args[0] // Use provided argument
		}
		if cmd.Flags().Changed("file") {
			fileName, _ = cmd.Flags().GetString("file")
		}
		if cmd.Flags().Changed("schema") {
			schema, _ = cmd.Flags().GetString("schemas-atmos-manifest")
		}
		if schema == "" {
			schema = os.Getenv("ATMOS_SCHEMAS_ATMOS_MANIFEST")
		}
		if err := exec.NewAtmosValidatorExecuter(&atmosConfig).ExecuteAtmosValidateSchemaCmd(fileName, schema, key); err != nil {
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	ValidateSchemaCmd.PersistentFlags().String("schemas-atmos-manifest", "", "If you want to provide schema from external")
	ValidateSchemaCmd.PersistentFlags().String("file", "", "file to be validated")
	validateCmd.AddCommand(ValidateSchemaCmd)
}
