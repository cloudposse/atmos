package exec

import (
	"regexp"
	"strings"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/validation"
)

var componentInstanceLocation = regexp.MustCompile(`"instanceLocation"\s*:\s*"([^"]*)"`)

// ComponentValidationReport adapts the established component validator error
// contract into display diagnostics. JSON Schema exposes instance locations in
// its BasicOutput JSON; OPA continues to return strings and is anchored to the
// selected component declaration by the caller's fallback diagnostic.
func ComponentValidationReport(component string, err error) validation.Report {
	if err == nil {
		return validation.Report{}
	}
	context := GetLastMergeContext()
	matches := componentInstanceLocation.FindAllStringSubmatch(err.Error(), -1)
	report := validation.Report{}
	for _, match := range matches {
		path := componentPointerPath(match[1])
		entry := componentProvenance(context, component, path)
		diagnostic := validation.Diagnostic{
			Source: "component", RuleID: "jsonschema", Severity: validation.SeverityError,
			Message: err.Error(),
		}
		if entry != nil {
			diagnostic.File = entry.File
			diagnostic.Line = entry.Line
			diagnostic.Column = entry.Column
		}
		report.Diagnostics = append(report.Diagnostics, diagnostic)
	}
	if len(report.Diagnostics) == 0 {
		report.Diagnostics = append(report.Diagnostics, validation.Diagnostic{
			Source: "component", RuleID: "component", Severity: validation.SeverityError, Message: err.Error(),
		})
	}
	return report
}

func componentPointerPath(pointer string) string {
	pointer = strings.TrimPrefix(pointer, "/")
	if pointer == "" {
		return ""
	}
	parts := strings.Split(pointer, "/")
	for index := range parts {
		parts[index] = strings.ReplaceAll(strings.ReplaceAll(parts[index], "~1", "/"), "~0", "~")
	}
	return strings.Join(parts, ".")
}

func componentProvenance(context *m.MergeContext, component, field string) *m.ProvenanceEntry {
	if context == nil || !context.IsProvenanceEnabled() {
		return nil
	}
	suffix := field
	if suffix != "" {
		suffix = "." + suffix
	}
	componentNeedle := "." + component
	var best *m.ProvenanceEntry
	for _, path := range context.GetProvenancePaths() {
		if component != "" && !strings.Contains(path, componentNeedle) {
			continue
		}
		if suffix != "" && !strings.HasSuffix(path, suffix) {
			continue
		}
		entries := context.GetProvenance(path)
		if len(entries) == 0 {
			continue
		}
		entry := entries[len(entries)-1]
		best = &entry
		break
	}
	if best != nil || field == "" {
		return best
	}
	// A JSON Schema error can name an object rather than a scalar. Fall back
	// through ancestor paths until one has a concrete source entry.
	for field != "" {
		if cut := strings.LastIndex(field, "."); cut >= 0 {
			field = field[:cut]
		} else {
			field = ""
		}
		if entry := componentProvenance(context, component, field); entry != nil {
			return entry
		}
	}
	return nil
}
