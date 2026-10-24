package exec

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/degradation"
	functionpkg "github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	_ "github.com/cloudposse/atmos/pkg/script/starlark" // Register the embedded value evaluator.
)

type configurationResolver struct {
	config        *schema.AtmosConfiguration
	stack         string
	skip          []string
	resolution    *ResolutionContext
	info          *schema.ConfigAndStacksInfo
	warning       func(DegradationWarning)
	scope         map[string]any
	cache         map[string]any
	active        []string
	computedReads uint64
	sources       map[string]string
}

type configurationResolverOption func(*configurationResolver)

func withConfigurationContext(stack string, info *schema.ConfigAndStacksInfo) configurationResolverOption {
	return func(r *configurationResolver) { r.stack, r.info = stack, info }
}

func withConfigurationFunctions(skip []string, resolution *ResolutionContext) configurationResolverOption {
	return func(r *configurationResolver) { r.skip, r.resolution = skip, resolution }
}

func newConfigurationResolver(config *schema.AtmosConfiguration, input map[string]any, options ...configurationResolverOption) *configurationResolver {
	resolver := &configurationResolver{config: config, cache: make(map[string]any)}
	for _, option := range options {
		option(resolver)
	}
	scope := map[string]any{}
	if resolver.info != nil {
		maps.Copy(scope, resolver.info.ComponentSection)
	}
	resolver.scope = configurationScope(scope, input)
	return resolver
}

func containsStarlark(value any) bool {
	switch value := value.(type) {
	case string:
		return starlarksource.IsEncoded(value)
	case map[string]any:
		for _, item := range value {
			if containsStarlark(item) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if containsStarlark(item) {
				return true
			}
		}
	}
	return false
}

func (r *configurationResolver) resolve(path []string, raw any) (any, error) {
	key := configurationPathKey(path)
	if value, ok := r.cache[key]; ok {
		return value, nil
	}
	if slices.Contains(r.active, key) {
		chain := append(slices.Clone(r.active), key)
		return nil, fmt.Errorf("%w: !starlark configuration values %s", functionpkg.ErrCircularDependency, strings.Join(chain, " → "))
	}
	r.active = append(r.active, key)
	defer func() { r.active = r.active[:len(r.active)-1] }()
	value, err := r.resolveNode(path, raw)
	if err != nil {
		return nil, fmt.Errorf("configuration %s: %w", strings.Join(path, "."), err)
	}
	r.cache[key] = value
	return value, nil
}

func (r *configurationResolver) resolveNode(path []string, raw any) (any, error) {
	switch value := raw.(type) {
	case map[string]any:
		return r.resolveMap(path, value)
	case []any:
		result := make([]any, 0, len(value))
		for i, item := range value {
			resolved, err := r.resolve(append(slices.Clone(path), strconv.Itoa(i)), item)
			if err != nil {
				return nil, err
			}
			if _, unset := resolved.(UnsetMarker); !unset {
				result = append(result, resolved)
			}
		}
		return result, nil
	case string:
		return r.resolveFieldString(path, value)
	default:
		return value, nil
	}
}

func (r *configurationResolver) resolveMap(path []string, values map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(values))
	for _, key := range slices.Sorted(maps.Keys(values)) {
		value, err := r.resolve(append(slices.Clone(path), key), values[key])
		if err != nil {
			return nil, err
		}
		if _, unset := value.(UnsetMarker); !unset {
			result[key] = value
		}
	}
	return result, nil
}

func (r *configurationResolver) resolveString(value string) (any, error) {
	if starlarksource.IsEncoded(value) {
		if skipFunc(r.skip, starlarksource.Tag) {
			return value, nil
		}
		fn, err := functionpkg.DefaultRegistry().Get(functionpkg.TagStarlark)
		if err != nil {
			return nil, err
		}
		return fn.Execute(context.Background(), value, &functionpkg.ExecutionContext{EvaluateValue: r.evaluate})
	}
	resolved, err := processCustomTagsWithContext(r.config, value, r.stack, r.skip, r.resolution, r.info)
	if err != nil && r.warning != nil && canDegradeValue(r.config, err) {
		r.degrade(value, err)
		return degradation.AtmosComputedValue{}, nil
	}
	return resolved, err
}

func (r *configurationResolver) evaluate(ctx context.Context, value string) (any, error) {
	engine, ok := script.Get("starlark")
	if !ok {
		return nil, fmt.Errorf("%w: Starlark interpreter is unavailable", functionpkg.ErrExecutionFailed)
	}
	evaluator, ok := engine.(script.ValueEvaluator)
	if !ok {
		return nil, fmt.Errorf("%w: Starlark interpreter does not support configuration values", functionpkg.ErrExecutionFailed)
	}
	source := starlarksource.Decode(value)
	before := r.computedReads
	result, err := evaluator.EvaluateValue(ctx, script.Evaluation{Source: source.Code, Filename: source.File, Line: source.Line, Context: r.context()})
	if r.computedReads != before {
		// Preserve unknown values through dependent expressions in lenient inspection.
		return degradation.AtmosComputedValue{}, nil
	}
	return result, err
}

func (r *configurationResolver) resolveFieldString(path []string, value string) (any, error) {
	if r.sources != nil {
		source, found := r.sources[configurationPathKey(path)]
		if !found || source != value {
			return value, nil
		}
	}
	return r.resolveString(value)
}
