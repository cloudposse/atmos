package lsp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiagnosticFormatter_FormatForAI(t *testing.T) {
	formatter := NewDiagnosticFormatter()

	tests := []struct {
		name           string
		uri            string
		diagnostics    []Diagnostic
		expectContains []string
	}{
		{
			name:        "No diagnostics",
			uri:         "file:///test/file.yaml",
			diagnostics: []Diagnostic{},
			expectContains: []string{
				"No issues found",
				"/test/file.yaml",
			},
		},
		{
			name: "Single error",
			uri:  "file:///test/stack.yaml",
			diagnostics: []Diagnostic{
				{
					Range: Range{
						Start: Position{Line: 10, Character: 5},
						End:   Position{Line: 10, Character: 15},
					},
					Severity: DiagnosticSeverityError,
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
			diagnostics: []Diagnostic{
				{
					Range:    Range{Start: Position{Line: 5, Character: 0}},
					Severity: DiagnosticSeverityError,
					Message:  "Missing required argument",
				},
				{
					Range:    Range{Start: Position{Line: 10, Character: 2}},
					Severity: DiagnosticSeverityError,
					Message:  "Invalid CIDR format",
				},
				{
					Range:    Range{Start: Position{Line: 15, Character: 0}},
					Severity: DiagnosticSeverityWarning,
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

	diagnostics := []Diagnostic{
		{
			Range: Range{
				Start: Position{Line: 10, Character: 5},
				End:   Position{Line: 10, Character: 15},
			},
			Severity: DiagnosticSeverityError,
			Message:  "Syntax error",
			Source:   "yaml-ls",
			Code:     "E001",
		},
		{
			Range: Range{
				Start: Position{Line: 20, Character: 0},
			},
			Severity: DiagnosticSeverityWarning,
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

	diagnostics := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 5, Character: 10}},
			Severity: DiagnosticSeverityError,
			Message:  "Parse error",
			Source:   "yaml-ls",
		},
		{
			Range:    Range{Start: Position{Line: 15, Character: 0}},
			Severity: DiagnosticSeverityWarning,
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

	diagnosticsByURI := map[string][]Diagnostic{
		"file:///test/file1.yaml": {
			{Severity: DiagnosticSeverityError, Message: "Error 1"},
			{Severity: DiagnosticSeverityError, Message: "Error 2"},
			{Severity: DiagnosticSeverityWarning, Message: "Warning 1"},
		},
		"file:///test/file2.tf": {
			{Severity: DiagnosticSeverityWarning, Message: "Warning 2"},
			{Severity: DiagnosticSeverityInformation, Message: "Info 1"},
			{Severity: DiagnosticSeverityHint, Message: "Hint 1"},
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

	diagnosticsByURI := map[string][]Diagnostic{
		"file:///test/file1.yaml": {
			{
				Range:    Range{Start: Position{Line: 0, Character: 0}},
				Severity: DiagnosticSeverityError,
				Message:  "Error in file1",
			},
		},
		"file:///test/file2.yaml": {
			{
				Range:    Range{Start: Position{Line: 5, Character: 0}},
				Severity: DiagnosticSeverityWarning,
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

	diagnostics := []Diagnostic{
		{Severity: DiagnosticSeverityError, Message: "Error 1"},
		{Severity: DiagnosticSeverityError, Message: "Error 2"},
		{Severity: DiagnosticSeverityWarning, Message: "Warning 1"},
		{Severity: DiagnosticSeverityInformation, Message: "Info 1"},
		{Severity: DiagnosticSeverityHint, Message: "Hint 1"},
	}

	errors := formatter.filterBySeverity(diagnostics, DiagnosticSeverityError)
	assert.Len(t, errors, 2)
	assert.Equal(t, "Error 1", errors[0].Message)
	assert.Equal(t, "Error 2", errors[1].Message)

	warnings := formatter.filterBySeverity(diagnostics, DiagnosticSeverityWarning)
	assert.Len(t, warnings, 1)
	assert.Equal(t, "Warning 1", warnings[0].Message)

	infos := formatter.filterBySeverity(diagnostics, DiagnosticSeverityInformation)
	assert.Len(t, infos, 1)

	hints := formatter.filterBySeverity(diagnostics, DiagnosticSeverityHint)
	assert.Len(t, hints, 1)
}

func TestHasErrors(t *testing.T) {
	tests := []struct {
		name        string
		diagnostics []Diagnostic
		expected    bool
	}{
		{
			name:        "No diagnostics",
			diagnostics: []Diagnostic{},
			expected:    false,
		},
		{
			name: "Has errors",
			diagnostics: []Diagnostic{
				{Severity: DiagnosticSeverityError, Message: "Error"},
			},
			expected: true,
		},
		{
			name: "Only warnings",
			diagnostics: []Diagnostic{
				{Severity: DiagnosticSeverityWarning, Message: "Warning"},
			},
			expected: false,
		},
		{
			name: "Mixed with errors",
			diagnostics: []Diagnostic{
				{Severity: DiagnosticSeverityWarning, Message: "Warning"},
				{Severity: DiagnosticSeverityError, Message: "Error"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasErrors(tt.diagnostics)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasWarnings(t *testing.T) {
	tests := []struct {
		name        string
		diagnostics []Diagnostic
		expected    bool
	}{
		{
			name:        "No diagnostics",
			diagnostics: []Diagnostic{},
			expected:    false,
		},
		{
			name: "Has warnings",
			diagnostics: []Diagnostic{
				{Severity: DiagnosticSeverityWarning, Message: "Warning"},
			},
			expected: true,
		},
		{
			name: "Only errors",
			diagnostics: []Diagnostic{
				{Severity: DiagnosticSeverityError, Message: "Error"},
			},
			expected: false,
		},
		{
			name: "Mixed with warnings",
			diagnostics: []Diagnostic{
				{Severity: DiagnosticSeverityError, Message: "Error"},
				{Severity: DiagnosticSeverityWarning, Message: "Warning"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasWarnings(tt.diagnostics)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCountBySeverity(t *testing.T) {
	diagnostics := []Diagnostic{
		{Severity: DiagnosticSeverityError, Message: "Error 1"},
		{Severity: DiagnosticSeverityError, Message: "Error 2"},
		{Severity: DiagnosticSeverityWarning, Message: "Warning 1"},
		{Severity: DiagnosticSeverityWarning, Message: "Warning 2"},
		{Severity: DiagnosticSeverityWarning, Message: "Warning 3"},
		{Severity: DiagnosticSeverityInformation, Message: "Info 1"},
		{Severity: DiagnosticSeverityHint, Message: "Hint 1"},
	}

	counts := CountBySeverity(diagnostics)

	assert.Equal(t, 2, counts[DiagnosticSeverityError])
	assert.Equal(t, 3, counts[DiagnosticSeverityWarning])
	assert.Equal(t, 1, counts[DiagnosticSeverityInformation])
	assert.Equal(t, 1, counts[DiagnosticSeverityHint])
}

func TestDiagnosticFormatter_WithRelatedInformation(t *testing.T) {
	formatter := NewDiagnosticFormatter()
	formatter.ShowRelated = true

	diagnostics := []Diagnostic{
		{
			Range:    Range{Start: Position{Line: 10, Character: 5}},
			Severity: DiagnosticSeverityError,
			Message:  "Variable 'foo' is undefined",
			Source:   "typescript",
			RelatedInformation: []DiagnosticInfo{
				{
					Location: Location{
						URI:   "file:///test/other.ts",
						Range: Range{Start: Position{Line: 5, Character: 10}},
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
		severity DiagnosticSeverity
		expected string
	}{
		{DiagnosticSeverityError, "error"},
		{DiagnosticSeverityWarning, "warning"},
		{DiagnosticSeverityInformation, "info"},
		{DiagnosticSeverityHint, "hint"},
		{DiagnosticSeverity(99), "unknown"},
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
