package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	log "github.com/cloudposse/atmos/pkg/logger"
)

var validateSchemaParser = flags.NewStandardOptionsBuilder().
	WithSchemasAtmosManifest("").
	WithPositionalArgs(flags.NewValidateSchemaPositionalArgsBuilder().
		WithSchemaType(false). // Optional schemaType argument.
		Build()).
	Build()

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
	Use:   "schema [schemaType]",
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
	// Positional args are validated by the StandardParser using the builder pattern.
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		// Parse flags and positional args using builder pattern.
		// SchemaType is extracted by builder pattern into opts.SchemaType field.
		opts, err := validateSchemaParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		key := opts.SchemaType
		schema := opts.SchemasAtmosManifest

		if key == "" && schema != "" {
			log.Error("key not provided for the schema to be used")
			errUtils.OsExit(1)
		}

		if err := exec.NewAtmosValidatorExecutor(&atmosConfig).ExecuteAtmosValidateSchemaCmd(key, schema); err != nil {
			if errors.Is(err, exec.ErrInvalidYAML) {
				errUtils.OsExit(1)
			}
			return err
		}

		return nil
	},
}

func init() {
	// Register StandardOptions flags.
	validateSchemaParser.RegisterFlags(ValidateSchemaCmd)
	_ = validateSchemaParser.BindToViper(viper.GetViper())

	validateCmd.AddCommand(ValidateSchemaCmd)
}
