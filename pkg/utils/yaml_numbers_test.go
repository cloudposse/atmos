package utils

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/yaml/scalar"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func TestExactNumberYAMLRoundTrip(t *testing.T) {
	want := map[string]any{"big": scalar.Number("18446744073709551617"), "list": []any{scalar.Number("1e+500"), "1e+500", nil, 1.75}}
	b, err := yaml.Marshal(want)
	require.NoError(t, err)
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal(b, &node))
	var got map[string]any
	require.NoError(t, decodeVariableNumbers(&node, &got))
	require.Equal(t, want, got)
	// A second decode of the same tree must return the same values.
	var untyped any
	require.NoError(t, decodeVariableNumbers(&node, &untyped))
	require.Equal(t, want, untyped)
}

func TestExactNumberAliasesAndErrors(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("base: &base {n: !!int 18446744073709551617}\ncopy: {<<: *base}\nother: {1: !!int 18446744073709551617}\n"), &node))
	var data map[string]any
	require.NoError(t, decodeVariableNumbers(&node, &data))
	require.Equal(t, data["base"], data["copy"])
	require.Equal(t, map[any]any{1: scalar.Number("18446744073709551617")}, data["other"])
	require.NoError(t, yaml.Unmarshal([]byte("n: !!int 18446744073709551617\nn: 1\n"), &node))
	require.ErrorContains(t, decodeVariableNumbers(&node, &data), "already defined")
	require.NoError(t, yaml.Unmarshal([]byte("n: &n [*n, !!int 18446744073709551617]\n"), &node))
	require.Error(t, decodeVariableNumbers(&node, &data))
	require.NoError(t, yaml.Unmarshal([]byte("[!!int 18446744073709551617, !!float 1.25, !!float .inf]\n"), &node))
	var list []any
	require.NoError(t, decodeVariableNumbers(&node, &list))
	require.Equal(t, scalar.Number("18446744073709551617"), list[0])
	require.NoError(t, yaml.Unmarshal([]byte("42"), &node))
	var integer int
	require.NoError(t, decodeVariableNumbers(&node, &integer))
	require.Equal(t, 42, integer)
}

func TestExactNumberUnrelatedValues(t *testing.T) {
	for _, tc := range []struct{ value, tag string }{
		{"", "!!int"}, {"null", "!!int"}, {"1.75", "!!float"}, {"0xff", "!!int"}, {"1e+999", "!!str"},
	} {
		n := yaml.Node{Kind: yaml.ScalarNode, Style: yaml.TaggedStyle, Tag: tc.tag, Value: tc.value}
		require.False(t, isExactNumber(&n))
	}
	var node yaml.Node
	var value any
	require.NoError(t, yaml.Unmarshal([]byte("null"), &node))
	require.NoError(t, decodeVariableNumbers(&node, &value))
	require.Nil(t, value)
}
