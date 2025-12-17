package locals

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
