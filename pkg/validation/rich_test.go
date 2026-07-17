package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRichRendersSourceContextAndRange(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "example.yaml"), []byte("one: 1\ntwo: 2\nthree: 3\nfour: 4\n"), 0o600))

	output := Rich(Report{Diagnostics: []Diagnostic{{
		Source: "schema", RuleID: "type", Severity: SeverityError,
		Message: "expected string", File: "example.yaml", Line: 2, Column: 2, EndLine: 3,
	}}}, RichOptions{Root: root, Width: 80})

	assert.Contains(t, output, "[schema] example.yaml:2:2")
	assert.Contains(t, output, "error: expected string [type]")
	assert.Contains(t, output, "1 | one: 1")
	assert.Contains(t, output, "2 | two: 2")
	assert.Contains(t, output, "3 | three: 3")
	assert.Contains(t, output, "4 | four: 4")
	assert.Equal(t, 2, strings.Count(output, "^"))
	assert.NotContains(t, output, "\x1b[")
}

func TestRichFallsBackWhenLocationCannotBeRead(t *testing.T) {
	output := Rich(Report{Diagnostics: []Diagnostic{{
		Source: "component", RuleID: "component", Severity: SeverityError,
		Message: "policy rejected component", File: "missing.yaml",
	}}}, RichOptions{Root: t.TempDir()})
	assert.Contains(t, output, "[component] missing.yaml")
	assert.Contains(t, output, "policy rejected component")
	assert.NotContains(t, output, " | ")
}

func TestRichSortsDiagnosticsAndClipsLongLines(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.yaml"), []byte("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n"), 0o600))
	output := Rich(Report{Diagnostics: []Diagnostic{
		{Source: "test", RuleID: "z", Severity: SeverityError, Message: "z", File: "z.yaml"},
		{Source: "test", RuleID: "a", Severity: SeverityError, Message: "a", File: "a.yaml", Line: 1, Column: 35},
	}}, RichOptions{Root: root, Width: 30})
	assert.Less(t, strings.Index(output, "a.yaml"), strings.Index(output, "z.yaml"))
	assert.Contains(t, output, "…")
}

func TestRichHelpers(t *testing.T) {
	root := t.TempDir()
	assert.Equal(t, "file.yaml", richPath("", "file.yaml"))
	assert.Equal(t, filepath.Join(root, "file.yaml"), richPath(root, "file.yaml"))
	assert.Equal(t, filepath.Join(root, "file.yaml"), richPath(root, filepath.Join(root, "file.yaml")))

	diagnostic := Diagnostic{Line: 2, Column: 4}
	assert.Equal(t, 4, diagnosticColumn(diagnostic, "  value", 2))
	assert.Equal(t, 4, diagnosticColumn(diagnostic, "\t  value", 1))
	assert.Equal(t, 1, firstContentColumn(" \t "))

	short, shortOffset := richClip("short", 1, 1)
	assert.Equal(t, "short", short)
	assert.Zero(t, shortOffset)
	clipped, offset := richClip("abcdefghijklmnopqrstuvwxyz", 20, 25)
	assert.Contains(t, clipped, "…")
	assert.Positive(t, offset)

	assert.Equal(t, "yellow", severityColor(SeverityWarning))
	assert.Equal(t, "blue", severityColor(SeverityNotice))
	assert.Equal(t, "red", severityColor(Severity("other")))
	assert.Equal(t, "value", richStyle("value", "bold", false))
	assert.Equal(t, "value", richStyle("value", "unknown", true))
	assert.Equal(t, "\x1b[1mvalue\x1b[0m", richStyle("value", "bold", true))
}

func TestRichHandlesDefaultHeadersAndOutOfRangeLocations(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "example.yaml"), []byte("first\nsecond\n"), 0o600))
	output := Rich(Report{Diagnostics: []Diagnostic{
		{Message: "missing fields"},
		{Severity: SeverityWarning, Message: "past end", File: "example.yaml", Line: 3},
		{Severity: SeverityNotice, Message: "range", File: "example.yaml", Line: 2, EndLine: 1, EndColumn: 9},
	}}, RichOptions{Root: root, Width: 0})

	assert.Contains(t, output, "[validation] (unknown location)")
	assert.Contains(t, output, "error: missing fields")
	assert.Contains(t, output, "warning: past end")
	assert.Contains(t, output, "notice: range")
	assert.Contains(t, output, "2 | second")
}
