package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// docsGenerateCmd is the subcommand under docs (with alias "docs") that groups generation operations.
var docsGenerateCmd = &cobra.Command{
	Use:     "generate",
	Aliases: []string{"docs"},
	Short:   "Generate documentation artifacts",
	Long: `Generate documentation by merging YAML data sources and applying templates.
Supports native terraform-docs injection.`,
}

// docsGenerateReadmeCmd is the new subcommand under "docs generate" that specifically generates the README.
var docsGenerateReadmeCmd = &cobra.Command{
	Use:   "readme",
	Short: "Generate README.md from README.yaml and templates",
	Long: `Generate the README.md file using the README.yaml data and the configured template.
All file paths are resolved relative to the configured base-dir (default is ".").`,
	Example: `Generate the README.md in the current directory:
  atmos docs generate readme

Alternatively, using the top-level generate command:
  atmos generate docs readme`,
	RunE: e.ExecuteDocsGenerateCmd,
}

// generateDocsCmd is a new top-level command so we can do `atmos generate docs`.
var generateDocsCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generic generation commands (e.g. docs, scaffolding, etc.)",
	Long:  `A collection of subcommands for generating artifacts such as documentation or code.`,
}

func init() {
	docsCmd.AddCommand(docsGenerateCmd)
	docsGenerateCmd.AddCommand(docsGenerateReadmeCmd)

	RootCmd.AddCommand(generateDocsCmd)
	generateDocsCmd.AddCommand(docsGenerateCmd)
}
