package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
)

// validateConfigSchemaKey is the well-known `schemas:` registry key that
// targets the atmos.yaml schema — the built-in entry `atmos validate schema`
// seeds by default (see internal/exec).
const validateConfigSchemaKey = "config"

// ValidateConfigCmd represents the 'atmos validate config' command.
var ValidateConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Validate atmos.yaml against its JSON Schema",
	Long: `Validate the Atmos CLI configuration — atmos.yaml, atmos.d fragments, and
project-local profiles — against the JSON Schema generated from the Atmos
configuration code. This is an alias for ` + "`atmos validate schema config`" + ` (and
` + "`atmos config validate`" + `).`,
	Example: "atmos validate config",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(&atmosConfig, "cmd.validateConfigRunE")()

		return runValidateSchema(cmd, []string{validateConfigSchemaKey})
	},
}

func init() {
	addValidationFormatFlag(ValidateConfigCmd)
	validateCmd.AddCommand(ValidateConfigCmd)
}
