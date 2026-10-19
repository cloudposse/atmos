package policy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSameJSONIgnoresObjectKeyOrder(t *testing.T) {
	require.True(t, sameJSON(json.RawMessage(`{"reviewers":["alice"],"minimum":1}`), map[string]any{"minimum": 1, "reviewers": []any{"alice"}}),
		"a fixture in the documented order matches an approval spec built from a map")
	require.True(t, sameJSON(json.RawMessage(`{"a":{"y":2,"x":1}}`), json.RawMessage(`{"a":{"x":1,"y":2}}`)), "nested objects too")
	require.False(t, sameJSON(json.RawMessage(`{"minimum":1}`), json.RawMessage(`{"minimum":2}`)), "different values differ")
	require.False(t, sameJSON(json.RawMessage(`["a","b"]`), json.RawMessage(`["b","a"]`)), "array order still matters")
	require.False(t, sameJSON(json.RawMessage(`null`), json.RawMessage(`[]`)), "null and an empty list stay distinct")
	require.False(t, sameJSON(json.RawMessage(`{`), json.RawMessage(`{`)), "undecodable values never match")
}
