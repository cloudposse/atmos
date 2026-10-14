package deferred

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandStarlarkEvaluationPaths(t *testing.T) {
	data := map[string]any{"vars": map[string]any{
		"name":    `!starlark return ctx.vars["stage"]`,
		"dynamic": `!starlark return dict(ctx.vars)`,
		"stage":   "prod",
	}}
	// A statically bounded read adds exactly the field it reads.
	assert.ElementsMatch(t, [][]string{{"vars", "name"}, {"vars", "stage"}}, ExpandEvaluationPaths(data, [][]string{{"vars", "name"}}))
	assert.Equal(t, [][]string{{"vars", "stage"}}, ExpandEvaluationPaths(data, [][]string{{"vars", "stage"}}))
	// A read that cannot be bounded keeps the full rendered context.
	assert.Nil(t, ExpandEvaluationPaths(data, [][]string{{"vars", "dynamic"}}))
}

func TestStarlarkReferences(t *testing.T) {
	for _, tc := range []struct {
		name    string
		source  string
		want    [][]string
		bounded bool
	}{
		{"no ctx", `return 1 + 2`, nil, true},
		{"attribute read", `return ctx.vars.stage`, [][]string{{"vars", "stage"}}, true},
		{"subscript read", `return ctx.vars["stage"]`, [][]string{{"vars", "stage"}}, true},
		{"single quoted subscript", `return ctx.settings['region']`, [][]string{{"settings", "region"}}, true},
		{"nested attribute and subscript", `return ctx.settings.network["cidr"].block`, [][]string{{"settings", "network", "cidr", "block"}}, true},
		{
			"metadata env and locals", `return [ctx.metadata.name, ctx.env["HOME"], ctx.locals.prefix]`,
			[][]string{{"metadata", "name"}, {"env", "HOME"}, {"locals", "prefix"}},
			true,
		},
		{"get with a literal key", `return ctx.vars.get("stage", "dev")`, [][]string{{"vars", "stage"}}, true},
		{"get on a nested mapping", `return ctx.settings.network.get("cidr")`, [][]string{{"settings", "network", "cidr"}}, true},
		{"identity fields need no configuration", `return ctx.stack + ctx.component + ctx.component_type`, nil, true},
		{
			"several reads", `
def label():
    return ctx.vars["stage"] + "-" + ctx.vars["region"]
return label() if ctx.settings.enabled else "off"`,
			[][]string{{"vars", "stage"}, {"vars", "region"}, {"settings", "enabled"}},
			true,
		},

		{"whole section through dict", `return dict(ctx.vars)`, nil, false},
		{"whole section through json.encode", `return json.encode(ctx.vars)`, nil, false},
		{"whole section returned", `return ctx.vars`, nil, false},
		{"iteration over a section", `return [k for k in ctx.vars]`, nil, false},
		{"alias of a section", "v = ctx.vars\nreturn v.stage", nil, false},
		{"alias of ctx", "c = ctx\nreturn c.vars.stage", nil, false},
		{"ctx passed to a function", "def f(c):\n    return c.vars.stage\nreturn f(ctx)", nil, false},
		{"computed key", `return ctx.vars[ctx.stack]`, nil, false},
		{"get with a computed key", "key = \"stage\"\nreturn ctx.vars.get(key)", nil, false},
		{"get with a keyword key", `return ctx.vars.get(key="stage")`, nil, false},
		{"get with a non literal after literal reads", `return ctx.vars.get("a") + ctx.vars.get(name)`, nil, false},
		{"items", `return ctx.vars.items()`, nil, false},
		{"keys", `return ctx.settings.keys()`, nil, false},
		{"values", `return ctx.vars.values()`, nil, false},
		{"method on a nested mapping", `return ctx.settings.tags.items()`, nil, false},
		{"unknown ctx field", `return ctx.other`, nil, false},
		{"ctx subscript with a literal that is not a section", `return ctx["other"]`, nil, false},
		{"does not parse", `return (`, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, bounded := StarlarkReferences("!starlark " + tc.source)
			assert.Equal(t, tc.bounded, bounded)
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}

func TestStarlarkReferencesAcceptLoaderEncodedSource(t *testing.T) {
	encoded := "!starlark\n# atmos-starlark-source: eyJmaWxlIjoiYS55YW1sIiwibGluZSI6M30\nreturn ctx.vars.stage"
	got, bounded := StarlarkReferences(encoded)
	require.True(t, bounded)
	assert.Equal(t, [][]string{{"vars", "stage"}}, got)
}

// A reference found in a Starlark value is followed like any other: the field it reads may itself
// be a template or another Starlark value.
func TestExpandEvaluationPathsFollowsStarlarkReferences(t *testing.T) {
	data := map[string]any{
		"settings": map[string]any{"note": `!starlark return ctx.vars["derived"]`},
		"vars": map[string]any{
			"derived": `!starlark return ctx.vars.base + "-x"`,
			"base":    "{{ .settings.region }}",
			"unused":  "ignored",
		},
	}
	paths := ExpandEvaluationPaths(data, [][]string{{"settings", "note"}})
	assert.ElementsMatch(t, [][]string{{"settings", "note"}, {"vars", "derived"}, {"vars", "base"}, {"settings", "region"}}, paths)
}
