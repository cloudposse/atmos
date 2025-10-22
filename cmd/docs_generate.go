package cmd

import (
	_ "embed"

	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

//go:embed markdown/atmos_docs_generate_usage.md
var docsGenerateUsageMarkdown string

// docsGenerateCmd is the subcommand under docs that groups generation operations.
var docsGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate documentation artifacts",
	Long: `Generate documentation by merging YAML data sources and applying templates.
Supports native terraform-docs injection.`,
	Example:   docsGenerateUsageMarkdown,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"readme"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return ErrInvalidArguments
		}
		err := e.ExecuteDocsGenerateCmd(cmd, args)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	docsCmd.AddCommand(docsGenerateCmd)
}
