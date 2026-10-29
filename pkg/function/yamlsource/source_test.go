package yamlsource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSourceCloningAndOutput(t *testing.T) {
	original := Source{Tag: "!env", Argument: "NAME", File: "stack.yaml", Line: 5, Column: 8}
	values := map[string]any{"tagged": original.Encode(), "literal": original.Expression()}
	encoded, err := json.Marshal(values)
	require.NoError(t, err)
	var cloned map[string]any
	require.NoError(t, json.Unmarshal(encoded, &cloned))
	source, ok := Decode(cloned["tagged"].(string))
	require.True(t, ok)
	require.Equal(t, original, source)
	_, ok = Decode(cloned["literal"].(string))
	require.False(t, ok)

	var node yaml.Node
	require.NoError(t, node.Encode(cloned))
	RestoreTags(&node)
	output, err := yaml.Marshal(&node)
	require.NoError(t, err)
	require.Contains(t, string(output), `tagged: !env "NAME"`)
	require.Contains(t, string(output), "literal: '!env NAME'")
	require.NotContains(t, string(output), "atmos-yaml-source:")
}

func TestAuthoredStringsCannotImpersonateSource(t *testing.T) {
	for _, value := range []string{"!env NAME", "!atmos-yaml-source:old-process:eyJ0YWciOiIhZW52In0:NAME", prefix + "invalid:NAME"} {
		_, ok := Decode(value)
		require.False(t, ok)
	}
}

func TestContainsInterpolatedSource(t *testing.T) {
	source := (Source{Tag: "!env", Argument: "STAGE"}).Encode()
	for _, value := range []string{source, "prefix-" + source + "-suffix"} {
		require.True(t, Contains(value))
	}
	for _, value := range []string{"dev", "!env STAGE", prefix + "invalid"} {
		require.False(t, Contains(value))
	}
}
