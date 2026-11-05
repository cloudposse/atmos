package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
)

// validateCmd validates the complete project or one of its focused validation targets.
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configurations against OPA policies and JSON schemas",
	Long: `Validate the current project. Without a subcommand, Atmos validates the
configuration schema, stack manifests, EditorConfig rules, and GitHub Actions workflows.
Use a subcommand to run one validation target, including component JSON Schema or OPA policies.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runValidateAll(cmd)
	},
}

func init() {
	addValidationFormatFlag(validateCmd)
	addAffectedValidationFlags(validateCmd)
	addValidationExcludeFlag(validateCmd)
	RootCmd.AddCommand(validateCmd)
	internal.RegisterCommandMetadata(
		validateCmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/validate/usage"),
		internal.WithExample("examples/demo-schemas", "Schema Validation"),
		internal.WithExample("examples/demo-opa", "OPA Validation"),
	)
}
