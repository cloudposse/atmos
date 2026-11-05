package starlark

import (
	"context"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestEvaluateConfigurationValue(t *testing.T) {
	cases := []struct {
		name, source string
		want         any
	}{
		{"null", "return None", nil},
		{"string", `return "001"`, "001"},
		{"bool", "return True", true},
		{"integer", "return 9007199254740993", int64(9007199254740993)},
		{"float", "return 1.5", 1.5},
		{"list", "return [1, None, False]", []any{int64(1), nil, false}},
		{"map", `return {"a": 1}`, map[string]any{"a": int64(1)}},
		{"empty list", "return []", []any{}},
		{"empty map", "return {}", map[string]any{}},
		{"helper", "def twice(x):\n    return x * 2\nreturn twice(3)", int64(6)},
		{"early return", "if ctx.vars[\"stage\"] == \"prod\":\n    return 3\nreturn 1", int64(3)},
		{"get default", `return ctx.metadata.get("owner", "platform")`, "platform"},
		{"copy context", "return ctx.vars", map[string]any{"stage": "prod"}},
		{"iteration", "return [key for key in ctx.vars]", []any{"stage"}},
		{"items", "return {k: v for k, v in ctx.vars.items()}", map[string]any{"stage": "prod"}},
		{"numerics", "return sum([round(1.5), 3])", int64(5)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: tc.source, Filename: "stack.yaml", Line: 7, Context: staticValueMap{"vars": map[string]any{"stage": "prod"}, "metadata": map[string]any{}}})
			require.NoError(t, err)
			assert.Equal(t, tc.want, value)
		})
	}
}

func TestEvaluateConfigurationFailures(t *testing.T) {
	for _, code := range []string{
		"return exec.run(['echo'])", "return fs.read_file('secret')", "return steps.input()",
		"load('file.star', 'x')\nreturn x", "ctx.vars['stage'] = 'dev'\nreturn 1",
		"return {1: 'bad'}", "return 1 << 100", "return float('inf')", "return lambda x: x",
		"a = []\na.append(a)\nreturn a", "while True:\n    pass", "",
	} {
		t.Run(code, func(t *testing.T) {
			_, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: code, Filename: "stack.yaml", Context: staticValueMap{"vars": map[string]any{"stage": "prod"}}})
			require.Error(t, err)
		})
	}
}

func TestEvaluateConfigurationSourceLocation(t *testing.T) {
	_, err := New().EvaluateValue(context.Background(), script.Evaluation{Source: "return 1 // 0", Filename: "catalog/service.yaml", Line: 27})
	require.Error(t, err)
	assert.Contains(t, strings.Join(cockroach.GetAllDetails(err), "\n"), "catalog/service.yaml:27")
}

func TestEvaluateConfigurationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New().EvaluateValue(ctx, script.Evaluation{Source: "return 1"})
	require.ErrorIs(t, err, context.Canceled)
}
