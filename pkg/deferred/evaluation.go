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
				length := min(len(field), len(path))
				if slices.Equal(field[:length], path[:length]) {
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
func ExpandEvaluationPaths(data map[string]any, paths [][]string) [][]string {
	defer perf.Track(nil, "deferred.ExpandEvaluationPaths")()

	if paths == nil {
		return nil
	}
	result := slices.Clone(paths)
	if result == nil {
		result = make([][]string, 0)
	}
	seen := make(map[string]bool)
	for i := 0; i < len(result); i++ {
		path := result[i]
		key := strings.Join(path, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		var value any = data
		for _, part := range path {
			section, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = section[part]
		}
		refs, ok := evaluationReferences(value)
		if !ok {
			return nil
		}
		for _, ref := range refs {
			if !seen[strings.Join(ref, "\x00")] {
				result = append(result, ref)
			}
		}
	}
	return result
}

func evaluationReferences(value any) ([][]string, bool) {
	var paths [][]string
	switch v := value.(type) {
	case map[string]any:
		for _, child := range v {
			refs, ok := evaluationReferences(child)
			if !ok {
				return nil, false
			}
			paths = append(paths, refs...)
		}
	case []any:
		for _, child := range v {
			refs, ok := evaluationReferences(child)
			if !ok {
				return nil, false
			}
			paths = append(paths, refs...)
		}
	case string:
		return templateEvaluationReferences(v)
	}
	return paths, true
}

func templateEvaluationReferences(value string) ([][]string, bool) {
	if !strings.Contains(value, "{{") {
		return nil, true
	}
	refs, static := atmostemplate.StaticFieldRefs(value)
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
		if len(path) == 0 || path[0] != key {
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
