package client

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/lsp"
)

func TestDiagnosticFormatter_FormatForAI(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	tests := []struct {
		name           string
		uri            string
		diagnostics    []lsp.Diagnostic
		expectContains []string
	}{
		{
			name:        "No diagnostics",
			uri:         "file:///test/file.yaml",
			diagnostics: []lsp.Diagnostic{},
			expectContains: []string{
				"No issues found",
				"/test/file.yaml",
			},
		},
		{
			name: "Single error",
			uri:  "file:///test/stack.yaml",
			diagnostics: []lsp.Diagnostic{
				{
					Range: lsp.Range{
						Start: lsp.Position{Line: 10, Character: 5},
						End:   lsp.Position{Line: 10, Character: 15},
					},
					Severity: lsp.DiagnosticSeverityError,
					Message:  "Unknown property 'vpc_cidr'",
					Source:   "yaml-language-server",
				},
			},
			expectContains: []string{
				"Found 1 issue(s)",
				"ERRORS (1)",
				"Line 11",
				"Unknown property 'vpc_cidr'",
			},
		},
		{
			name: "Multiple errors and warnings",
			uri:  "file:///test/main.tf",
			diagnostics: []lsp.Diagnostic{
				{
					Range:    lsp.Range{Start: lsp.Position{Line: 5, Character: 0}},
					Severity: lsp.DiagnosticSeverityError,
					Message:  "Missing required argument",
				},
				{
					Range:    lsp.Range{Start: lsp.Position{Line: 10, Character: 2}},
					Severity: lsp.DiagnosticSeverityError,
					Message:  "Invalid CIDR format",
				},
				{
					Range:    lsp.Range{Start: lsp.Position{Line: 15, Character: 0}},
					Severity: lsp.DiagnosticSeverityWarning,
					Message:  "Deprecated argument",
				},
			},
			expectContains: []string{
				"Found 3 issue(s)",
				"ERRORS (2)",
				"WARNINGS (1)",
				"Line 6",
				"Line 11",
				"Line 16",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatter.FormatForAI(tt.uri, tt.diagnostics)

			for _, expected := range tt.expectContains {
				assert.Contains(t, result, expected,
					"Expected result to contain '%s'", expected)
			}
		})
	}
}

func TestDiagnosticFormatter_FormatDiagnostics(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	diagnostics := []lsp.Diagnostic{
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 10, Character: 5},
				End:   lsp.Position{Line: 10, Character: 15},
			},
			Severity: lsp.DiagnosticSeverityError,
			Message:  "Syntax error",
			Source:   "yaml-ls",
			Code:     "E001",
		},
		{
			Range: lsp.Range{
				Start: lsp.Position{Line: 20, Character: 0},
			},
			Severity: lsp.DiagnosticSeverityWarning,
			Message:  "Unused variable",
			Source:   "terraform-ls",
		},
	}

	uri := "file:///test/config.yaml"
	result := formatter.FormatDiagnostics(uri, diagnostics)

	// Check structure
	assert.Contains(t, result, "File: /test/config.yaml")
	assert.Contains(t, result, "Summary: 1 error(s), 1 warning(s)")
	assert.Contains(t, result, "ERRORS:")
	assert.Contains(t, result, "WARNINGS:")

	// Check error details
	assert.Contains(t, result, "Line 11:6") // LSP is 0-based, display is 1-based
	assert.Contains(t, result, "[yaml-ls]")
	assert.Contains(t, result, "(Code: E001)")
	assert.Contains(t, result, "Syntax error")

	// Check warning details
	assert.Contains(t, result, "Line 21:1")
	assert.Contains(t, result, "[terraform-ls]")
	assert.Contains(t, result, "Unused variable")
}

func TestDiagnosticFormatter_FormatCompact(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	diagnostics := []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 5, Character: 10}},
			Severity: lsp.DiagnosticSeverityError,
			Message:  "Parse error",
			Source:   "yaml-ls",
		},
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 15, Character: 0}},
			Severity: lsp.DiagnosticSeverityWarning,
			Message:  "Deprecated syntax",
		},
	}

	uri := "file:///test/stack.yaml"
	result := formatter.FormatCompact(uri, diagnostics)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	assert.Len(t, lines, 2, "Expected 2 lines of output")

	// Check compact format
	assert.Contains(t, lines[0], "/test/stack.yaml:6:11: error: Parse error")
	assert.Contains(t, lines[0], "[yaml-ls]")
	assert.Contains(t, lines[1], "/test/stack.yaml:16:1: warning: Deprecated syntax")
}

func TestDiagnosticFormatter_GetDiagnosticSummary(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	diagnosticsByURI := map[string][]lsp.Diagnostic{
		"file:///test/file1.yaml": {
			{Severity: lsp.DiagnosticSeverityError, Message: "Error 1"},
			{Severity: lsp.DiagnosticSeverityError, Message: "Error 2"},
			{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning 1"},
		},
		"file:///test/file2.tf": {
			{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning 2"},
			{Severity: lsp.DiagnosticSeverityInformation, Message: "Info 1"},
			{Severity: lsp.DiagnosticSeverityHint, Message: "Hint 1"},
		},
	}

	summary := formatter.GetDiagnosticSummary(diagnosticsByURI)

	assert.Equal(t, 2, summary.FilesWithIssues)
	assert.Equal(t, 2, summary.TotalErrors)
	assert.Equal(t, 2, summary.TotalWarnings)
	assert.Equal(t, 1, summary.TotalInfos)
	assert.Equal(t, 1, summary.TotalHints)

	// Check file-specific summaries
	assert.Equal(t, 2, summary.Files["file:///test/file1.yaml"].Errors)
	assert.Equal(t, 1, summary.Files["file:///test/file1.yaml"].Warnings)
	assert.Equal(t, 0, summary.Files["file:///test/file2.tf"].Errors)
	assert.Equal(t, 1, summary.Files["file:///test/file2.tf"].Warnings)
}

func TestDiagnosticFormatter_FormatAllDiagnostics(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	diagnosticsByURI := map[string][]lsp.Diagnostic{
		"file:///test/file1.yaml": {
			{
				Range:    lsp.Range{Start: lsp.Position{Line: 0, Character: 0}},
				Severity: lsp.DiagnosticSeverityError,
				Message:  "Error in file1",
			},
		},
		"file:///test/file2.yaml": {
			{
				Range:    lsp.Range{Start: lsp.Position{Line: 5, Character: 0}},
				Severity: lsp.DiagnosticSeverityWarning,
				Message:  "Warning in file2",
			},
		},
	}

	result := formatter.FormatAllDiagnostics(diagnosticsByURI)

	assert.Contains(t, result, "DIAGNOSTICS SUMMARY")
	assert.Contains(t, result, "Files with issues: 2")
	assert.Contains(t, result, "Total: 1 error(s), 1 warning(s)")
	assert.Contains(t, result, "file1.yaml")
	assert.Contains(t, result, "file2.yaml")
	assert.Contains(t, result, "Error in file1")
	assert.Contains(t, result, "Warning in file2")
}

func TestDiagnosticFormatter_FilterBySeverity(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	diagnostics := []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityError, Message: "Error 1"},
		{Severity: lsp.DiagnosticSeverityError, Message: "Error 2"},
		{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning 1"},
		{Severity: lsp.DiagnosticSeverityInformation, Message: "Info 1"},
		{Severity: lsp.DiagnosticSeverityHint, Message: "Hint 1"},
	}

	errors := formatter.filterBySeverity(diagnostics, lsp.DiagnosticSeverityError)
	assert.Len(t, errors, 2)
	assert.Equal(t, "Error 1", errors[0].Message)
	assert.Equal(t, "Error 2", errors[1].Message)

	warnings := formatter.filterBySeverity(diagnostics, lsp.DiagnosticSeverityWarning)
	assert.Len(t, warnings, 1)
	assert.Equal(t, "Warning 1", warnings[0].Message)

	infos := formatter.filterBySeverity(diagnostics, lsp.DiagnosticSeverityInformation)
	assert.Len(t, infos, 1)

	hints := formatter.filterBySeverity(diagnostics, lsp.DiagnosticSeverityHint)
	assert.Len(t, hints, 1)
}

func TestHasErrors(t *testing.T) {
	noDiagnostics := []lsp.Diagnostic{}
	onlyErrors := []lsp.Diagnostic{{Severity: lsp.DiagnosticSeverityError, Message: "Error"}}
	onlyWarnings := []lsp.Diagnostic{{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning"}}
	mixed := []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning"},
		{Severity: lsp.DiagnosticSeverityError, Message: "Error"},
	}

	assert.False(t, HasErrors(noDiagnostics), "No diagnostics should return false")
	assert.True(t, HasErrors(onlyErrors), "Only errors should return true")
	assert.False(t, HasErrors(onlyWarnings), "Only warnings should return false")
	assert.True(t, HasErrors(mixed), "Mixed with errors should return true")
}

func TestHasWarnings(t *testing.T) {
	noDiagnostics := []lsp.Diagnostic{}
	onlyWarnings := []lsp.Diagnostic{{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning"}}
	onlyErrors := []lsp.Diagnostic{{Severity: lsp.DiagnosticSeverityError, Message: "Error"}}
	mixed := []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityError, Message: "Error"},
		{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning"},
	}

	assert.False(t, HasWarnings(noDiagnostics), "No diagnostics should return false")
	assert.True(t, HasWarnings(onlyWarnings), "Only warnings should return true")
	assert.False(t, HasWarnings(onlyErrors), "Only errors should return false")
	assert.True(t, HasWarnings(mixed), "Mixed with warnings should return true")
}

func TestCountBySeverity(t *testing.T) {
	diagnostics := []lsp.Diagnostic{
		{Severity: lsp.DiagnosticSeverityError, Message: "Error 1"},
		{Severity: lsp.DiagnosticSeverityError, Message: "Error 2"},
		{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning 1"},
		{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning 2"},
		{Severity: lsp.DiagnosticSeverityWarning, Message: "Warning 3"},
		{Severity: lsp.DiagnosticSeverityInformation, Message: "Info 1"},
		{Severity: lsp.DiagnosticSeverityHint, Message: "Hint 1"},
	}

	counts := CountBySeverity(diagnostics)

	assert.Equal(t, 2, counts[lsp.DiagnosticSeverityError])
	assert.Equal(t, 3, counts[lsp.DiagnosticSeverityWarning])
	assert.Equal(t, 1, counts[lsp.DiagnosticSeverityInformation])
	assert.Equal(t, 1, counts[lsp.DiagnosticSeverityHint])
}

func TestDiagnosticFormatter_WithRelatedInformation(t *testing.T) {
	formatter := NewDiagnosticFormatter()
	formatter.ShowRelated = true

	diagnostics := []lsp.Diagnostic{
		{
			Range:    lsp.Range{Start: lsp.Position{Line: 10, Character: 5}},
			Severity: lsp.DiagnosticSeverityError,
			Message:  "Variable 'foo' is undefined",
			Source:   "typescript",
			RelatedInformation: []lsp.DiagnosticInfo{
				{
					Location: lsp.Location{
						URI:   "file:///test/other.ts",
						Range: lsp.Range{Start: lsp.Position{Line: 5, Character: 10}},
					},
					Message: "Variable 'foo' was declared here",
				},
			},
		},
	}

	result := formatter.FormatDiagnostics("file:///test/main.ts", diagnostics)

	assert.Contains(t, result, "Variable 'foo' is undefined")
	assert.Contains(t, result, "Related:")
	assert.Contains(t, result, "other.ts:6:11")
	assert.Contains(t, result, "Variable 'foo' was declared here")
}

func TestDiagnosticFormatter_SeverityString(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	tests := []struct {
		severity lsp.DiagnosticSeverity
		expected string
	}{
		{lsp.DiagnosticSeverityError, "error"},
		{lsp.DiagnosticSeverityWarning, "warning"},
		{lsp.DiagnosticSeverityInformation, "info"},
		{lsp.DiagnosticSeverityHint, "hint"},
		{lsp.DiagnosticSeverity(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatter.severityString(tt.severity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDiagnosticFormatter_FormatURI(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	tests := []struct {
		uri      string
		expected string
	}{
		{"file:///path/to/file.yaml", "/path/to/file.yaml"},
		{"/path/to/file.yaml", "/path/to/file.yaml"},
		{"file:///C:/Users/test/file.yaml", "/C:/Users/test/file.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.uri, func(t *testing.T) {
			result := formatter.formatURI(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}
