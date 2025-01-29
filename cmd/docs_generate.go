package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// docsGenerateCmd generates README.md
var docsGenerateCmd = &cobra.Command{
	Use:     "generate [path]",
	Aliases: []string{"docs"},
	Short:   "Generate docs (README.md) from README.yaml data and templates",
	Long: `Generate documentation by merging multiple YAML data sources
and then using templates to produce documentation files. Also supports native terraform-docs injection.`,
	Example: `Generate a README.md in the current path:
atmos docs generate

Generate a README.md for the VPC component:
atmos docs generate components/terraform/vpc

Generate all README.md (recursively searches for README.yaml to rebuild docs):
atmos docs generate --all`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return e.ExecuteDocsGenerateCmd(cmd, args)
	},
}

// generateDocsCmd is a new top-level command so we can do `atmos generate docs`.
var generateDocsCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generic generation commands (e.g. docs, scaffolding, etc.)",
	Long:  `A collection of subcommands for generating artifacts such as documentation or code.`,
}

func init() {
	docsGenerateCmd.Flags().Bool("all", false, "Recursively rebuild README.md files from any discovered README.yaml")
	docsCmd.AddCommand(docsGenerateCmd)
	RootCmd.AddCommand(generateDocsCmd)
	generateDocsCmd.AddCommand(docsGenerateCmd)
}
