package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/validation"
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

Out of the box, a built-in ` + "`" + `config` + "`" + ` entry validates atmos.yaml — plus atmos.d
and project-local profile fragments — against the schema generated from the Atmos
configuration code (the same document ` + "`" + `atmos config schema` + "`" + ` prints). Define your
own ` + "`" + `schemas.config` + "`" + ` entry to override or disable it.
`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runValidateSchema(cmd, args)
	},
}

// runValidateSchema executes schema validation without terminating the process.
// It can therefore be composed by aggregate validators.
func runValidateSchema(cmd *cobra.Command, args []string) error {
	// Schema validation does not require a stacks directory — atmos.yaml (and its
	// fragments) must be validatable in repositories that only carry CLI configuration.
	if err := checkAtmosConfigE(WithStackValidation(false)); err != nil {
		return err
	}

	schema := ""
	key := ""
	if len(args) > 0 {
		key = args[0]
	}

	if cmd.Flags().Changed("schemas-atmos-manifest") {
		schema, _ = cmd.Flags().GetString("schemas-atmos-manifest")
	}

	if key == "" && schema != "" {
		return errUtils.ErrValidationFailed
	}

	executor := exec.NewAtmosValidatorExecutor(&atmosConfig)
	format, err := validationFormat(cmd)
	if err != nil {
		return err
	}
	if format != validateFormatRich {
		err := executor.ExecuteAtmosValidateSchemaCmd(key, schema)
		if errors.Is(err, exec.ErrInvalidYAML) {
			errUtils.OsExit(1)
		}
		return err
	}
	report, err := executor.ValidateAtmosSchemaReport(key, schema)
	if err != nil {
		return err
	}
	if !report.HasErrors() {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "✓ All YAML schemas validated successfully")
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), validation.Rich(report, validation.DefaultRichOptions(root))); err != nil {
		return err
	}
	return errUtils.ExitCodeError{Code: 1, Silent: true}
}

func init() {
	ValidateSchemaCmd.PersistentFlags().String("schemas-atmos-manifest", "", "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file")
	addValidationFormatFlag(ValidateSchemaCmd)
	validateCmd.AddCommand(ValidateSchemaCmd)
}
