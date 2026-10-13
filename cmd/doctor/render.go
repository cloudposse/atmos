package doctor

import (
	"encoding/json"
	"fmt"

	"github.com/cloudposse/atmos/pkg/data"
	diagnostic "github.com/cloudposse/atmos/pkg/doctor"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/validation"
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

// Render writes the report. The fix argument says whether the invocation requested repairs,
// which decides whether the summary describes applied fixes or available ones.
func render(r *diagnostic.Report, format string, fix bool) error {
	switch format {
	case formatJSON:
		return data.WriteJSON(r)
	case formatSARIF:
		return renderSARIF(r)
	case formatRich:
		ui.Writeln(validation.Rich(r.ValidationReport(), validation.RichOptions{}))
	default:
		renderFindings(r)
	}
	return renderSummary(r, fix)
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

// renderFindings prints every open finding exactly once. Progress rows only report per-file
// outcomes, so this list is the single place that carries the details.
func renderFindings(r *diagnostic.Report) {
	for i := range r.Findings {
		f := &r.Findings[i]
		if f.Status == "fixed" {
			continue
		}
		location := ""
		if f.File != "" {
			location = f.File
			if f.Line > 0 {
				location += fmt.Sprintf(":%d", f.Line)
			}
			location += " "
		}
		ui.Writef("%s%s [%s, %s] %s\n", location, f.RuleID, f.Severity, f.Status, f.Message)
		if f.Reason != "" {
			ui.Writef("  %s\n", f.Reason)
		}
		if f.Remediation != "" {
			ui.Writef("  %s\n", f.Remediation)
		}
	}
}

// findingCounts splits findings into those already repaired, those still open, and the open ones a repair run could apply.
func findingCounts(r *diagnostic.Report) (fixed, remaining, fixable int) {
	for i := range r.Findings {
		f := &r.Findings[i]
		switch {
		case f.Status == "fixed":
			fixed++
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

// Findings are summarized as applied fixes after a repair run and as available fixes otherwise.
func summarizeFindings(r *diagnostic.Report, fix bool) {
	fixed, remaining, fixable := findingCounts(r)
	switch {
	case fix || r.DryRun || fixed > 0:
		ui.Writef("Fixed %d findings in %d files. %d findings remain.\n", fixed, len(r.ChangedFiles), remaining)
	case remaining == 0:
		ui.Writeln("No findings.")
	case fixable > 0:
		ui.Writef("%d %s (%d fixable). Run atmos doctor --fix to apply %d.\n", remaining, plural(remaining, "finding", "findings"), fixable, fixable)
	default:
		ui.Writef("%d %s (none fixable).\n", remaining, plural(remaining, "finding", "findings"))
	}
}

func renderSummary(r *diagnostic.Report, fix bool) error {
	summarizeFindings(r, fix)
	for _, check := range r.Checks {
		if check.Status == "incomplete" || check.Status == "skipped" {
			ui.Writef("%s: %s. %s\n", check.ID, check.Status, check.Reason)
		}
	}
	if r.DryRun {
		ui.Writeln("Dry run: project files and backups were not written.")
	}
	if r.Backup != "" {
		ui.Writef("Backup: %s\n%s\n", r.Backup, r.Recovery)
	}
	if r.OperationalError != "" {
		ui.Error(r.OperationalError)
	}
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
