package cmd

import (
	"errors"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ValidateSchemaCmd represents the 'atmos validate schema' command.
//
// This command reads the 'schemas' section from the atmos.yaml configuration file,
// where each schema entry specifies a JSON schema path and a glob pattern for matching YAML files.
//
// For each entry:
//   - The JSON schema is loaded.
//   - All YAML files matching the glob pattern are discovered.
//   - Each YAML file is converted to JSON and validated against the schema.
//
// This command ensures that configuration files conform to expected structures and helps
// catch errors early in the development or deployment process.
var ValidateSchemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Validate YAML files against JSON schemas defined in atmos.yaml",
	Long: `The validate schema command reads the ` + "`" + `schemas` + "`" + ` section of the atmos.yaml file
and validates matching YAML files against their corresponding JSON schemas.

Each entry under ` + "`" + `schemas` + "`" + ` should define:
  - ` + "`" + `schema` + "`" + `: The path to the JSON schema file.
  - ` + "`" + `matches` + "`" + `: A glob pattern that specifies which YAML files to validate.

For every schema entry:
  - The JSON schema is loaded from the specified path.
  - All files matching the glob pattern are collected.
  - Each matching YAML file is parsed and converted to JSON.
  - The converted YAML is validated against the schema.

This command helps ensure that configuration files follow a defined structure
and are compliant with expected formats, reducing configuration drift and runtime errors.
`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		if err := checkAtmosConfig(); err != nil {
			return err
		}

		schema := ""
		key := ""
		if len(args) > 0 {
			key = args[0] // Use provided argument
		}

		if cmd.Flags().Changed("schemas-atmos-manifest") {
			schema, _ = cmd.Flags().GetString("schemas-atmos-manifest")
		}

		if key == "" && schema != "" {
			log.Error("key not provided for the schema to be used")
			u.OsExit(1)
		}

		if err := exec.NewAtmosValidatorExecutor(&atmosConfig).ExecuteAtmosValidateSchemaCmd(key, schema); err != nil {
			if errors.Is(err, exec.ErrInvalidYAML) {
				u.OsExit(1)
			}
			return err
		}

		return nil
	},
}

func init() {
	ValidateSchemaCmd.PersistentFlags().String("schemas-atmos-manifest", "", "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file")
	validateCmd.AddCommand(ValidateSchemaCmd)
}
