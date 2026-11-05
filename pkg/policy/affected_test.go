package policy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProtectedLabelChanges(t *testing.T) {
	identity := ObjectIdentity{Name: "vpc", ComponentType: "terraform", Stack: "prod"}
	for _, action := range []ChangeAction{Added, Modified, Removed} {
		t.Run(string(action), func(t *testing.T) {
			change := FieldChange{Identity: identity, Path: "/metadata/labels/compliance", Action: action}
			if action != Added {
				change.BeforePresent, change.Before = true, json.RawMessage(`"pci"`)
			}
			if action != Removed {
				change.AfterPresent, change.After = true, json.RawMessage(`null`)
			}
			data, err := json.Marshal(AffectedInput{
				Complete: true, Reasons: []string{},
				Files:      []FileChange{{Path: "stack.yaml", Action: Modified}},
				Components: []ObjectChange{}, Stacks: []ObjectChange{},
				Instances: []ObjectChange{{Identity: identity, Action: Modified, Reasons: []string{"stack configuration"}}},
				Changes:   []FieldChange{change},
			})
			require.NoError(t, err)
			rule := testRule("protected-label", "input.complete && input.changes.exists(c, c.path == '/metadata/labels/compliance' && c.action in ['modify', 'remove'])", Effects{Approvals: []ApprovalRequirement{{ID: "platform", Provider: "github-actions", Spec: json.RawMessage(`{"users":["alice"]}`)}}})
			rule.On = AtmosAffected
			decision, err := testEngine(t).Evaluate(t.Context(), &Request{Kind: AtmosAffected, Input: data}, []Rule{rule})
			require.NoError(t, err)
			if action == Added {
				assert.Equal(t, NotApplicable, decision.Status)
			} else {
				assert.Equal(t, AwaitingApproval, decision.Status)
				assert.Equal(t, rule.Effects.Approvals, decision.Findings[0].Effects.Approvals)
			}
			var decoded AffectedInput
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, []FieldChange{change}, decoded.Changes)
			assert.Equal(t, []ObjectChange{{Identity: identity, Action: Modified, Reasons: []string{"stack configuration"}}}, decoded.Instances)
		})
	}
}

func TestIncompleteAffectedFailsClosed(t *testing.T) {
	for _, input := range []string{
		`{"complete":false,"files":[],"components":[],"instances":[],"stacks":[],"changes":[]}`,
		`{"complete":true,"files":[]}`,
		`{"complete":true,"files":[],"components":[],"instances":null,"stacks":[],"changes":[]}`,
	} {
		rule := testRule("affected", "false", Effects{})
		rule.On = AtmosAffected
		decision, err := testEngine(t).Evaluate(t.Context(), &Request{Kind: AtmosAffected, Input: json.RawMessage(input)}, []Rule{rule})
		require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		assert.Equal(t, EvaluationError, decision.Status)
	}
	require.ErrorIs(t, validateAffectedInput(json.RawMessage(`{"files":true}`)), errUtils.ErrPolicyInputInvalid)
}
