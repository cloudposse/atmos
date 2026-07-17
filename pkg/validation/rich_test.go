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
