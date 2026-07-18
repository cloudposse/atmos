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
	// Preserve any text that isn't part of a matched GCC-formatted line (e.g.
	// general stack setup errors mixed in with schema failures) instead of
	// discarding it whenever at least one diagnostic was parsed.
	unmatched := strings.TrimSpace(gccDiagnostic.ReplaceAllString(message, ""))
	if unmatched != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Source: source, RuleID: source, Severity: SeverityError,
			Message: unmatched,
		})
	}
	return Report{Diagnostics: diagnostics}
}
