package stack

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// ComponentsSectionName is the top-level stack manifest key under which all
// components are nested.
const ComponentsSectionName = "components"

// BuildComponentInFilePath maps a component-relative dot-path (e.g. "vars.region")
// to its location inside a stack manifest file, which nests component config
// under components.<type>.<name>. Segments are joined raw (unescaped), matching
// how merge provenance keys are recorded (see pkg/utils.AppendJSONPathKey), so
// this form is used for provenance lookups. For addressing YAML nodes safely,
// use BuildComponentYqPath instead.
//
// Example: ("terraform", "vpc", "vars.region") -> "components.terraform.vpc.vars.region".
func BuildComponentInFilePath(componentType, componentName, relPath string) string {
	defer perf.Track(nil, "stack.BuildComponentInFilePath")()

	parts := []string{ComponentsSectionName, componentType, componentName}
	if relPath != "" {
		parts = append(parts, relPath)
	}
	return strings.Join(parts, ".")
}

// BuildComponentYqPath builds the same component-relative location as
// BuildComponentInFilePath but quotes the literal structural segments
// (componentType, componentName) when they are not simple identifiers, so a
// component named "vpc.prod" or "foo[0]" addresses the correct manifest node
// instead of being parsed as nested path syntax. The relPath is appended as-is
// because it is already user-facing dot-path syntax (parsed downstream by the
// YAML path translator). Use this form for all real YAML reads/writes.
//
// Example: ("terraform", `vpc.prod`, "vars.region") ->
//
//	`components.terraform."vpc.prod".vars.region`.
func BuildComponentYqPath(componentType, componentName, relPath string) string {
	defer perf.Track(nil, "stack.BuildComponentYqPath")()

	parts := []string{
		ComponentsSectionName,
		atmosyaml.QuotePathSegment(componentType),
		atmosyaml.QuotePathSegment(componentName),
	}
	joined := strings.Join(parts, ".")
	if relPath != "" {
		joined += "." + relPath
	}
	return joined
}

// PickProvenanceFile returns the source file (and line) of the winning provenance
// entry for a path. Provenance records entries in merge order, so the LAST entry
// is the one whose value is effective after deep-merge. Returns ok=false when
// there are no entries (the path is not defined anywhere).
func PickProvenanceFile(entries []merge.ProvenanceEntry) (file string, line int, ok bool) {
	defer perf.Track(nil, "stack.PickProvenanceFile")()

	if len(entries) == 0 {
		return "", 0, false
	}
	last := entries[len(entries)-1]
	return last.File, last.Line, true
}
