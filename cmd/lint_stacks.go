package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/internal/exec"
)

// lintStacksCmd runs the lint stacks subcommand.
var lintStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "Lint stack configurations for quality and best practices",
	Long: `This command analyzes Atmos stack YAML configurations for optimization opportunities,
anti-patterns, and structural issues.

Linting is distinct from 'atmos validate stacks': validation checks correctness,
while linting checks quality, DRY-ness, and best practices.

Rules:
  L-01  Dead Var Detection           (warning)
  L-02  Redundant No-Op Override     (warning, auto-fixable)
  L-03  Import Depth Warning         (warning)
  L-04  Abstract Component Leak      (error)
  L-05  Catalog File Cohesion        (info)
  L-06  DRY Extraction Opportunity   (info)
  L-07  Orphaned Catalog File        (warning)
  L-08  Sensitive Var at Global Scope (warning)
  L-09  Inheritance Cycle Detection  (error)
  L-10  Env Var Shadowing            (warning)`,
	Example:            "atmos lint stacks\natmos lint stacks --format json\natmos lint stacks --rule L-09,L-04\natmos lint stacks --severity error",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		checkAtmosConfig()

		return exec.ExecuteLintStacksCmd(cmd, args)
	},
}

func init() {
	lintStacksCmd.PersistentFlags().StringP("stack", "s", "", "Scope linting to a specific stack name")
	lintStacksCmd.PersistentFlags().String("rule", "", "Comma-separated list of rule IDs to run (e.g. L-02,L-07)")
	lintStacksCmd.PersistentFlags().String("format", "text", "Output format: text (default), json")
	lintStacksCmd.PersistentFlags().String("severity", "info", "Minimum severity to report: info (default), warning, error")

	lintCmd.AddCommand(lintStacksCmd)
}
