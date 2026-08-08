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

// filterEmptySections removes top-level sections that have no provenance.
// This prevents displaying sections like "backend: {}" or "overrides: {}" when they
// weren't explicitly defined in any file and are just generated placeholders.
func filterEmptySections(data any, ctx *m.MergeContext) any {
	defer perf.Track(nil, "provenance.filterEmptySections")()

	dataMap, ok := data.(map[string]any)
	if !ok {
		return data
	}

	// Create a new map to hold filtered results
	filtered := make(map[string]any)

	for key, value := range dataMap {
		if hasSectionProvenance(ctx, key) {
			filtered[key] = value
		}
	}

	return filtered
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
