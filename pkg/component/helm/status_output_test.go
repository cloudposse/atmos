package helm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2: cluster-mutating commands (apply, delete) must print a status line instead of
// succeeding silently. emitOperationStatus writes a human-readable summary via the status
// writer for apply/delete on success only (never on error, never for template/diff which
// produce their own output).
func TestEmitOperationStatus(t *testing.T) {
	summary := map[string]any{
		"release_name": "echo-server-public",
		"namespace":    "echo-server-public",
		"chart":        "ealenn/echo-server",
	}

	tests := []struct {
		name      string
		operation Operation
		opErr     error
		wantWrite bool
	}{
		{"apply success writes", OperationApply, nil, true},
		{"delete success writes", OperationDelete, nil, true},
		{"apply error is silent", OperationApply, assert.AnError, false},
		{"template is silent", OperationTemplate, nil, false},
		{"diff is silent", OperationDiff, nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var captured string
			var called bool
			orig := writeStatusLine
			writeStatusLine = func(s string) { called = true; captured = s }
			t.Cleanup(func() { writeStatusLine = orig })

			emitOperationStatus(tc.operation, summary, tc.opErr)

			require.Equal(t, tc.wantWrite, called)
			if tc.wantWrite {
				require.Contains(t, captured, "echo-server-public")
			}
		})
	}
}

// formatOperationStatus names the release and namespace for apply/delete and is empty otherwise.
func TestFormatOperationStatus(t *testing.T) {
	summary := map[string]any{
		"release_name": "echo-server",
		"namespace":    "echo-server",
		"chart":        "./",
	}

	apply := formatOperationStatus(OperationApply, summary)
	require.Contains(t, apply, "echo-server")
	require.Contains(t, strings.ToLower(apply), "namespace")
	// The release/namespace are markdown-backticked so ui.Success renders them as inline code.
	require.Contains(t, apply, "`echo-server`")

	del := formatOperationStatus(OperationDelete, summary)
	require.Contains(t, del, "`echo-server`")

	require.Empty(t, formatOperationStatus(OperationTemplate, summary))
	require.Empty(t, formatOperationStatus(OperationDiff, summary))
}

// The chart segment must use single, balanced parens around a single markdown code
// span. A doubled "((chart `%s`))" breaks Glamour's inline-code detection, so the
// backticks leak into the rendered terminal output as literal characters instead of
// styling the path as code (confirmed by rendering both forms through the real
// ui.Formatter -- the doubled form renders with visible backticks, the single form
// renders the path in the same style as the backticked release/namespace above it).
func TestFormatOperationStatus_ChartUsesSingleBalancedParens(t *testing.T) {
	summary := map[string]any{
		"release_name": "echo-server",
		"namespace":    "echo-server",
		"chart":        "./chart",
	}

	apply := formatOperationStatus(OperationApply, summary)
	require.Contains(t, apply, "(chart `./chart`)")
	assert.NotContains(t, apply, "((")
	assert.NotContains(t, apply, "))")
}

// displayPath renders an absolute chart path relative to the current working
// directory for terminal display, since local charts are always resolved to an
// absolute path internally (see resolveLocalChart) regardless of invoking directory.
func TestDisplayPath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	wd, err := os.Getwd()
	require.NoError(t, err)

	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty path passes through unchanged", "", ""},
		{"already-relative path passes through unchanged", "./chart", "./chart"},
		{"absolute path under cwd becomes relative", filepath.Join(wd, "components", "helm", "demo"), filepath.Join("components", "helm", "demo")},
		{"absolute path equal to cwd becomes a dot", wd, "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, displayPath(tt.path))
		})
	}
}
