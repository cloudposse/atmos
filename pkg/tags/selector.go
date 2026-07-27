package tags

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SelectorUnresolved reports whether a metadata.tags/metadata.labels value
// still contains an unresolved marker, meaning its real value can only be
// known after template/YAML-function evaluation:
//   - a Go template marker ("{{"), or
//   - an unprocessed Atmos YAML function (custom-tagged scalars are stored as
//     plain strings like "!env DEPLOY_TIER" until function processing runs —
//     see getValueWithTag in pkg/utils).
//
// Callers use this to treat such selectors as undecidable and fall through to
// full evaluation instead of silently misjudging them as out-of-scope. A
// quoted literal that merely looks like a marker is conservatively treated as
// unresolved too — the safe direction, since it only forces evaluation.
func SelectorUnresolved(v any) bool {
	defer perf.Track(nil, "tags.SelectorUnresolved")()

	return selectorUnresolved(v)
}

// selectorUnresolved walks nested slices/maps without re-entering perf tracking.
func selectorUnresolved(v any) bool {
	switch value := v.(type) {
	case string:
		return strings.Contains(value, "{{") || strings.HasPrefix(strings.TrimSpace(value), "!")
	case []any:
		for _, item := range value {
			if selectorUnresolved(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range value {
			if selectorUnresolved(item) {
				return true
			}
		}
	}
	return false
}
