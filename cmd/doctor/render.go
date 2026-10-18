package doctor

import (
	"encoding/json"
	"fmt"

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

// Render writes the report. The fix argument says whether the invocation requested repairs,
// which decides whether the summary describes applied fixes or available ones.
func render(r *diagnostic.Report, format string, fix bool) error {
	switch format {
	case formatJSON:
		return data.WriteJSON(r)
	case formatSARIF:
		return renderSARIF(r)
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

// Findings are summarized as verified repairs after a dry run, as applied fixes after a repair run,
// and as available fixes otherwise.
func summarizeFindings(r *diagnostic.Report, fix bool) {
	p := newReportRenderer()
	fixed, remaining, fixable := findingCounts(r)
	p.line("", findingsSummary(r, fix, [3]int{fixed, remaining, fixable}), p.styles.Body.Bold(true))
	if r.DryRun {
		p.line("", "Dry run: project files and backups were not written.", p.styles.Body)
	}
	if fixable > 0 && r.OperationalError == "" {
		p.line("Run: ", "atmos doctor --fix", p.styles.Command)
	}
}

// findingsSummary returns the summary sentence for the fixed, remaining, and fixable counts.
func findingsSummary(r *diagnostic.Report, fix bool, counts [3]int) string {
	fixed, remaining, fixable := counts[0], counts[1], counts[2]
	switch {
	case r.DryRun:
		verified := verifiedRepairs(r)
		return fmt.Sprintf("%d %s verified. %d %s.", verified, plural(verified, "repair", "repairs"), remaining, plural(remaining, "finding remains", "findings remain"))
	case fix || fixed > 0:
		files := len(r.ChangedFiles)
		return fmt.Sprintf("Fixed %d %s in %d %s. %d %s.", fixed, plural(fixed, "finding", "findings"), files, plural(files, "file", "files"), remaining, plural(remaining, "finding remains", "findings remain"))
	case remaining == 0:
		return "No findings."
	case fixable > 0:
		return fmt.Sprintf("%d %s (%d fixable).", remaining, plural(remaining, "finding", "findings"), fixable)
	default:
		return fmt.Sprintf("%d %s (none fixable).", remaining, plural(remaining, "finding", "findings"))
	}
}

// verifiedRepairs counts the repairs a dry run verified without writing.
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

func renderSummary(r *diagnostic.Report, fix bool) error {
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
		p.line("Backup: ", r.Backup, p.styles.Body)
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
	summarizeFindings(r, fix)
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
