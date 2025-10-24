package lsp

import (
	"fmt"
	"sort"
	"strings"
)

// DiagnosticFormatter formats LSP diagnostics for display.
type DiagnosticFormatter struct {
	ShowRelated bool // Show related information
	ShowSource  bool // Show diagnostic source
}

// NewDiagnosticFormatter creates a new diagnostic formatter.
func NewDiagnosticFormatter() *DiagnosticFormatter {
	return &DiagnosticFormatter{
		ShowRelated: true,
		ShowSource:  true,
	}
}

// FormatDiagnostics formats a list of diagnostics into a human-readable string.
func (f *DiagnosticFormatter) FormatDiagnostics(uri string, diagnostics []Diagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}

	var sb strings.Builder

	// Group diagnostics by severity
	errors := f.filterBySeverity(diagnostics, DiagnosticSeverityError)
	warnings := f.filterBySeverity(diagnostics, DiagnosticSeverityWarning)
	infos := f.filterBySeverity(diagnostics, DiagnosticSeverityInformation)
	hints := f.filterBySeverity(diagnostics, DiagnosticSeverityHint)

	// Summary
	sb.WriteString(fmt.Sprintf("File: %s\n", f.formatURI(uri)))
	sb.WriteString(fmt.Sprintf("Summary: %d error(s), %d warning(s), %d info(s), %d hint(s)\n\n",
		len(errors), len(warnings), len(infos), len(hints)))

	// Format errors first
	if len(errors) > 0 {
		sb.WriteString("ERRORS:\n")
		for i, diag := range errors {
			sb.WriteString(f.formatDiagnostic(i+1, diag))
			sb.WriteString("\n")
		}
	}

	// Then warnings
	if len(warnings) > 0 {
		sb.WriteString("WARNINGS:\n")
		for i, diag := range warnings {
			sb.WriteString(f.formatDiagnostic(i+1, diag))
			sb.WriteString("\n")
		}
	}

	// Information
	if len(infos) > 0 {
		sb.WriteString("INFORMATION:\n")
		for i, diag := range infos {
			sb.WriteString(f.formatDiagnostic(i+1, diag))
			sb.WriteString("\n")
		}
	}

	// Hints
	if len(hints) > 0 {
		sb.WriteString("HINTS:\n")
		for i, diag := range hints {
			sb.WriteString(f.formatDiagnostic(i+1, diag))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatDiagnostic formats a single diagnostic.
func (f *DiagnosticFormatter) formatDiagnostic(index int, diag Diagnostic) string {
	var sb strings.Builder

	// Number and location
	sb.WriteString(fmt.Sprintf("%d. Line %d:%d - ", index,
		diag.Range.Start.Line+1, // LSP is 0-based, display as 1-based
		diag.Range.Start.Character+1))

	// Source
	if f.ShowSource && diag.Source != "" {
		sb.WriteString(fmt.Sprintf("[%s] ", diag.Source))
	}

	// Code
	if diag.Code != nil {
		sb.WriteString(fmt.Sprintf("(Code: %v) ", diag.Code))
	}

	// Message
	sb.WriteString(diag.Message)

	// Related information
	if f.ShowRelated && len(diag.RelatedInformation) > 0 {
		sb.WriteString("\n   Related:")
		for _, related := range diag.RelatedInformation {
			sb.WriteString(fmt.Sprintf("\n   - %s: %s",
				f.formatLocation(related.Location),
				related.Message))
		}
	}

	return sb.String()
}

// FormatAllDiagnostics formats diagnostics from multiple files.
func (f *DiagnosticFormatter) FormatAllDiagnostics(diagnosticsByURI map[string][]Diagnostic) string {
	if len(diagnosticsByURI) == 0 {
		return "No diagnostics found."
	}

	var sb strings.Builder

	// Sort URIs for consistent output
	uris := make([]string, 0, len(diagnosticsByURI))
	for uri := range diagnosticsByURI {
		uris = append(uris, uri)
	}
	sort.Strings(uris)

	// Count totals
	totalErrors := 0
	totalWarnings := 0
	totalInfos := 0
	totalHints := 0

	for _, diagnostics := range diagnosticsByURI {
		totalErrors += len(f.filterBySeverity(diagnostics, DiagnosticSeverityError))
		totalWarnings += len(f.filterBySeverity(diagnostics, DiagnosticSeverityWarning))
		totalInfos += len(f.filterBySeverity(diagnostics, DiagnosticSeverityInformation))
		totalHints += len(f.filterBySeverity(diagnostics, DiagnosticSeverityHint))
	}

	sb.WriteString(fmt.Sprintf("DIAGNOSTICS SUMMARY:\n"))
	sb.WriteString(fmt.Sprintf("Files with issues: %d\n", len(diagnosticsByURI)))
	sb.WriteString(fmt.Sprintf("Total: %d error(s), %d warning(s), %d info(s), %d hint(s)\n\n",
		totalErrors, totalWarnings, totalInfos, totalHints))

	// Format each file
	for _, uri := range uris {
		diagnostics := diagnosticsByURI[uri]
		if len(diagnostics) > 0 {
			sb.WriteString(f.FormatDiagnostics(uri, diagnostics))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatCompact formats diagnostics in a compact, one-line-per-issue format.
func (f *DiagnosticFormatter) FormatCompact(uri string, diagnostics []Diagnostic) string {
	var sb strings.Builder

	for _, diag := range diagnostics {
		sb.WriteString(fmt.Sprintf("%s:%d:%d: %s: %s",
			f.formatURI(uri),
			diag.Range.Start.Line+1,
			diag.Range.Start.Character+1,
			f.severityString(diag.Severity),
			diag.Message))

		if diag.Source != "" {
			sb.WriteString(fmt.Sprintf(" [%s]", diag.Source))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatForAI formats diagnostics in a format optimized for AI consumption.
func (f *DiagnosticFormatter) FormatForAI(uri string, diagnostics []Diagnostic) string {
	if len(diagnostics) == 0 {
		return fmt.Sprintf("No issues found in %s", f.formatURI(uri))
	}

	var sb strings.Builder

	errors := f.filterBySeverity(diagnostics, DiagnosticSeverityError)
	warnings := f.filterBySeverity(diagnostics, DiagnosticSeverityWarning)

	sb.WriteString(fmt.Sprintf("Found %d issue(s) in %s:\n\n",
		len(diagnostics), f.formatURI(uri)))

	// Group errors and warnings
	if len(errors) > 0 {
		sb.WriteString(fmt.Sprintf("ERRORS (%d):\n", len(errors)))
		for i, diag := range errors {
			sb.WriteString(fmt.Sprintf("%d. Line %d: %s\n",
				i+1, diag.Range.Start.Line+1, diag.Message))
		}
		sb.WriteString("\n")
	}

	if len(warnings) > 0 {
		sb.WriteString(fmt.Sprintf("WARNINGS (%d):\n", len(warnings)))
		for i, diag := range warnings {
			sb.WriteString(fmt.Sprintf("%d. Line %d: %s\n",
				i+1, diag.Range.Start.Line+1, diag.Message))
		}
	}

	return sb.String()
}

// GetDiagnosticSummary returns a summary of diagnostics.
func (f *DiagnosticFormatter) GetDiagnosticSummary(diagnosticsByURI map[string][]Diagnostic) DiagnosticSummary {
	summary := DiagnosticSummary{
		Files: make(map[string]FileDiagnosticSummary),
	}

	for uri, diagnostics := range diagnosticsByURI {
		fileSummary := FileDiagnosticSummary{
			Errors:   len(f.filterBySeverity(diagnostics, DiagnosticSeverityError)),
			Warnings: len(f.filterBySeverity(diagnostics, DiagnosticSeverityWarning)),
			Infos:    len(f.filterBySeverity(diagnostics, DiagnosticSeverityInformation)),
			Hints:    len(f.filterBySeverity(diagnostics, DiagnosticSeverityHint)),
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
func (f *DiagnosticFormatter) filterBySeverity(diagnostics []Diagnostic, severity DiagnosticSeverity) []Diagnostic {
	var filtered []Diagnostic
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
func (f *DiagnosticFormatter) formatLocation(loc Location) string {
	return fmt.Sprintf("%s:%d:%d",
		f.formatURI(loc.URI),
		loc.Range.Start.Line+1,
		loc.Range.Start.Character+1)
}

// severityString returns a string representation of a severity level.
func (f *DiagnosticFormatter) severityString(severity DiagnosticSeverity) string {
	switch severity {
	case DiagnosticSeverityError:
		return "error"
	case DiagnosticSeverityWarning:
		return "warning"
	case DiagnosticSeverityInformation:
		return "info"
	case DiagnosticSeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// HasErrors returns true if there are any error-level diagnostics.
func HasErrors(diagnostics []Diagnostic) bool {
	for _, diag := range diagnostics {
		if diag.Severity == DiagnosticSeverityError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if there are any warning-level diagnostics.
func HasWarnings(diagnostics []Diagnostic) bool {
	for _, diag := range diagnostics {
		if diag.Severity == DiagnosticSeverityWarning {
			return true
		}
	}
	return false
}

// CountByeSeverity counts diagnostics by severity.
func CountBySeverity(diagnostics []Diagnostic) map[DiagnosticSeverity]int {
	counts := make(map[DiagnosticSeverity]int)
	for _, diag := range diagnostics {
		counts[diag.Severity]++
	}
	return counts
}
