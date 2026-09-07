package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/cloudposse/atmos/pkg/logger"
)

// TestLogDiagnostics_NoDiagnostics verifies that a model with no diagnostics logs nothing,
// so callers don't see spurious empty log lines after every run.
func TestLogDiagnostics_NoDiagnostics(t *testing.T) {
	origLevel := log.GetLevel()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(origLevel)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.InfoLevel)

	m := NewModel("comp", "stack", "plan", strings.NewReader(""))
	m.LogDiagnostics()

	assert.Empty(t, buf.String())
}

// TestLogDiagnostics_RoutesBySeverity is a regression-style test for logDiagnostic's
// severity-to-log-level routing: "error" must go to log.Error, "warning" to log.Warn, and
// anything else (including the default terraform "" severity) to log.Info. Getting this
// wrong would either hide real errors at a low log level or spam users with warnings logged
// as errors.
func TestLogDiagnostics_RoutesBySeverity(t *testing.T) {
	tests := []struct {
		name         string
		severity     string
		summary      string
		expectPrefix string
	}{
		{"error severity logs at error level", "error", "Resource creation failed", "ERRO"},
		{"warning severity logs at warn level", "warning", "Deprecated argument used", "WARN"},
		{"unknown severity defaults to info level", "notice", "Informational notice", "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLevel := log.GetLevel()
			t.Cleanup(func() {
				log.SetOutput(os.Stderr)
				log.SetLevel(origLevel)
			})

			var buf bytes.Buffer
			log.SetOutput(&buf)
			log.SetLevel(log.InfoLevel)

			m := NewModel("comp", "stack", "plan", strings.NewReader(""))
			m.GetTracker().HandleMessage(&DiagnosticMessage{
				Diagnostic: Diagnostic{
					Severity: tt.severity,
					Summary:  tt.summary,
				},
			})

			m.LogDiagnostics()

			out := buf.String()
			assert.Contains(t, out, tt.expectPrefix)
			assert.Contains(t, out, tt.summary)
		})
	}
}

// TestLogDiagnostics_MultipleDiagnostics verifies every tracked diagnostic is logged, not
// just the first or last one.
func TestLogDiagnostics_MultipleDiagnostics(t *testing.T) {
	origLevel := log.GetLevel()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(origLevel)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.InfoLevel)

	m := NewModel("comp", "stack", "plan", strings.NewReader(""))
	m.GetTracker().HandleMessage(&DiagnosticMessage{
		Diagnostic: Diagnostic{Severity: "warning", Summary: "first warning"},
	})
	m.GetTracker().HandleMessage(&DiagnosticMessage{
		Diagnostic: Diagnostic{Severity: "error", Summary: "second problem"},
	})

	m.LogDiagnostics()

	out := buf.String()
	assert.Contains(t, out, "first warning")
	assert.Contains(t, out, "second problem")
}

// TestLogDiagnostic_IncludesAddress verifies the resource address is attached as a
// structured keyval so users can tell which resource a diagnostic came from.
func TestLogDiagnostic_IncludesAddress(t *testing.T) {
	origLevel := log.GetLevel()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(origLevel)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.InfoLevel)

	m := NewModel("comp", "stack", "plan", strings.NewReader(""))
	m.GetTracker().HandleMessage(&DiagnosticMessage{
		Diagnostic: Diagnostic{
			Severity: "error",
			Summary:  "Resource precondition failed",
			Detail:   "value must be positive.",
			Address:  "aws_instance.web",
		},
	})

	m.LogDiagnostics()

	out := buf.String()
	assert.Contains(t, out, "address=aws_instance.web")
	assert.Contains(t, out, "precondition failed")
}

// TestLogDiagnostic_IncludesSourceLocation verifies the file/line keyvals are attached
// when the diagnostic carries a source range, so users can jump straight to the offending
// line instead of grepping through the whole configuration.
func TestLogDiagnostic_IncludesSourceLocation(t *testing.T) {
	origLevel := log.GetLevel()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(origLevel)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.InfoLevel)

	m := NewModel("comp", "stack", "plan", strings.NewReader(""))
	m.GetTracker().HandleMessage(&DiagnosticMessage{
		Diagnostic: Diagnostic{
			Severity: "warning",
			Summary:  "Deprecated argument",
			Range: &DiagnosticRange{
				Filename: "main.tf",
				Start:    DiagnosticLocation{Line: 42},
			},
		},
	})

	m.LogDiagnostics()

	out := buf.String()
	assert.Contains(t, out, "file=main.tf")
	assert.Contains(t, out, "line=42")
}

// TestLogDiagnostic_UndeclaredVariableKeyval verifies the extra keyvals returned by
// formatDiagnosticMessage (e.g. var=name for undeclared variables) actually make it through
// logDiagnostic into the logged output, not just into the formatted message string.
func TestLogDiagnostic_UndeclaredVariableKeyval(t *testing.T) {
	origLevel := log.GetLevel()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(origLevel)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.InfoLevel)

	m := NewModel("comp", "stack", "plan", strings.NewReader(""))
	m.GetTracker().HandleMessage(&DiagnosticMessage{
		Diagnostic: Diagnostic{
			Severity: "warning",
			Summary:  "Value for undeclared variable",
			Detail:   `The root module does not declare a variable named "environment" but a value was found.`,
		},
	})

	m.LogDiagnostics()

	out := buf.String()
	assert.Contains(t, out, "undeclared variable")
	assert.Contains(t, out, "var=environment")
}

// TestExtractFirstSentence_NoWordBoundaryTruncatesRaw covers the final fallback branch: a
// long single "word" with no spaces (so no word boundary exists within the truncation
// window) must still be truncated with an ellipsis rather than left unbounded.
func TestExtractFirstSentence_NoWordBoundaryTruncatesRaw(t *testing.T) {
	text := strings.Repeat("a", maxTextLength+50)

	result := extractFirstSentence(text)

	require.True(t, strings.HasSuffix(result, "..."))
	assert.Equal(t, text[:maxTextLength]+"...", result)
}
