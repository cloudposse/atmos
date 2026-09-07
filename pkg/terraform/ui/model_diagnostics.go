package ui

import (
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Text truncation constants.
const (
	maxTextLength   = 100
	minWordBoundary = 60
)

// LogDiagnostics sends all diagnostics to the Atmos logger at appropriate severity levels.
// Call this after the TUI completes to display warnings after the completion message.
func (m *Model) LogDiagnostics() {
	defer perf.Track(nil, "terraform.ui.Model.LogDiagnostics")()

	for _, diag := range m.tracker.GetDiagnostics() {
		m.logDiagnostic(diag)
	}
}

// logDiagnostic logs a single diagnostic at the appropriate level based on severity.
func (m *Model) logDiagnostic(diag *DiagnosticMessage) {
	// Format the diagnostic message - extract key info and make it concise.
	msg, extraKeyvals := formatDiagnosticMessage(diag.Diagnostic.Summary, diag.Diagnostic.Detail)

	// Build structured key-value pairs for additional context.
	// Note: We don't include stack/component since they're already shown in the completion message.
	var keyvals []interface{}

	// Add extra keyvals from formatting (e.g., var=name for undeclared variables).
	keyvals = append(keyvals, extraKeyvals...)

	// Add resource address if present.
	if diag.Diagnostic.Address != "" {
		keyvals = append(keyvals, "address", diag.Diagnostic.Address)
	}

	// Add source location if present.
	if diag.Diagnostic.Range != nil {
		keyvals = append(
			keyvals,
			log.FieldFile, diag.Diagnostic.Range.Filename,
			"line", diag.Diagnostic.Range.Start.Line,
		)
	}

	// Route to appropriate logger level based on severity.
	switch diag.Diagnostic.Severity {
	case "error":
		log.Error(msg, keyvals...)
	case "warning":
		log.Warn(msg, keyvals...)
	default:
		log.Info(msg, keyvals...)
	}
}

// formatDiagnosticMessage formats a Terraform diagnostic into a concise log message.
// It recognizes common patterns and extracts key information.
// Returns the message and optional extra keyvals for structured logging.
func formatDiagnosticMessage(summary, detail string) (string, []interface{}) {
	summary = strings.TrimSpace(summary)
	detail = strings.TrimSpace(detail)

	// Pattern: "Value for undeclared variable" - extract variable name as keyval.
	if summary == "Value for undeclared variable" {
		if varName := extractQuotedValue(detail, "variable named"); varName != "" {
			return "undeclared variable", []interface{}{"var", varName}
		}
	}

	// Pattern: "Check block assertion failed" - extract the error message.
	if summary == "Check block assertion failed" {
		// The detail contains the assertion error message.
		if firstSentence := extractFirstSentence(detail); firstSentence != "" {
			return "check failed: " + firstSentence, nil
		}
		return "check failed", nil
	}

	// Pattern: "Resource precondition failed" - extract the error message.
	if summary == "Resource precondition failed" {
		if firstSentence := extractFirstSentence(detail); firstSentence != "" {
			return "precondition failed: " + firstSentence, nil
		}
		return "precondition failed", nil
	}

	// Default: use summary with first sentence of detail if available.
	if detail != "" {
		if firstSentence := extractFirstSentence(detail); firstSentence != "" {
			return summary + ": " + firstSentence, nil
		}
	}
	return summary, nil
}

// extractQuotedValue extracts a quoted value following a prefix pattern.
// For example: extractQuotedValue(`variable named "foo"`, "variable named") returns "foo".
func extractQuotedValue(text, prefix string) string {
	idx := strings.Index(text, prefix)
	if idx < 0 {
		return ""
	}

	// Find the opening quote after the prefix.
	remainder := text[idx+len(prefix):]
	startQuote := strings.Index(remainder, `"`)
	if startQuote < 0 {
		return ""
	}

	// Find the closing quote.
	afterStart := remainder[startQuote+1:]
	endQuote := strings.Index(afterStart, `"`)
	if endQuote < 0 {
		return ""
	}

	return afterStart[:endQuote]
}

// extractFirstSentence returns the first sentence from a block of text.
func extractFirstSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	// Find the first sentence ending with a period followed by space or newline.
	for i := 0; i < len(text)-1; i++ {
		if text[i] == '.' && (text[i+1] == ' ' || text[i+1] == '\n') {
			return strings.TrimSpace(text[:i+1])
		}
	}

	// If no sentence boundary found, check if text ends with period.
	if text[len(text)-1] == '.' {
		return text
	}

	// Return the whole text if it's short enough.
	if len(text) <= maxTextLength {
		return text
	}

	// Truncate at word boundary.
	truncated := text[:maxTextLength]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > minWordBoundary {
		return truncated[:lastSpace] + "..."
	}
	return truncated + "..."
}
