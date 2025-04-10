package cmd

import (
	"errors"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/utils"
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
		schema := ""
		key := ""
		if len(args) > 0 {
			key = args[0] // Use provided argument
		}

		if cmd.Flags().Changed("schema") {
			schema, _ = cmd.Flags().GetString("schema")
		}

		if key == "" && schema != "" {
			log.Error("key not provided for the schema to be used")
			utils.OsExit(1)
		}

		if err := exec.NewAtmosValidatorExecuter(&atmosConfig).ExecuteAtmosValidateSchemaCmd(key, schema); err != nil {
			if errors.Is(err, exec.ErrInvalidYAML) {
				utils.OsExit(1)
			}
			u.PrintErrorMarkdownAndExit("", err, "")
		}
	},
}

func init() {
	ValidateSchemaCmd.PersistentFlags().String("schemas-atmos-manifest", "", "If you want to provide schema from external")
	ValidateSchemaCmd.PersistentFlags().String("file", "", "file to be validated")
	validateCmd.AddCommand(ValidateSchemaCmd)
}
