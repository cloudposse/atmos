package yaml

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	yamlv3 "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSetJSONPreservesValuesAndComments(t *testing.T) {
	// The shared yq evaluator is serialized internally.
	value := map[string]any{
		"version": ">=1.0.0 <2.0.0",
		"config": map[string]any{
			"multiline": "first\nsecond",
			"literal":   `\u003e & "quotes"`,
			"unicode":   "OIDC 🔑",
			"code":      `") | .unrelated = "changed" | ("`,
			"enabled":   true,
			"scopes":    []any{"openid", "profile"},
		},
	}
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	result, err := SetJSON([]byte("# retain this comment\nunrelated: original\n"), "plugin", raw)
	require.NoError(t, err)
	require.Contains(t, string(result), "# retain this comment")
	var parsed map[string]any
	require.NoError(t, yamlv3.Unmarshal(result, &parsed))
	require.Equal(t, value, parsed["plugin"])
	require.Equal(t, "original", parsed["unrelated"])
}

func TestSetJSONRejectsExpressionsAndMalformedInput(t *testing.T) {
	for _, value := range []string{`load("/outside")`, `{invalid`, `{"value":1} | .other = 1`} {
		result, err := SetJSON([]byte("keep: true\n"), "value", []byte(value))
		require.ErrorIs(t, err, errUtils.ErrInvalidArgumentError)
		require.Nil(t, result)
	}
	result, err := SetJSON(nil, "items", []byte(`["first","last"]`))
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, yamlv3.Unmarshal(result, &parsed))
	require.Equal(t, []any{"first", "last"}, parsed["items"])
}
