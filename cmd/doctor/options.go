package doctor

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
)

// usageExitCode is the exit status for invalid invocations.
const usageExitCode = 2

var failOnThresholds = []string{"error", "warning", "none"}

// listChecksIncompatible are flags that have no meaning when only listing checks.
// --format and --fail-on stay valid: format selects the listing encoding and a
// threshold is harmless because listing produces no findings.
var listChecksIncompatible = []string{"fix", "dry-run", "affected", "base", "exclude", "only", "offline"}

// optionError builds an invalid-invocation error that names the option and carries guidance.
func optionError(message string, hints ...string) error {
	builder := errUtils.Build(fmt.Errorf("%w: %s", diagnostic.ErrOptions, message)).WithExitCode(usageExitCode)
	for _, hint := range hints {
		builder = builder.WithHint(hint)
	}
	return builder.Err()
}

// invalidValueError reports an option value that is not one of the supported values.
func invalidValueError(option, value string, valid []string) error {
	return optionError(
		fmt.Sprintf("--%s=%q is not supported", option, value),
		fmt.Sprintf("Valid values for --%s: %s", option, strings.Join(valid, ", ")),
	)
}

// validateFormat checks --format on its own, because an invalid format cannot select the error report encoding.
func validateFormat(format string) error {
	if slices.Contains(formats, format) {
		return nil
	}
	return invalidValueError("format", format, formats)
}

// validateInvocation rejects invalid option values and combinations before any project input is read.
func validateInvocation(cmd *cobra.Command, format string) error {
	flags := cmd.Flags()
	threshold, _ := flags.GetString("fail-on")
	if !slices.Contains(failOnThresholds, threshold) {
		return invalidValueError("fail-on", threshold, failOnThresholds)
	}
	if list, _ := flags.GetBool("list-checks"); list {
		return validateListChecks(cmd, format)
	}
	fix, _ := flags.GetBool("fix")
	if dryRun, _ := flags.GetBool("dry-run"); dryRun && !fix {
		return optionError("--dry-run requires --fix", "Add --fix to preview repairs without writing files, for example: atmos doctor --fix --dry-run")
	}
	affected, _ := flags.GetBool("affected")
	if flags.Changed("base") && !affected {
		return optionError("--base requires --affected", "Add --affected to check only inputs changed since the base, for example: atmos doctor --affected --base=main")
	}
	return validateOnly(cmd)
}

func validateListChecks(cmd *cobra.Command, format string) error {
	if format == formatRich {
		return optionError("--format=rich is not supported with --list-checks", "Use --format=text, --format=json, or --format=sarif to list checks")
	}
	for _, name := range listChecksIncompatible {
		if cmd.Flags().Changed(name) {
			return optionError(
				fmt.Sprintf("--%s cannot be combined with --list-checks", name),
				"--list-checks only prints the check catalog; run atmos doctor without it to inspect a project",
			)
		}
	}
	return nil
}

// validateOnly rejects --only selectors that match no built-in check ID or category.
func validateOnly(cmd *cobra.Command) error {
	only, _ := cmd.Flags().GetStringSlice("only")
	if len(only) == 0 {
		return nil
	}
	var valid []string
	for _, rule := range diagnostic.Rules() {
		for _, name := range []string{rule.ID, rule.Category} {
			if !slices.Contains(valid, name) {
				valid = append(valid, name)
			}
		}
	}
	for _, selector := range only {
		if !slices.Contains(valid, selector) {
			return optionError(
				fmt.Sprintf("--only=%q does not match a check or category", selector),
				"Valid values for --only: "+strings.Join(valid, ", "),
				"Run atmos doctor --list-checks to see every check",
			)
		}
	}
	return nil
}
