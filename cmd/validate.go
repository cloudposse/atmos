package cmd

import (
	"github.com/spf13/cobra"
)

// validateCmd validates the complete project or one of its focused validation targets.
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate project configurations",
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
	validateCmd.Flags().String("format", "", "Output format for aggregate validation: text, rich")
	RootCmd.AddCommand(validateCmd)
}
