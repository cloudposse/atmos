package starlark

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestEvaluateConfigurationMapping(t *testing.T) {
	cases := []struct {
		source string
		want   any
	}{
		{`return ctx.vars.get("stage", "fallback")`, "prod"},
		{`return ctx.vars.get("missing")`, nil},
		{`return ctx.vars.keys()`, []any{"list", "stage"}},
		{`return ctx.metadata.values()`, []any{"platform"}},
		{`return "stage" in ctx.vars`, true},
		{`return len(ctx.vars)`, int64(2)},
		{`return bool(ctx.metadata)`, true},
		{`return ctx.vars["list"]`, []any{int64(1), "two"}},
		{`return json.decode('{"enabled": true}')`, map[string]any{"enabled": true}},
		{`return 18446744073709551615`, uint64(18446744073709551615)},
		{`x = 1`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			result, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: tc.source, Context: staticValueMap{
				"vars": map[string]any{"stage": "prod", "list": []any{1, "two"}}, "metadata": map[string]any{"owner": "platform"},
			}})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result)
		})
	}
}

func TestEvaluateConfigurationInvalidAccess(t *testing.T) {
	cases := []string{
		`return ctx.missing`, `return ctx.vars["missing"]`, `return ctx.vars[1]`,
		`return ctx.vars.get()`, `return ctx.vars.keys(1)`, `return ctx.vars.unknown()`,
		`return {ctx.vars: "key"}`, `ctx.vars["items"].append(1)` + "\nreturn 1",
		`return (1, 2)`, `return set([1])`, `return float('nan')`,
		"d = {}\nd['self'] = d\nreturn d",
	}
	for _, source := range cases {
		t.Run(source, func(t *testing.T) {
			_, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: source, Context: staticValueMap{"vars": map[string]any{"items": []any{1}}}})
			require.Error(t, err)
		})
	}
}

func TestEvaluateConfigurationResultIsolation(t *testing.T) {
	original := map[string]any{"nested": map[string]any{"items": []any{"first", "last"}}}
	value, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: "return ctx.vars", Context: staticValueMap{"vars": original}})
	require.NoError(t, err)
	result := value.(map[string]any)
	outputItems := result["nested"].(map[string]any)["items"].([]any)
	inputItems := original["nested"].(map[string]any)["items"].([]any)
	assert.Equal(t, []any{"first", "last"}, outputItems)
	outputItems[0] = "changed result"
	assert.Equal(t, "first", inputItems[0])
	inputItems[1] = "changed source"
	assert.Equal(t, "last", outputItems[1])
}

func TestEvaluateConfigurationInputTypes(t *testing.T) {
	input := map[string]any{
		"null": nil, "bool": true, "signed": int64(-9007199254740993),
		"unsigned": uint64(18446744073709551615), "float": 1.5, "tag": "!env NOT_AN_EXPRESSION",
	}
	result, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: "return ctx.vars", Context: staticValueMap{"vars": input}})
	require.NoError(t, err)
	assert.Equal(t, input, result)
	for _, value := range []any{make(chan int), []any{make(chan int)}} {
		_, err = New().EvaluateValue(context.Background(), script.Evaluation{Source: "return ctx.vars['value']", Context: staticValueMap{"vars": map[string]any{"value": value}}})
		require.ErrorContains(t, err, "unsupported configuration input")
	}
}

func TestEvaluateConfigurationMappingItems(t *testing.T) {
	input := staticValueMap{
		"vars": map[string]any{"region": "us-east-2", "stage": "prod", "tags": map[string]any{"team": "plat"}},
	}
	cases := []struct {
		name   string
		source string
		want   any
	}{
		{"json encodes an object", `return json.encode(ctx.vars)`, `{"region":"us-east-2","stage":"prod","tags":{"team":"plat"}}`},
		{"json encodes a nested mapping", `return json.encode(ctx.vars["tags"])`, `{"team":"plat"}`},
		{"dict copies key/value pairs", `return dict(ctx.vars)["stage"]`, "prod"},
		{"keyword unpacking", "def pick(region, **rest):\n    return region + \"/\" + rest[\"stage\"]\nreturn pick(**ctx.vars)", "us-east-2/prod"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: tc.source, Context: input})
			require.NoError(t, err)
			assert.Equal(t, tc.want, result)
		})
	}
}

type failingValueMap struct{}

func (failingValueMap) Keys() []string { return []string{"ok", "bad"} }

func (failingValueMap) Get(key string) (any, bool, error) {
	if key == "bad" {
		return nil, false, errFailingValue
	}
	return "fine", true, nil
}

var errFailingValue = errors.New("value cannot be resolved")

func TestEvaluateConfigurationItemsSurfaceErrors(t *testing.T) {
	_, err := New().EvaluateValue(context.Background(), script.Evaluation{
		Source:  `return json.encode(ctx.vars)`,
		Context: staticValueMap{"vars": failingValueMap{}},
	})
	require.ErrorIs(t, err, errFailingValue)
}
