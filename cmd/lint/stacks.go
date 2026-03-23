package lint

import (
	_ "embed"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

//go:embed markdown/atmos_lint_stacks_usage.md
var lintStacksUsage string

// lintStacksParser is the flag parser for the lint stacks command.
var lintStacksParser *flags.StandardFlagParser

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
  L-10  Env Var Shadowing            (warning)

Controlling severity and silencing rules:
  To run only specific rules (case-insensitive):
    atmos lint stacks --rule L-02,L-07
    atmos lint stacks --rule l-02,l-7   # normalized automatically

  To silence a rule without removing it, set its severity to 'info' in
  atmos.yaml and use --severity warning or --severity error:
    lint:
      stacks:
        rules:
          L-05: info  # silenced — below default reporting threshold

  A declarative 'disabled_rules' key is not yet supported.

Stack scoping:
  When --stack is provided, linting is scoped to the import closure of that
  stack. If the stack name does not match any manifest file stem under the
  stacks base path, the command fails with a clear error (fail-closed).
  To lint all stacks, omit --stack.`,
	Example: lintStacksUsage,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()
		if err := lintStacksParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		return exec.ExecuteLintStacksCmd(cmd, args)
	},
}

func init() {
	lintStacksParser = flags.NewStandardFlagParser(
		flags.WithStringFlag("stack", "s", "", "Scope linting to a specific stack name"),
		flags.WithStringFlag("rule", "", "", "Comma-separated list of rule IDs to run (e.g. L-02,L-07)"),
		flags.WithStringFlag("format", "", "text", "Output format: text (default), json"),
		flags.WithStringFlag("severity", "", "info", "Minimum severity to report: info (default), warning, error"),
		flags.WithEnvVars("stack", "ATMOS_STACK"),
		flags.WithEnvVars("rule", "ATMOS_LINT_RULE"),
		flags.WithEnvVars("format", "ATMOS_LINT_FORMAT"),
		flags.WithEnvVars("severity", "ATMOS_LINT_SEVERITY"),
	)

	lintStacksParser.RegisterFlags(lintStacksCmd)

	if err := lintStacksParser.BindToViper(viper.GetViper()); err != nil {
		// Log error but don't fail initialization.
		// This allows the command to still work even if Viper binding fails.
		_ = err
	}

	lintCmd.AddCommand(lintStacksCmd)
}
