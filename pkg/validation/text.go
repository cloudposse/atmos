package validation

import (
	"regexp"
	"strconv"
	"strings"
)

var gccDiagnostic = regexp.MustCompile(`(?m)^\s*(?:[-*]\s+)?([^:\n]+):(\d+):(\d+):\s*(?:error:\s*)?(.+)$`)

// FromGCCText converts the location-bearing text produced by existing stack
// validation into normalized diagnostics. Lines that do not carry a source
// location become a single file-less finding so rich output never discards
// useful validation context.
func FromGCCText(source, message string) Report {
	matches := gccDiagnostic.FindAllStringSubmatch(message, -1)
	diagnostics := make([]Diagnostic, 0, len(matches))
	for _, match := range matches {
		line, _ := strconv.Atoi(match[2])
		column, _ := strconv.Atoi(match[3])
		diagnostics = append(diagnostics, Diagnostic{
			Source: source, RuleID: source, Severity: SeverityError,
			File: strings.TrimSpace(match[1]), Line: line, Column: column,
			Message: strings.TrimSpace(match[4]),
		})
	}
	if len(diagnostics) == 0 && strings.TrimSpace(message) != "" {
		diagnostics = append(diagnostics, Diagnostic{Source: source, RuleID: source, Severity: SeverityError, Message: strings.TrimSpace(message)})
	}
	return Report{Diagnostics: diagnostics}
}
