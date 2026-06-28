package stack

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ComponentsSectionName is the top-level stack manifest key under which all
// components are nested.
const ComponentsSectionName = "components"

// BuildComponentInFilePath maps a component-relative dot-path (e.g. "vars.region")
// to its location inside a stack manifest file, which nests component config
// under components.<type>.<name>.
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
