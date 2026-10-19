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

func TestTestEffectsMatchAcceptsEmptyListsForNoEffects(t *testing.T) {
	none := Decision{}
	withApproval := Decision{Findings: []Finding{{Effects: Effects{Approvals: []ApprovalRequirement{{ID: "platform"}}}}}}

	require.True(t, testEffectsMatch(none, &TestCase{}), "omitted expectations assert nothing about approvals")
	require.True(t, testEffectsMatch(none, &TestCase{Hooks: []HookRequirement{}, Approvals: []ApprovalRequirement{}}),
		"explicit empty lists assert that a rule requests no hooks or approvals")
	require.False(t, testEffectsMatch(withApproval, &TestCase{Approvals: []ApprovalRequirement{}}),
		"an empty approvals list fails when an approval is requested")
	require.True(t, testEffectsMatch(withApproval, &TestCase{}), "omitted approvals skip the check")
	require.True(t, testEffectsMatch(withApproval, &TestCase{Approvals: []ApprovalRequirement{{ID: "platform"}}}))
}
