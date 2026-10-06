package exec

import (
	"fmt"
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/degradation"
	functionpkg "github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type configurationContext struct {
	resolver *configurationResolver
	values   map[string]any
	path     []string
}

func (m *configurationContext) Keys() []string {
	defer perf.Track(m.resolver.config, "exec.configurationContext.Keys")()

	keys := make([]string, 0, len(m.values))
	for key, value := range m.values {
		if text, ok := value.(string); ok && matchesPrefix(text, u.AtmosYamlFuncUnset, m.resolver.skip) {
			continue
		}
		if marker, ok := value.(UnsetMarker); ok && marker.IsUnset {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func (m *configurationContext) Get(key string) (any, bool, error) {
	defer perf.Track(m.resolver.config, "exec.configurationContext.Get")()

	raw, found := m.values[key]
	if !found {
		return nil, false, nil
	}
	path := append(slices.Clone(m.path), key)
	if values, ok := raw.(map[string]any); ok {
		return &configurationContext{resolver: m.resolver, values: values, path: path}, true, nil
	}
	if values, ok := raw.(script.ValueMap); ok {
		return values, true, nil
	}
	value, err := m.resolver.resolve(path, raw)
	if isComputedConfiguration(value) {
		m.resolver.computedReads++
		return nil, false, fmt.Errorf("%w: configuration dependency %s is unresolved", functionpkg.ErrExecutionFailed, key)
	}
	if _, unset := value.(UnsetMarker); unset {
		return nil, false, nil
	}
	return value, true, err
}

func (r *configurationResolver) context() script.ValueMap {
	values := map[string]any{"stack": r.stack}
	if r.info != nil {
		values["component"] = r.info.Component
		values["component_type"] = r.info.ComponentType
	}
	for _, name := range []string{"vars", "metadata", "settings", "env", "locals"} {
		section, ok := r.scope[name]
		if !ok {
			section = map[string]any{}
		}
		values[name] = section
	}
	return &configurationContext{resolver: r, values: values}
}

func (r *configurationResolver) degrade(value string, err error) {
	component := ""
	if r.info != nil {
		component = r.info.Component
	}
	r.warning(DegradationWarning{Stack: r.stack, Component: component, Function: value, Reason: err.Error()})
}

// configurationScope overlays rendered fields while retaining siblings excluded
// from selective evaluation. It never writes to either input map.
func configurationScope(base, overrides map[string]any) map[string]any {
	result := maps.Clone(base)
	if result == nil {
		result = make(map[string]any)
	}
	for key, value := range overrides {
		if values, ok := value.(map[string]any); ok {
			original, _ := result[key].(map[string]any)
			value = configurationScope(original, values)
		}
		result[key] = value
	}
	return result
}

func isComputedConfiguration(value any) bool {
	switch value := value.(type) {
	case degradation.AtmosComputedValue:
		return true
	case []any:
		for _, item := range value {
			if isComputedConfiguration(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range value {
			if isComputedConfiguration(item) {
				return true
			}
		}
	}
	return false
}
