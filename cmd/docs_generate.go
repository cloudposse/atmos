package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// docsGenerateCmd is the subcommand under docs that groups generation operations.
var docsGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate documentation artifacts",
	Long: `Generate documentation by merging YAML data sources and applying templates.
Supports native terraform-docs injection.`,
	Example: `Generate the README.md in the current directory:
  atmos docs generate readme`,
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
