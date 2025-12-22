package locals

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestResolver_SimpleResolution(t *testing.T) {
	locals := map[string]any{
		"a": "value-a",
		"b": "{{ .locals.a }}-extended",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "value-a", result["a"])
	assert.Equal(t, "value-a-extended", result["b"])
}

func TestResolver_ChainedLocals(t *testing.T) {
	locals := map[string]any{
		"a": "start",
		"b": "{{ .locals.a }}-middle",
		"c": "{{ .locals.b }}-end",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "start", result["a"])
	assert.Equal(t, "start-middle", result["b"])
	assert.Equal(t, "start-middle-end", result["c"])
}

func TestResolver_OrderIndependent(t *testing.T) {
	// Defined in reverse dependency order.
	locals := map[string]any{
		"c": "{{ .locals.b }}-c",
		"b": "{{ .locals.a }}-b",
		"a": "start",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "start", result["a"])
	assert.Equal(t, "start-b", result["b"])
	assert.Equal(t, "start-b-c", result["c"])
}

func TestResolver_ParentScopeAccess(t *testing.T) {
	parentLocals := map[string]any{
		"global": "from-parent",
	}
	locals := map[string]any{
		"child": "{{ .locals.global }}-child",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(parentLocals)

	require.NoError(t, err)
	assert.Equal(t, "from-parent", result["global"]) // Parent local preserved.
	assert.Equal(t, "from-parent-child", result["child"])
}

func TestResolver_CycleDetection(t *testing.T) {
	locals := map[string]any{
		"a": "{{ .locals.c }}",
		"b": "{{ .locals.a }}",
		"c": "{{ .locals.b }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrLocalsCircularDep), "error should be ErrLocalsCircularDep sentinel")
	assert.Contains(t, err.Error(), "circular dependency")
	// The cycle should be detected (order may vary due to map iteration).
	assert.Contains(t, err.Error(), "→")
}

func TestResolver_SelfReference(t *testing.T) {
	locals := map[string]any{
		"a": "{{ .locals.a }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolver_NonStringValues(t *testing.T) {
	locals := map[string]any{
		"number":  42,
		"boolean": true,
		"list":    []any{1, 2, 3},
		"map":     map[string]any{"key": "value"},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, 42, result["number"])
	assert.Equal(t, true, result["boolean"])
	assert.Equal(t, []any{1, 2, 3}, result["list"])
	assert.Equal(t, map[string]any{"key": "value"}, result["map"])
}

func TestResolver_EmptyLocals(t *testing.T) {
	resolver := NewResolver(map[string]any{}, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestResolver_EmptyLocalsWithParent(t *testing.T) {
	parentLocals := map[string]any{
		"parent": "value",
	}

	resolver := NewResolver(map[string]any{}, "test.yaml")
	result, err := resolver.Resolve(parentLocals)

	require.NoError(t, err)
	assert.Equal(t, "value", result["parent"])
}

func TestResolver_NilLocals(t *testing.T) {
	resolver := NewResolver(nil, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestResolver_MultipleRefsInOneLocal(t *testing.T) {
	locals := map[string]any{
		"a":      "first",
		"b":      "second",
		"c":      "third",
		"result": "{{ .locals.a }}-{{ .locals.b }}-{{ .locals.c }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "first-second-third", result["result"])
}

func TestResolver_SprigFunctions(t *testing.T) {
	locals := map[string]any{
		"name":   "hello",
		"upper":  "{{ .locals.name | upper }}",
		"quoted": `{{ .locals.name | quote }}`,
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "hello", result["name"])
	assert.Equal(t, "HELLO", result["upper"])
	assert.Equal(t, `"hello"`, result["quoted"])
}

func TestResolver_ConditionalTemplate(t *testing.T) {
	locals := map[string]any{
		"env":    "prod",
		"result": `{{ if eq .locals.env "prod" }}production{{ else }}development{{ end }}`,
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "production", result["result"])
}

func TestResolver_MapWithTemplates(t *testing.T) {
	locals := map[string]any{
		"prefix": "app",
		"config": map[string]any{
			"name": "{{ .locals.prefix }}-service",
			"port": 8080,
		},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	config := result["config"].(map[string]any)
	assert.Equal(t, "app-service", config["name"])
	assert.Equal(t, 8080, config["port"])
}

func TestResolver_SliceWithTemplates(t *testing.T) {
	locals := map[string]any{
		"domain": "example.com",
		"hosts": []any{
			"www.{{ .locals.domain }}",
			"api.{{ .locals.domain }}",
		},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	hosts := result["hosts"].([]any)
	assert.Equal(t, "www.example.com", hosts[0])
	assert.Equal(t, "api.example.com", hosts[1])
}

func TestResolver_NestedMapWithTemplates(t *testing.T) {
	locals := map[string]any{
		"env": "prod",
		"config": map[string]any{
			"database": map[string]any{
				"host": "db-{{ .locals.env }}.example.com",
			},
		},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	config := result["config"].(map[string]any)
	database := config["database"].(map[string]any)
	assert.Equal(t, "db-prod.example.com", database["host"])
}

func TestResolver_NoTemplateString(t *testing.T) {
	locals := map[string]any{
		"plain": "just a plain string without templates",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "just a plain string without templates", result["plain"])
}

func TestResolver_GetDependencies(t *testing.T) {
	locals := map[string]any{
		"a": "value",
		"b": "{{ .locals.a }}",
		"c": "{{ .locals.a }}-{{ .locals.b }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	err := resolver.buildDependencyGraph()
	require.NoError(t, err)

	deps := resolver.GetDependencies()

	assert.Empty(t, deps["a"])
	assert.Equal(t, []string{"a"}, deps["b"])

	// Sort for deterministic comparison.
	sort.Strings(deps["c"])
	assert.Equal(t, []string{"a", "b"}, deps["c"])
}

func TestResolver_ParentDoesNotCreateCycle(t *testing.T) {
	// Parent scope local should not create a cycle.
	parentLocals := map[string]any{
		"parent": "parent-value",
	}
	locals := map[string]any{
		"child": "{{ .locals.parent }}-extended",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(parentLocals)

	require.NoError(t, err)
	assert.Equal(t, "parent-value-extended", result["child"])
}

func TestResolver_OverrideParentLocal(t *testing.T) {
	parentLocals := map[string]any{
		"shared": "parent-value",
	}
	locals := map[string]any{
		"shared": "child-value", // Override parent.
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(parentLocals)

	require.NoError(t, err)
	assert.Equal(t, "child-value", result["shared"])
}

func TestResolver_UndefinedLocalError(t *testing.T) {
	locals := map[string]any{
		"a": "{{ .locals.undefined }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve local")
	assert.Contains(t, err.Error(), "test.yaml")
}

func TestResolver_InvalidTemplate(t *testing.T) {
	locals := map[string]any{
		"a": "{{ .locals.foo }",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse template")
}

func TestResolver_DiamondDependency(t *testing.T) {
	// Diamond dependency pattern:
	//     a
	//    / \
	//   b   c
	//    \ /
	//     d
	locals := map[string]any{
		"a": "root",
		"b": "{{ .locals.a }}-left",
		"c": "{{ .locals.a }}-right",
		"d": "{{ .locals.b }}-{{ .locals.c }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "root", result["a"])
	assert.Equal(t, "root-left", result["b"])
	assert.Equal(t, "root-right", result["c"])
	assert.Equal(t, "root-left-root-right", result["d"])
}

func TestResolver_ComplexChain(t *testing.T) {
	locals := map[string]any{
		"project":     "myapp",
		"environment": "prod",
		"region":      "us-east-1",
		"prefix":      "{{ .locals.project }}-{{ .locals.environment }}",
		"full_prefix": "{{ .locals.prefix }}-{{ .locals.region }}",
		"bucket_name": "{{ .locals.full_prefix }}-assets",
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, "myapp-prod-us-east-1-assets", result["bucket_name"])
}

func TestResolver_UndefinedLocalShowsAvailableLocals(t *testing.T) {
	// Test that when a local references an undefined local,
	// the error message includes the list of available locals.
	// Note: "aaa" and "aab" are alphabetically before "zzz", so they resolve first.
	locals := map[string]any{
		"aaa": "value1",
		"aab": "value2",
		"zzz": "{{ .locals.undefined }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Available locals")
	assert.Contains(t, err.Error(), "aaa")
	assert.Contains(t, err.Error(), "aab")
}

func TestResolver_UndefinedLocalNoAvailableLocals(t *testing.T) {
	// Test error message when no locals are available (shows "(none)").
	locals := map[string]any{
		"bad": "{{ .locals.undefined }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Available locals")
	assert.Contains(t, err.Error(), "(none)")
}

func TestResolver_ComplexCycleDetection(t *testing.T) {
	// Test a more complex cycle: a → b → c → d → b (not back to a).
	locals := map[string]any{
		"a": "{{ .locals.b }}",
		"b": "{{ .locals.c }}",
		"c": "{{ .locals.d }}",
		"d": "{{ .locals.b }}", // Creates cycle b → c → d → b
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
	assert.Contains(t, err.Error(), "→")
}

func TestResolver_CycleWithParentLocalReference(t *testing.T) {
	// Cycle should still be detected even with parent local references.
	parentLocals := map[string]any{
		"parent": "parent-value",
	}
	locals := map[string]any{
		"a": "{{ .locals.parent }}-{{ .locals.b }}",
		"b": "{{ .locals.a }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(parentLocals)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolver_MapWithNestedDependencies(t *testing.T) {
	// Test dependency extraction from deeply nested maps.
	locals := map[string]any{
		"base": "root",
		"config": map[string]any{
			"level1": map[string]any{
				"level2": "{{ .locals.base }}-nested",
			},
		},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	config := result["config"].(map[string]any)
	level1 := config["level1"].(map[string]any)
	assert.Equal(t, "root-nested", level1["level2"])
}

func TestResolver_SliceWithMixedTypes(t *testing.T) {
	// Test slice with mixed types including templates.
	locals := map[string]any{
		"prefix": "item",
		"items": []any{
			"{{ .locals.prefix }}-1",
			42,
			true,
			map[string]any{"name": "{{ .locals.prefix }}-nested"},
		},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	items := result["items"].([]any)
	assert.Equal(t, "item-1", items[0])
	assert.Equal(t, 42, items[1])
	assert.Equal(t, true, items[2])
	nested := items[3].(map[string]any)
	assert.Equal(t, "item-nested", nested["name"])
}

func TestResolver_MapWithMultipleDependencies(t *testing.T) {
	// Test map value with multiple local references for dependency extraction.
	locals := map[string]any{
		"a": "first",
		"b": "second",
		"c": "third",
		"combined": map[string]any{
			"all": "{{ .locals.a }}-{{ .locals.b }}-{{ .locals.c }}",
		},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	combined := result["combined"].(map[string]any)
	assert.Equal(t, "first-second-third", combined["all"])
}

func TestResolver_SliceWithDependencyChain(t *testing.T) {
	// Test slice elements that form a dependency chain.
	locals := map[string]any{
		"base": "root",
		"mid":  "{{ .locals.base }}-mid",
		"items": []any{
			"{{ .locals.mid }}-item1",
			"{{ .locals.base }}-item2",
		},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	items := result["items"].([]any)
	assert.Equal(t, "root-mid-item1", items[0])
	assert.Equal(t, "root-item2", items[1])
}

func TestResolver_FloatAndNilValues(t *testing.T) {
	// Test that float and nil values pass through unchanged.
	locals := map[string]any{
		"float_val": 3.14159,
		"nil_val":   nil,
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, 3.14159, result["float_val"])
	assert.Nil(t, result["nil_val"])
}

func TestResolver_EmptyMapAndSlice(t *testing.T) {
	// Test empty map and slice values.
	locals := map[string]any{
		"empty_map":   map[string]any{},
		"empty_slice": []any{},
	}

	resolver := NewResolver(locals, "test.yaml")
	result, err := resolver.Resolve(nil)

	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, result["empty_map"])
	assert.Equal(t, []any{}, result["empty_slice"])
}

func TestResolver_LargeCycle(t *testing.T) {
	// Test cycle detection with a larger cycle.
	locals := map[string]any{
		"a": "{{ .locals.b }}",
		"b": "{{ .locals.c }}",
		"c": "{{ .locals.d }}",
		"d": "{{ .locals.e }}",
		"e": "{{ .locals.a }}", // Creates cycle a → b → c → d → e → a
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolver_MultipleSelfReferences(t *testing.T) {
	// Test multiple self-referencing locals.
	locals := map[string]any{
		"a": "{{ .locals.a }}",
		"b": "{{ .locals.b }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	_, err := resolver.Resolve(nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestResolver_DependencyGraphWithParentRefs(t *testing.T) {
	// Test that parent local references don't appear in the dependency graph.
	parentLocals := map[string]any{
		"parent1": "p1",
		"parent2": "p2",
	}
	locals := map[string]any{
		"child": "{{ .locals.parent1 }}-{{ .locals.parent2 }}",
	}

	resolver := NewResolver(locals, "test.yaml")
	err := resolver.buildDependencyGraph()
	require.NoError(t, err)

	deps := resolver.GetDependencies()
	// Dependencies include parent refs, but they're not in the local scope.
	assert.Contains(t, deps["child"], "parent1")
	assert.Contains(t, deps["child"], "parent2")

	// Resolve should work because parent locals are provided.
	result, err := resolver.Resolve(parentLocals)
	require.NoError(t, err)
	assert.Equal(t, "p1-p2", result["child"])
}
