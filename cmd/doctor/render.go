package doctor

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"al.essio.dev/pkg/shellescape"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/data"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	formatText  = "text"
	formatRich  = "rich"
	formatJSON  = "json"
	formatSARIF = "sarif"
)

// Supported report formats, in the order shown to users.
var formats = []string{formatText, formatRich, formatJSON, formatSARIF}

// Machine formats are consumed by tools and own stdout; isMachineFormat reports whether format is one.
func isMachineFormat(format string) bool {
	return format == formatJSON || format == formatSARIF
}

const (
	// RuleRecovery is the rule ID of notices that tell the user how to recover files; they are guidance,
	// not findings left in the project.
	ruleRecovery = "recovery"
	// RuleExclude is the rule ID of notices about --exclude patterns that matched nothing; they ask the
	// user to check the pattern and are never repaired.
	ruleExclude = "exclude"
	// BaseFixCommand is the repair command suggested when the invocation carried no scoping flags.
	baseFixCommand = "atmos doctor --fix"
)

// fixFlagsSkipped are the flags a suggested repair command replaces or has no use for.
var fixFlagsSkipped = []string{"fix", "dry-run", "format", "list-checks", "help"}

// renderOptions carries the invocation facts a rendered report needs.
type renderOptions struct {
	// Fix says whether the invocation requested repairs, which decides whether the summary describes
	// applied fixes or available ones.
	Fix bool
	// FixCommand is the repair command to suggest. It preserves the scoping flags of the invocation;
	// empty means the plain command.
	FixCommand string
}

// fixCommandOrDefault returns the suggested repair command.
func (o renderOptions) fixCommandOrDefault() string {
	if o.FixCommand == "" {
		return baseFixCommand
	}
	return o.FixCommand
}

// fixCommand builds the repair command for the invocation: "atmos doctor --fix" plus every flag the user
// set, root persistent flags included, so the suggestion repairs exactly what this run inspected.
func fixCommand(cmd *cobra.Command) string {
	parts := []string{baseFixCommand}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if slices.Contains(fixFlagsSkipped, f.Name) {
			return
		}
		for _, arg := range flagArguments(f) {
			parts = append(parts, shellescape.Quote(arg))
		}
	})
	return strings.Join(parts, reportSpace)
}

// flagArguments renders a set flag as command-line arguments, one "--name=value" per element for list flags.
func flagArguments(f *pflag.Flag) []string {
	if list, ok := f.Value.(pflag.SliceValue); ok {
		values := list.GetSlice()
		args := make([]string, 0, len(values))
		for _, value := range values {
			args = append(args, "--"+f.Name+equalsSign+value)
		}
		return args
	}
	if f.Value.Type() == "bool" {
		if enabled, err := strconv.ParseBool(f.Value.String()); err == nil && enabled {
			return []string{"--" + f.Name}
		}
	}
	return []string{"--" + f.Name + equalsSign + f.Value.String()}
}

// Render writes the report using the invocation facts in opts.
func render(r *diagnostic.Report, format string, opts renderOptions) error {
	switch format {
	case formatJSON:
		return data.WriteJSON(r)
	case formatSARIF:
		return renderSARIF(r)
	default:
		renderFindings(r)
	}
	return renderSummary(r, opts)
}

func renderSARIF(r *diagnostic.Report) error {
	out, err := r.ValidationReport().SARIF()
	if err != nil {
		return err
	}
	var document map[string]any
	if err := json.Unmarshal(out, &document); err != nil {
		return err
	}
	if runs, ok := document["runs"].([]any); ok && len(runs) > 0 {
		if run, ok := runs[0].(map[string]any); ok {
			run["properties"] = map[string]any{"doctor": r}
			run["invocations"] = []any{map[string]any{"executionSuccessful": r.OperationalError == ""}}
		}
	}
	return data.WriteJSON(document)
}

// findingCounts splits findings into those already repaired, those still open, and the open ones a repair run could apply.
func findingCounts(r *diagnostic.Report) (fixed, remaining, fixable int) {
	for i := range r.Findings {
		f := &r.Findings[i]
		switch {
		case f.Status == "fixed":
			fixed++
		case f.RuleID == ruleRecovery || f.RuleID == ruleExclude:
			// Recovery and exclude notices are guidance; they never remain to be fixed.
		case f.Fixable:
			remaining++
			fixable++
		default:
			remaining++
		}
	}
	return fixed, remaining, fixable
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

// Findings are summarized as verified repairs after a dry run, as applied fixes after a repair run,
// and as available fixes otherwise.
func summarizeFindings(r *diagnostic.Report, opts renderOptions) {
	p := newReportRenderer()
	fixed, remaining, fixable := findingCounts(r)
	p.line("", findingsSummary(r, opts, [3]int{fixed, remaining, fixable}), p.styles.Body.Bold(true))
	if r.DryRun {
		p.line("", "Dry run: project files and backups were not written.", p.styles.Body)
	}
	if fixable > 0 && r.OperationalError == "" {
		p.line("Run: ", opts.fixCommandOrDefault(), p.styles.Command)
	}
}

// findingsSummary returns the one-line summary for the counts in order fixed, remaining, fixable.
func findingsSummary(r *diagnostic.Report, opts renderOptions, counts [3]int) string {
	fixed, remaining, fixable := counts[0], counts[1], counts[2]
	remains := fmt.Sprintf("%d %s", remaining, plural(remaining, "finding remains", "findings remain"))
	switch {
	case r.DryRun:
		verified := verifiedRepairs(r)
		return fmt.Sprintf("%d %s verified. %s.", verified, plural(verified, "repair", "repairs"), remains)
	case opts.Fix && fixed == 0 && len(r.ChangedFiles) == 0:
		return nothingToFix(remaining)
	case opts.Fix || fixed > 0:
		files := len(r.ChangedFiles)
		return fmt.Sprintf("Fixed %d %s in %d %s. %s.", fixed, plural(fixed, "finding", "findings"), files, plural(files, "file", "files"), remains)
	case remaining == 0:
		return "No findings."
	case fixable > 0:
		return fmt.Sprintf("%d %s (%d fixable).", remaining, plural(remaining, "finding", "findings"), fixable)
	}
	return fmt.Sprintf("%d %s (none fixable).", remaining, plural(remaining, "finding", "findings"))
}

// verifiedRepairs counts the repairs a dry run verified without writing them.
func verifiedRepairs(r *diagnostic.Report) int {
	verified := 0
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Fixable && f.Reason == "Candidate verified; dry run left the source unchanged" {
			verified++
		}
	}
	return verified
}

// nothingToFix summarizes a repair run that changed nothing, so it never claims "Fixed 0 findings in 0 files".
func nothingToFix(remaining int) string {
	if remaining == 0 {
		return "Nothing to fix."
	}
	return fmt.Sprintf("Nothing to fix. %d %s.", remaining, plural(remaining, "finding remains", "findings remain"))
}

func renderSummary(r *diagnostic.Report, opts renderOptions) error {
	p := newReportRenderer()
	checksHeading := false
	for _, check := range r.Checks {
		if check.Status != "incomplete" && check.Status != "skipped" {
			continue
		}
		if !checksHeading {
			p.line("", "Check coverage", p.styles.Label)
			checksHeading = true
		}
		p.line("  ", fmt.Sprintf("%s: %s. %s", check.ID, check.Status, check.Reason), p.styles.Body)
	}
	if checksHeading {
		ui.Writeln("")
	}
	if r.Backup != "" {
		p.line("", "Recovery", p.styles.Label)
		// The path is never wrapped: a path broken across lines cannot be copied.
		p.unwrapped("Backup: ", r.Backup, p.styles.Body)
		if r.Recovery != "" {
			p.line("", r.Recovery, p.styles.Command)
		}
		ui.Writeln("")
	}
	if r.OperationalError != "" {
		p.line("", "Operational error", p.styles.Error.Bold(true))
		p.line("", r.OperationalError, p.styles.Body)
		ui.Writeln("")
	}
	p.line("", "Summary", p.styles.Label)
	summarizeFindings(r, opts)
	return nil
}

// listChecks prints the built-in check catalog as data on stdout.
func listChecks(format string) error {
	switch format {
	case formatJSON:
		return data.WriteJSON(diagnostic.Rules())
	case formatSARIF:
		report := diagnostic.Report{Version: 1}
		for _, rule := range diagnostic.Rules() {
			report.Checks = append(report.Checks, diagnostic.Check{ID: rule.ID, Status: "listed", Reason: rule.Description})
		}
		return renderSARIF(&report)
	}
	for _, r := range diagnostic.Rules() {
		if err := data.Writef("%s [%s]: %s\n", r.ID, r.Category, r.Description); err != nil {
			return err
		}
	}
	return nil
}
