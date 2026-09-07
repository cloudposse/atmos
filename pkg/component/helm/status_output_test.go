package helm

import (
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
