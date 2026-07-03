package client

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/lsp"
)

// newline is the newline character constant used throughout formatting.
const newline = "\n"

// DiagnosticFormatter formats LSP diagnostics for display.
type DiagnosticFormatter struct {
	ShowRelated bool // Show related information.
	ShowSource  bool // Show diagnostic source.
}

// NewDiagnosticFormatter creates a new diagnostic formatter.
func NewDiagnosticFormatter() *DiagnosticFormatter {
	return &DiagnosticFormatter{
		ShowRelated: true,
		ShowSource:  true,
	}
}

// FormatDiagnostics formats a list of diagnostics into a human-readable string.
func (f *DiagnosticFormatter) FormatDiagnostics(uri string, diagnostics []lsp.Diagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}

	var sb strings.Builder

	// Group diagnostics by severity.
	errors := f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityError)
	warnings := f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityWarning)
	infos := f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityInformation)
	hints := f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityHint)

	// Summary.
	fmt.Fprintf(&sb, "File: %s%s", f.formatURI(uri), newline)
	fmt.Fprintf(&sb, "Summary: %d error(s), %d warning(s), %d info(s), %d hint(s)%s%s",
		len(errors), len(warnings), len(infos), len(hints), newline, newline)

	// Format errors first.
	if len(errors) > 0 {
		sb.WriteString("ERRORS:" + newline)
		for i, diag := range errors {
			sb.WriteString(f.formatDiagnostic(i+1, &diag))
			sb.WriteString(newline)
		}
	}

	// Then warnings.
	if len(warnings) > 0 {
		sb.WriteString("WARNINGS:" + newline)
		for i, diag := range warnings {
			sb.WriteString(f.formatDiagnostic(i+1, &diag))
			sb.WriteString(newline)
		}
	}

	// Information.
	if len(infos) > 0 {
		sb.WriteString("INFORMATION:" + newline)
		for i, diag := range infos {
			sb.WriteString(f.formatDiagnostic(i+1, &diag))
			sb.WriteString(newline)
		}
	}

	// Hints.
	if len(hints) > 0 {
		sb.WriteString("HINTS:" + newline)
		for i, diag := range hints {
			sb.WriteString(f.formatDiagnostic(i+1, &diag))
			sb.WriteString(newline)
		}
	}

	return sb.String()
}

// formatDiagnostic formats a single diagnostic.
func (f *DiagnosticFormatter) formatDiagnostic(index int, diag *lsp.Diagnostic) string {
	var sb strings.Builder

	// Number and location.
	fmt.Fprintf(&sb, "%d. Line %d:%d - ", index,
		diag.Range.Start.Line+1, // LSP is 0-based, display as 1-based.
		diag.Range.Start.Character+1)

	// Source.
	if f.ShowSource && diag.Source != "" {
		fmt.Fprintf(&sb, "[%s] ", diag.Source)
	}

	// Code.
	if diag.Code != nil {
		fmt.Fprintf(&sb, "(Code: %v) ", diag.Code)
	}

	// Message.
	sb.WriteString(diag.Message)

	// Related information.
	if f.ShowRelated && len(diag.RelatedInformation) > 0 {
		sb.WriteString(newline + "   Related:")
		for _, related := range diag.RelatedInformation {
			fmt.Fprintf(&sb, "%s   - %s: %s",
				newline,
				f.formatLocation(related.Location),
				related.Message)
		}
	}

	return sb.String()
}

// FormatAllDiagnostics formats diagnostics from multiple files.
func (f *DiagnosticFormatter) FormatAllDiagnostics(diagnosticsByURI map[string][]lsp.Diagnostic) string {
	if len(diagnosticsByURI) == 0 {
		return "No diagnostics found."
	}

	var sb strings.Builder

	// Sort URIs for consistent output.
	uris := make([]string, 0, len(diagnosticsByURI))
	for uri := range diagnosticsByURI {
		uris = append(uris, uri)
	}
	sort.Strings(uris)

	// Count totals.
	totalErrors := 0
	totalWarnings := 0
	totalInfos := 0
	totalHints := 0

	for _, diagnostics := range diagnosticsByURI {
		totalErrors += len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityError))
		totalWarnings += len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityWarning))
		totalInfos += len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityInformation))
		totalHints += len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityHint))
	}

	sb.WriteString("DIAGNOSTICS SUMMARY:" + newline)
	fmt.Fprintf(&sb, "Files with issues: %d%s", len(diagnosticsByURI), newline)
	fmt.Fprintf(&sb, "Total: %d error(s), %d warning(s), %d info(s), %d hint(s)%s%s",
		totalErrors, totalWarnings, totalInfos, totalHints, newline, newline)

	// Format each file.
	for _, uri := range uris {
		diagnostics := diagnosticsByURI[uri]
		if len(diagnostics) > 0 {
			sb.WriteString(f.FormatDiagnostics(uri, diagnostics))
			sb.WriteString(newline)
		}
	}

	return sb.String()
}

// FormatCompact formats diagnostics in a compact, one-line-per-issue format.
func (f *DiagnosticFormatter) FormatCompact(uri string, diagnostics []lsp.Diagnostic) string {
	var sb strings.Builder

	for _, diag := range diagnostics {
		fmt.Fprintf(&sb, "%s:%d:%d: %s: %s",
			f.formatURI(uri),
			diag.Range.Start.Line+1,
			diag.Range.Start.Character+1,
			f.severityString(diag.Severity),
			diag.Message)

		if diag.Source != "" {
			fmt.Fprintf(&sb, " [%s]", diag.Source)
		}

		sb.WriteString(newline)
	}

	return sb.String()
}

// FormatForAI formats diagnostics in a format optimized for AI consumption.
func (f *DiagnosticFormatter) FormatForAI(uri string, diagnostics []lsp.Diagnostic) string {
	if len(diagnostics) == 0 {
		return fmt.Sprintf("No issues found in %s", f.formatURI(uri))
	}

	var sb strings.Builder

	errors := f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityError)
	warnings := f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityWarning)

	fmt.Fprintf(&sb, "Found %d issue(s) in %s:%s%s",
		len(diagnostics), f.formatURI(uri), newline, newline)

	// Group errors and warnings.
	if len(errors) > 0 {
		fmt.Fprintf(&sb, "ERRORS (%d):%s", len(errors), newline)
		for i, diag := range errors {
			fmt.Fprintf(&sb, "%d. Line %d: %s%s",
				i+1, diag.Range.Start.Line+1, diag.Message, newline)
		}
		sb.WriteString(newline)
	}

	if len(warnings) > 0 {
		fmt.Fprintf(&sb, "WARNINGS (%d):%s", len(warnings), newline)
		for i, diag := range warnings {
			fmt.Fprintf(&sb, "%d. Line %d: %s%s",
				i+1, diag.Range.Start.Line+1, diag.Message, newline)
		}
	}

	return sb.String()
}

// GetDiagnosticSummary returns a summary of diagnostics.
func (f *DiagnosticFormatter) GetDiagnosticSummary(diagnosticsByURI map[string][]lsp.Diagnostic) DiagnosticSummary {
	summary := DiagnosticSummary{
		Files: make(map[string]FileDiagnosticSummary),
	}

	for uri, diagnostics := range diagnosticsByURI {
		fileSummary := FileDiagnosticSummary{
			Errors:   len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityError)),
			Warnings: len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityWarning)),
			Infos:    len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityInformation)),
			Hints:    len(f.filterBySeverity(diagnostics, lsp.DiagnosticSeverityHint)),
		}
		summary.Files[uri] = fileSummary
		summary.TotalErrors += fileSummary.Errors
		summary.TotalWarnings += fileSummary.Warnings
		summary.TotalInfos += fileSummary.Infos
		summary.TotalHints += fileSummary.Hints
	}

	summary.FilesWithIssues = len(diagnosticsByURI)

	return summary
}

// DiagnosticSummary contains summary information about diagnostics.
type DiagnosticSummary struct {
	FilesWithIssues int                              `json:"files_with_issues"`
	TotalErrors     int                              `json:"total_errors"`
	TotalWarnings   int                              `json:"total_warnings"`
	TotalInfos      int                              `json:"total_infos"`
	TotalHints      int                              `json:"total_hints"`
	Files           map[string]FileDiagnosticSummary `json:"files"`
}

// FileDiagnosticSummary contains summary for a single file.
type FileDiagnosticSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
	Hints    int `json:"hints"`
}

// filterBySeverity filters diagnostics by severity level.
func (f *DiagnosticFormatter) filterBySeverity(diagnostics []lsp.Diagnostic, severity lsp.DiagnosticSeverity) []lsp.Diagnostic {
	var filtered []lsp.Diagnostic
	for _, diag := range diagnostics {
		if diag.Severity == severity {
			filtered = append(filtered, diag)
		}
	}
	return filtered
}

// formatURI formats a URI for display (removes file:// prefix).
func (f *DiagnosticFormatter) formatURI(uri string) string {
	return strings.TrimPrefix(uri, "file://")
}

// formatLocation formats a location for display.
func (f *DiagnosticFormatter) formatLocation(loc lsp.Location) string {
	return fmt.Sprintf("%s:%d:%d",
		f.formatURI(loc.URI),
		loc.Range.Start.Line+1,
		loc.Range.Start.Character+1)
}

// severityString returns a string representation of a severity level.
func (f *DiagnosticFormatter) severityString(severity lsp.DiagnosticSeverity) string {
	switch severity {
	case lsp.DiagnosticSeverityError:
		return "error"
	case lsp.DiagnosticSeverityWarning:
		return "warning"
	case lsp.DiagnosticSeverityInformation:
		return "info"
	case lsp.DiagnosticSeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// HasErrors returns true if there are any error-level diagnostics.
func HasErrors(diagnostics []lsp.Diagnostic) bool {
	for _, diag := range diagnostics {
		if diag.Severity == lsp.DiagnosticSeverityError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if there are any warning-level diagnostics.
func HasWarnings(diagnostics []lsp.Diagnostic) bool {
	for _, diag := range diagnostics {
		if diag.Severity == lsp.DiagnosticSeverityWarning {
			return true
		}
	}
	return false
}

// CountByeSeverity counts diagnostics by severity.
func CountBySeverity(diagnostics []lsp.Diagnostic) map[lsp.DiagnosticSeverity]int {
	counts := make(map[lsp.DiagnosticSeverity]int)
	for _, diag := range diagnostics {
		counts[diag.Severity]++
	}
	return counts
}
