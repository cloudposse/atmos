// Package deferred selects, evaluates, and caches only the values a caller needs.
// Authentication resolution belongs to pkg/auth/deferred.
package deferred

import (
	"slices"
	"strings"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	atmostemplate "github.com/cloudposse/atmos/pkg/template"
)

// FilterDeferredFields applies the same demand boundary to the final merge pass.
// An ancestor function must run when it can supply a requested descendant.
func FilterDeferredFields(dctx *m.DeferredMergeContext, section string, paths [][]string) {
	defer perf.Track(nil, "deferred.FilterDeferredFields")()

	if paths == nil {
		return
	}
	for key, values := range dctx.GetDeferredValues() {
		needed := false
		for _, value := range values {
			field := append([]string{section}, value.Path...)
			for _, path := range paths {
				if evaluationPathsOverlap(field, path) {
					needed = true
				}
			}
		}
		if !needed {
			delete(dctx.GetDeferredValues(), key)
		}
	}
}

// ExpandEvaluationPaths includes fields referenced by selected template values.
// Dynamic scope/root access cannot be proven safe to narrow, so it evaluates fully.
// A "*" path segment selects every map key at that level. Ancestor values are
// inspected when they must be evaluated to materialize a requested descendant.
// Optional delimiters keep dependency analysis aligned with template rendering.
func ExpandEvaluationPaths(data map[string]any, paths [][]string, delimiters ...string) [][]string {
	defer perf.Track(nil, "deferred.ExpandEvaluationPaths")()

	if len(paths) == 0 {
		return paths
	}
	// Component settings override CLI defaults, including an explicit empty pair
	// which resets template rendering to the standard delimiters.
	var ok bool
	delimiters, ok = evaluationTemplateDelimiters(data, delimiters)
	if !ok {
		return nil
	}
	result := slices.Clone(paths)
	seen := make(map[string]bool)
	for i := 0; i < len(result); i++ {
		path := result[i]
		key := strings.Join(path, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		var refs [][]string
		for _, value := range evaluationPathValues(data, path) {
			dependencies, static := evaluationReferences(value, delimiters)
			if !static {
				return nil
			}
			refs = append(refs, dependencies...)
		}
		for _, ref := range refs {
			if !seen[strings.Join(ref, "\x00")] {
				result = append(result, ref)
			}
		}
	}
	return result
}

func evaluationTemplateDelimiters(data map[string]any, fallback []string) ([]string, bool) {
	settings := data
	for _, key := range []string{"settings", "templates", "settings"} {
		nested, ok := settings[key].(map[string]any)
		if !ok {
			return fallback, true
		}
		settings = nested
	}
	raw, exists := settings["delimiters"]
	if !exists {
		return fallback, true
	}
	switch delimiters := raw.(type) {
	case []string:
		return delimiters, true
	case []any:
		result := make([]string, len(delimiters))
		for i, value := range delimiters {
			delimiter, ok := value.(string)
			if !ok {
				return nil, false
			}
			result[i] = delimiter
		}
		return result, true
	default:
		return nil, false
	}
}

func evaluationReferences(value any, delimiters []string) ([][]string, bool) {
	var paths [][]string
	switch v := value.(type) {
	case map[string]any:
		for _, child := range v {
			refs, ok := evaluationReferences(child, delimiters)
			if !ok {
				return nil, false
			}
			paths = append(paths, refs...)
		}
	case []any:
		for _, child := range v {
			refs, ok := evaluationReferences(child, delimiters)
			if !ok {
				return nil, false
			}
			paths = append(paths, refs...)
		}
	case string:
		return templateEvaluationReferences(v, delimiters)
	}
	return paths, true
}

func templateEvaluationReferences(value string, delimiters []string) ([][]string, bool) {
	left := "{{"
	if len(delimiters) != 0 {
		if len(delimiters) != 2 || delimiters[0] == "" || delimiters[1] == "" {
			return nil, false
		}
		left = delimiters[0]
	}
	if !strings.Contains(value, left) {
		return nil, true
	}
	refs, static := atmostemplate.StaticFieldRefs(value, delimiters...)
	if !static {
		return nil, false
	}
	paths := make([][]string, 0, len(refs))
	for _, ref := range refs {
		paths = append(paths, ref.Path)
	}
	return paths, true
}

func SplitEvaluationFields(data map[string]any, paths [][]string) (map[string]any, map[string]any) {
	defer perf.Track(nil, "deferred.SplitEvaluationFields")()

	if paths == nil {
		return data, nil
	}
	selected, excluded := make(map[string]any), make(map[string]any)
	for key, value := range data {
		children, whole := childEvaluationPaths(paths, key)
		if whole {
			selected[key] = value
		} else if len(children) == 0 {
			excluded[key] = value
		} else if nested, ok := value.(map[string]any); ok {
			selected[key], excluded[key] = SplitEvaluationFields(nested, children)
		} else {
			selected[key] = value
		}
	}
	return selected, excluded
}

func childEvaluationPaths(paths [][]string, key string) ([][]string, bool) {
	var children [][]string
	for _, path := range paths {
		if len(path) == 0 || (path[0] != key && path[0] != "*") {
			continue
		}
		if len(path) == 1 {
			return nil, true
		}
		children = append(children, path[1:])
	}
	return children, false
}

func RestoreEvaluationFields(result, excluded map[string]any) {
	defer perf.Track(nil, "deferred.RestoreEvaluationFields")()

	for key, value := range excluded {
		if nested, ok := result[key].(map[string]any); ok {
			if remainder, ok := value.(map[string]any); ok {
				RestoreEvaluationFields(nested, remainder)
			}
		} else if _, exists := result[key]; !exists {
			result[key] = value
		}
	}
}

// evaluationPathsOverlap keeps requested ancestors and descendants while treating
// wildcard segments in the requested path as any map key.
func evaluationPathsOverlap(field, requested []string) bool {
	for i := range min(len(field), len(requested)) {
		if requested[i] != "*" && requested[i] != field[i] {
			return false
		}
	}
	return true
}

// evaluationPathValues selects dependency-bearing values, including scalar
// ancestor expressions that produce maps whose keys are not available yet.
func evaluationPathValues(value any, path []string) []any {
	if len(path) == 0 {
		return []any{value}
	}
	section, ok := value.(map[string]any)
	if !ok {
		return []any{value}
	}
	if path[0] != "*" {
		return evaluationPathValues(section[path[0]], path[1:])
	}
	var values []any
	for _, child := range section {
		values = append(values, evaluationPathValues(child, path[1:])...)
	}
	return values
}
