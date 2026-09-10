package provenance

import (
	"fmt"
	"strings"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
)

// updateImportsProvenance updates provenance paths from "imports" to "import".
func updateImportsProvenance(ctx *m.MergeContext) {
	// Update main imports key - replay entire chain to preserve inheritance history.
	if ctx.HasProvenance(importsKey) {
		if entries := ctx.GetProvenance(importsKey); len(entries) > 0 {
			for _, entry := range entries {
				ctx.RecordProvenance("import", entry)
			}
		}
	}

	// Update array element paths - replay entire chain to preserve inheritance history.
	for i := 0; ; i++ {
		oldPath := fmt.Sprintf("%s[%d]", importsKey, i)
		if !ctx.HasProvenance(oldPath) {
			break
		}
		if entries := ctx.GetProvenance(oldPath); len(entries) > 0 {
			newPath := fmt.Sprintf("import[%d]", i)
			for _, entry := range entries {
				ctx.RecordProvenance(newPath, entry)
			}
		}
	}
}

// renameImportsToImport renames "imports" key to "import" for rendering.
func renameImportsToImport(data any, ctx *m.MergeContext) any {
	defer perf.Track(nil, "provenance.renameImportsToImport")()

	dataMap, ok := data.(map[string]any)
	if !ok || ctx == nil {
		return data
	}

	// Check if "imports" key exists.
	_, hasImports := dataMap[importsKey]
	if !hasImports {
		return data
	}

	// Create new map with "import" instead of "imports".
	newMap := make(map[string]any, len(dataMap))
	for k, v := range dataMap {
		if k == importsKey {
			newMap["import"] = v
		} else {
			newMap[k] = v
		}
	}

	// Update provenance paths from "imports" → "import".
	updateImportsProvenance(ctx)

	return newMap
}

// filterEmptySections removes top-level sections that are both empty and have
// no recorded provenance. This prevents displaying sections like "backend: {}"
// or "overrides: {}" when they weren't explicitly defined in any file and are
// just generated placeholders.
//
// A section is kept if it has recorded provenance OR its value is genuinely
// non-empty. The "OR non-empty" half matters because many component sections
// (e.g. aws/cloudformation's path/stack_name/parameters/hooks/settings, which
// are copied into the final component map as plain values rather than merged
// key-by-key through the provenance-tracked merge path) never get a per-key
// provenance entry recorded at all, even though they carry real, non-empty
// data. Treating "no provenance" as "must be empty" silently dropped those
// populated sections from `describe component --provenance` output — see
// docs/fixes/2026-09-09-cfn-describe-component-missing-fields.md.
func filterEmptySections(data any, ctx *m.MergeContext) any {
	defer perf.Track(nil, "provenance.filterEmptySections")()

	dataMap, ok := data.(map[string]any)
	if !ok {
		return data
	}

	// Create a new map to hold filtered results
	filtered := make(map[string]any)

	for key, value := range dataMap {
		if hasSectionProvenance(ctx, key) || !isEmptyValue(value) {
			filtered[key] = value
		}
	}

	return filtered
}

// isEmptyValue reports whether value is a shape Atmos uses for generated
// placeholder sections that were never configured: nil, an empty map, or an
// empty slice. Scalars (including the empty string) are never considered
// empty here, since a deliberately-set empty string is real data, not a
// placeholder.
func isEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case map[string]any:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return false
	}
}

// hasSectionProvenance reports whether any recorded provenance path belongs to
// the given top-level section key, once normalized the same way findProvenance
// does (stripping the "components.<type>.<component>." prefix). Recorded paths
// are always prefixed that way (e.g. "components.terraform.app.vars.vpc_id"),
// so a raw, unnormalized key like "vars" would never match without this.
// When ctx is nil (provenance tracking disabled), every key is kept.
func hasSectionProvenance(ctx *m.MergeContext, key string) bool {
	if ctx == nil {
		return true
	}

	for _, storedPath := range ctx.GetProvenancePaths() {
		normalized := normalizeProvenancePath(storedPath)
		if normalized == key || strings.HasPrefix(normalized, key+pathSeparator) || strings.HasPrefix(normalized, key+"[") {
			return true
		}
	}

	return false
}
