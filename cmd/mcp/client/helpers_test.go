package client

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: fails build if referenced schema fields are renamed.
var _ = schema.MCPServerConfig{
	Command:  "",
	Args:     nil,
	Env:      nil,
	Identity: "",
}

// TestFirstSentence tests the firstSentence helper that extracts the first sentence
// from a description string, collapsing whitespace and handling markdown boundaries.
func TestFirstSentence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal sentence with period and space",
			input:    "First sentence. Second sentence.",
			expected: "First sentence.",
		},
		{
			name:     "no period in text",
			input:    "Just a phrase",
			expected: "Just a phrase",
		},
		{
			name:     "markdown header boundary",
			input:    "Some text ## Header",
			expected: "Some text.",
		},
		{
			name:     "multi-line input with whitespace collapsed",
			input:    "Line one\nLine two. Line three.",
			expected: "Line one Line two.",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single period at end without trailing space",
			input:    "Only sentence.",
			expected: "Only sentence.",
		},
		{
			name:     "period at end followed by space",
			input:    "Only sentence. ",
			expected: "Only sentence.",
		},
		{
			name:     "tabs and multiple spaces collapsed",
			input:    "Word1\t\tWord2   Word3. Rest.",
			expected: "Word1 Word2 Word3.",
		},
		{
			name:     "markdown header without preceding text",
			input:    "## Header only",
			expected: "## Header only",
		},
		// Cases below are regression coverage for issue #6 in
		// docs/fixes/2026-05-15-mcp-review-fixes.md (firstSentence
		// hardening). They cover the previously-unrecognized terminators
		// (`!`, `?`), the URL/version false-split case, and the no-
		// terminator length bound.
		{
			name:     "exclamation+space ends a sentence",
			input:    "First sentence! Second sentence.",
			expected: "First sentence!",
		},
		{
			name:     "question+space ends a sentence",
			input:    "Is this a sentence? Yes it is.",
			expected: "Is this a sentence?",
		},
		{
			name: "earliest terminator wins among period/exclamation/question",
			// "?" appears before "." in the input — it should win.
			input:    "Really? Yes. Indeed.",
			expected: "Really?",
		},
		{
			name: "version string before real period does NOT cause false split",
			// "v1.0" has a period but no following space, so the old
			// `strings.Index(". ")` already handled it. This guards
			// the contract.
			input:    "Supports v1.0 and v2.0. End.",
			expected: "Supports v1.0 and v2.0.",
		},
		{
			name: "no terminator, long input is truncated to firstSentenceMaxLen",
			// 81 chars, no terminator → truncated to 79 + ellipsis (80 runes total).
			input:    strings.Repeat("a", 81),
			expected: strings.Repeat("a", 79) + "…",
		},
		{
			name: "no terminator, exactly maxLen input is returned as-is",
			// At the boundary the input must not be truncated.
			input:    strings.Repeat("a", firstSentenceMaxLen),
			expected: strings.Repeat("a", firstSentenceMaxLen),
		},
		{
			name:     "no terminator, short input is returned as-is",
			input:    "Just a short phrase",
			expected: "Just a short phrase",
		},
		{
			name: "markdown header is treated as a terminator but ceiling still applies",
			// The markdown-header break gives us a sentence boundary, but
			// the firstSentenceMaxLen ceiling is now universal — including
			// for terminated sentences and markdown breaks. So a 111-char
			// candidate gets truncated to firstSentenceMaxLen-1 + "…"
			// (79 + 1 = 80 runes total).
			input: "Some text " + strings.Repeat("x", 100) + " ## Header",
			expected: "Some text " + strings.Repeat("x",
				firstSentenceMaxLen-1-len("Some text ")) + "…",
		},
		{
			name: "long sentence ending in period is truncated to firstSentenceMaxLen",
			// Doc/impl mismatch guard: the firstSentenceMaxLen ceiling
			// must apply even when a terminator is present, otherwise a
			// 150-rune sentence ending in `. ` would leak into the
			// `atmos mcp tools` table (the original review nitpick).
			input:    strings.Repeat("a", 100) + ". Rest.",
			expected: strings.Repeat("a", firstSentenceMaxLen-1) + "…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := firstSentence(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Note: the previous TestBuildMCPJSONEntry exercised a cmd-local
// buildMCPJSONEntry that was removed when export.go was refactored to
// delegate to mcpclient.GenerateMCPConfig. The same contract is now
// covered by:
//
//   - pkg/mcp/client/mcpconfig_test.go (package-level unit tests for
//     BuildMCPJSONEntry and GenerateMCPConfig).
//   - cmd/mcp/client/export_test.go::TestExport_DelegatesToPackageGenerator
//     (cmd-level regression guard for identity wrapping + toolchain
//     PATH injection in the export path).

func TestFormatStatusRow(t *testing.T) {
	tests := []struct {
		name        string
		serverName  string
		description string
		result      *mcpclient.TestResult
		expected    []string
	}{
		{
			name:        "running server",
			serverName:  "aws-docs",
			description: "AWS Documentation",
			result:      &mcpclient.TestResult{ServerStarted: true, PingOK: true, ToolCount: 4},
			expected:    []string{"aws-docs", "running", "4", "AWS Documentation"},
		},
		{
			name:        "degraded server (started but ping failed)",
			serverName:  "aws-iam",
			description: "AWS IAM",
			result:      &mcpclient.TestResult{ServerStarted: true, PingOK: false, ToolCount: 0},
			expected:    []string{"aws-iam", "degraded", "0", "AWS IAM"},
		},
		{
			name:        "error server (failed to start)",
			serverName:  "aws-api",
			description: "AWS API",
			result:      &mcpclient.TestResult{ServerStarted: false, Error: errors.New("connection refused")},
			expected:    []string{"aws-api", "error", "0", "AWS API (connection refused)"},
		},
		{
			name:        "error with long message truncated",
			serverName:  "broken",
			description: "Broken server",
			result:      &mcpclient.TestResult{ServerStarted: false, Error: errors.New("this is a very long error message that exceeds the maximum display length for table cells")},
			expected:    []string{"broken", "error", "0", "Broken server (this is a very long error message that exceeds ...)"},
		},
		{
			name:        "zero tools",
			serverName:  "empty",
			description: "Empty",
			result:      &mcpclient.TestResult{ServerStarted: true, PingOK: true, ToolCount: 0},
			expected:    []string{"empty", "running", "0", "Empty"},
		},
		{
			name:        "error with empty description",
			serverName:  "broken",
			description: "",
			result:      &mcpclient.TestResult{ServerStarted: false, Error: errors.New("not found")},
			expected:    []string{"broken", "error", "0", "(not found)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := formatStatusRow(tt.serverName, tt.description, tt.result)
			assert.Equal(t, tt.expected, row)
		})
	}
}
