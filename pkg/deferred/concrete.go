package deferred

import (
	"strings"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultLeftDelimiter is the Go template opening delimiter used when none is configured.
const defaultLeftDelimiter = "{{"

// ConcreteStringFilter returns the predicate ResolveConcreteLayers uses to pick the strings that
// need resolving: anything that looks like a YAML function (a leading "!"), plus, when templates are
// available, anything that contains the configured left template delimiter.
//
// Pass templatesAvailable=false when no template context exists for this invocation (templating
// disabled). Rendering would then fail loudly, and the whole-document template pass did not render
// the surrounding document either, so such strings are left exactly as written.
func ConcreteStringFilter(delimiters []string, templatesAvailable bool) func(string) bool {
	defer perf.Track(nil, "deferred.ConcreteStringFilter")()

	left := defaultLeftDelimiter
	if len(delimiters) == 2 && delimiters[0] != "" {
		left = delimiters[0]
	}

	return func(s string) bool {
		return strings.HasPrefix(s, "!") || (templatesAvailable && strings.Contains(s, left))
	}
}

// ResolveConcreteLayers resolves the Go templates and YAML functions inside the concrete
// (non-function) map and list layers of every deferred path.
//
// When a component overrides a map with a YAML function (for example `vars.tags: !labels`), the
// structural merge keeps the lower-precedence map as a separate, concrete layer and merges it back
// in only after the function resolves. The document-wide template and YAML function passes ran
// earlier, when the field held just the function string, so they never saw that inherited map: its
// `{{ .atmos_stack }}` and `!git.repository` strings would otherwise reach the merged result
// verbatim. Calling this before ApplyDeferredMerges renders them first, so the function override
// and the plain-map override produce the same inherited values.
//
// Scalar layers are left alone: a scalar layer is replaced wholesale, never merged, so resolving it
// would be wasted work. The values handed to dctx are never mutated in place, because the context
// may share its maps with a cache. Resolved copies replace the entries instead.
func ResolveConcreteLayers(dctx *m.DeferredMergeContext, processor m.YAMLFunctionProcessor, shouldResolve func(string) bool) error {
	defer perf.Track(nil, "deferred.ResolveConcreteLayers")()

	if dctx == nil || processor == nil || shouldResolve == nil {
		return nil
	}

	for _, layers := range dctx.GetDeferredValues() {
		for _, layer := range layers {
			if layer.IsFunction || !isContainer(layer.Value) {
				continue
			}
			resolved, err := resolveContainer(layer.Value, processor, shouldResolve)
			if err != nil {
				return err
			}
			layer.Value = resolved
		}
	}

	return nil
}

// isContainer reports whether v is a map or a list, the only layer shapes that deep-merge.
func isContainer(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

// resolveContainer returns a copy of v with every selected string leaf resolved by the processor.
func resolveContainer(v any, processor m.YAMLFunctionProcessor, shouldResolve func(string) bool) (any, error) {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved, err := resolveContainer(item, processor, shouldResolve)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			resolved, err := resolveContainer(item, processor, shouldResolve)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	case string:
		if !shouldResolve(typed) {
			return typed, nil
		}
		return processor.ProcessYAMLFunctionString(typed)
	default:
		return v, nil
	}
}
