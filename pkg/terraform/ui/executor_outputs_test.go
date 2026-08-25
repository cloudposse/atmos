package ui

import (
	"bytes"
	"fmt"
	stdio "io"
	"math"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// outputsTestStreams is a minimal io.Streams implementation that lets tests capture what
// gets written to the UI (stderr) channel, mirroring the pattern used by
// pkg/ci/providers/github's tests for the same io.Context/ui.InitFormatter wiring.
type outputsTestStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (s *outputsTestStreams) Input() stdio.Reader     { return s.stdin }
func (s *outputsTestStreams) Output() stdio.Writer    { return s.stdout }
func (s *outputsTestStreams) Error() stdio.Writer     { return s.stderr }
func (s *outputsTestStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *outputsTestStreams) RawError() stdio.Writer  { return s.stderr }

// captureUIOutput initializes the global UI formatter with an in-memory stderr buffer so
// tests can assert on what displayOutputs/renderOutputsTable actually write, and restores
// global UI state afterward so other tests in the package aren't affected by the override.
func captureUIOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	stderr := &bytes.Buffer{}
	streams := &outputsTestStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: stderr}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	t.Cleanup(ui.Reset)
	return stderr
}

// TestOutputRows_SortsAndMasksSensitive verifies outputRows both sorts by output name and
// masks sensitive values, since callers rely on this for a stable, safe-to-print table.
func TestOutputRows_SortsAndMasksSensitive(t *testing.T) {
	outputs := map[string]OutputValue{
		"zebra":  {Value: "z-value"},
		"apple":  {Value: float64(42)},
		"secret": {Value: "shhh", Sensitive: true},
	}

	rows := outputRows(outputs)

	require.Len(t, rows, 3)
	assert.Equal(t, []string{"apple", "42"}, rows[0])
	assert.Equal(t, []string{"secret", "<sensitive>"}, rows[1])
	assert.Equal(t, []string{"zebra", "z-value"}, rows[2])
}

// TestOutputRows_Empty verifies an empty outputs map produces no rows.
func TestOutputRows_Empty(t *testing.T) {
	rows := outputRows(map[string]OutputValue{})
	assert.Empty(t, rows)
}

// TestFormatOutputValue_MarshalError verifies the default-case fallback: a value that cannot
// be JSON-marshaled (Inf) must still render via %v instead of silently disappearing.
func TestFormatOutputValue_MarshalError(t *testing.T) {
	value := map[string]interface{}{"rate": math.Inf(1)}

	result := formatOutputValue(value)

	assert.Equal(t, fmt.Sprintf("%v", value), result)
}

// TestDisplayOutputs_NoOutputs verifies nothing is written when the tracker never received an
// outputs message (e.g. a plan-only run).
func TestDisplayOutputs_NoOutputs(t *testing.T) {
	stderr := captureUIOutput(t)
	tracker := NewResourceTracker()

	displayOutputs(tracker)

	assert.Empty(t, stderr.String())
}

// TestDisplayOutputs_EmptyOutputsMap verifies nothing is written when an outputs message
// arrived but contained zero outputs.
func TestDisplayOutputs_EmptyOutputsMap(t *testing.T) {
	stderr := captureUIOutput(t)
	tracker := NewResourceTracker()
	tracker.HandleMessage(&OutputsMessage{Outputs: map[string]OutputValue{}})

	displayOutputs(tracker)

	assert.Empty(t, stderr.String())
}

// TestDisplayOutputs_WritesTable verifies real output values render in the table and that a
// sensitive value's actual content never reaches the UI channel.
func TestDisplayOutputs_WritesTable(t *testing.T) {
	stderr := captureUIOutput(t)
	tracker := NewResourceTracker()
	tracker.HandleMessage(&OutputsMessage{
		Outputs: map[string]OutputValue{
			"vpc_id":  {Value: "vpc-12345"},
			"api_key": {Value: "topsecret", Sensitive: true},
		},
	})

	displayOutputs(tracker)

	output := stderr.String()
	assert.Contains(t, output, "Output")
	assert.Contains(t, output, "Value")
	assert.Contains(t, output, "vpc_id")
	assert.Contains(t, output, "vpc-12345")
	assert.Contains(t, output, "api_key")
	assert.Contains(t, output, "<sensitive>")
	assert.NotContains(t, output, "topsecret", "sensitive value must never be printed")
}

// TestFetchAndDisplayOutputs_CommandNotFound verifies a command that can't be started (e.g.
// terraform missing from PATH) is handled silently rather than panicking or printing an error -
// outputs simply may not be available yet.
func TestFetchAndDisplayOutputs_CommandNotFound(t *testing.T) {
	stderr := captureUIOutput(t)

	fetchAndDisplayOutputs("atmos-nonexistent-binary-xyz", t.TempDir(), nil)

	assert.Empty(t, stderr.String())
}

// TestCreateOutputsTable_RendersHeadersAndRows verifies the assembled table string contains
// the headers and every row's content.
func TestCreateOutputsTable_RendersHeadersAndRows(t *testing.T) {
	rows := [][]string{
		{"region", "us-east-1"},
		{"enabled", "true"},
	}

	result := createOutputsTable([]string{"Output", "Value"}, rows)

	assert.Contains(t, result, "Output")
	assert.Contains(t, result, "Value")
	assert.Contains(t, result, "region")
	assert.Contains(t, result, "us-east-1")
	assert.Contains(t, result, "enabled")
	assert.Contains(t, result, "true")
}

// TestCreateOutputsStyleFunc_NilStyles verifies the nil-styles guard returns the bare base
// style rather than dereferencing a nil StyleSet.
func TestCreateOutputsStyleFunc_NilStyles(t *testing.T) {
	styleFunc := createOutputsStyleFunc([][]string{{"a", "b"}}, nil)

	result := styleFunc(0, 1)

	assert.Equal(t, lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1), result)
}

// TestCreateOutputsStyleFunc_RowRoles covers the header row, the name column, the value
// column (semantic styling), and the out-of-bounds fallback.
func TestCreateOutputsStyleFunc_RowRoles(t *testing.T) {
	styles := theme.GetCurrentStyles()
	rows := [][]string{{"enabled", "true"}}
	styleFunc := createOutputsStyleFunc(rows, styles)

	headerStyle := styleFunc(-1, 0)
	assert.Equal(t, styles.TableHeader.GetForeground(), headerStyle.GetForeground())

	nameColStyle := styleFunc(0, 0)
	assert.Equal(t, styles.TableRow.GetForeground(), nameColStyle.GetForeground())

	valueColStyle := styleFunc(0, 1)
	assert.Equal(t, styles.Success.GetForeground(), valueColStyle.GetForeground(), "'true' is boolean content and should use the success color")

	oobStyle := styleFunc(5, 5)
	assert.Equal(t, styles.TableRow.GetForeground(), oobStyle.GetForeground(), "out-of-bounds row/col must fall back to the default row style")
}

// TestGetOutputCellStyle covers every detected content type's semantic styling branch.
func TestGetOutputCellStyle(t *testing.T) {
	styles := theme.GetCurrentStyles()
	base := lipgloss.NewStyle()

	tests := []struct {
		name     string
		value    string
		expected lipgloss.TerminalColor
	}{
		{"boolean true uses success color", "true", styles.Success.GetForeground()},
		{"boolean false uses error color", "false", styles.Error.GetForeground()},
		{"number uses info color", "42", styles.Info.GetForeground()},
		{"sensitive marker uses muted color", "<sensitive>", styles.Muted.GetForeground()},
		{"null uses muted color", "null", styles.Muted.GetForeground()},
		{"default text uses the row style", "hello", styles.TableRow.GetForeground()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getOutputCellStyle(tt.value, base, styles)
			assert.Equal(t, tt.expected, result.GetForeground())
		})
	}
}
