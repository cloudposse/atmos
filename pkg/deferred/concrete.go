package deferred

import (
	"strings"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultLeftDelimiter is the Go template opening delimiter used when none is configured.
const defaultLeftDelimiter = "{{"

// ConcreteStringFilter returns the predicate WithConcreteResolution uses to pick the strings that
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

// WithConcreteResolution decorates a YAML processor to render only inherited strings
// that survive deferred merging. Concrete containers are copied and their unresolved
// strings are wrapped as opaque leaves before merging. The normal map/list precedence
// rules then discard replaced leaves without evaluating them. Function results are
// never wrapped, so a function returning literal template/tag text is not run again.
func WithConcreteResolution(processor m.YAMLFunctionProcessor, shouldResolve func(string) bool) m.YAMLFunctionProcessor {
	defer perf.Track(nil, "deferred.WithConcreteResolution")()

	if processor == nil || shouldResolve == nil {
		return processor
	}
	return &concreteProcessor{YAMLFunctionProcessor: processor, shouldResolve: shouldResolve}
}

type concreteProcessor struct {
	m.YAMLFunctionProcessor
	shouldResolve func(string) bool
}

// concreteString is an immutable scalar during merging, not a map or a function.
// Only strings inside concrete map/list layers are wrapped; scalar layers retain
// their existing replacement semantics.
type concreteString struct {
	value string
}

func (p *concreteProcessor) DeferConcreteValue(value any) any {
	defer perf.Track(nil, "deferred.concreteProcessor.DeferConcreteValue")()

	if !isContainer(value) {
		return value
	}
	return p.deferStrings(value)
}

func (p *concreteProcessor) deferStrings(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, child := range v {
			result[key] = p.deferStrings(child)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, child := range v {
			result[i] = p.deferStrings(child)
		}
		return result
	case string:
		if p.shouldResolve(v) {
			return concreteString{value: v}
		}
	}
	return value
}

func (p *concreteProcessor) ResolveConcreteValue(value any) (any, error) {
	defer perf.Track(nil, "deferred.concreteProcessor.ResolveConcreteValue")()

	return resolveContainer(value, p.YAMLFunctionProcessor)
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

// resolveContainer returns a copy with only the surviving concreteString leaves resolved.
func resolveContainer(v any, processor m.YAMLFunctionProcessor) (any, error) {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved, err := resolveContainer(item, processor)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			resolved, err := resolveContainer(item, processor)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	case concreteString:
		return processor.ProcessYAMLFunctionString(typed.value)
	default:
		return v, nil
	}
}
