package policy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateTerraformPlan(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		valid bool
	}{
		{"empty plan", `{"format_version":"1.2","planned_values":{}}`, true},
		{"future minor", `{"format_version":"1.99","planned_values":{},"future":{"secret":true}}`, true},
		{"invalid", `bad`, false},
		{"state", `{"format_version":"1.0","values":{}}`, false},
		{"no format", `{"planned_values":{}}`, false},
		{"new major", `{"format_version":"2.0","planned_values":{}}`, false},
		{"invalid version", `{"format_version":"1.bad","planned_values":{}}`, false},
		{"wrong planned type", `{"format_version":"1.2","planned_values":[]}`, false},
		{"errored", `{"format_version":"1.2","planned_values":{},"errored":true}`, false},
		{"incomplete", `{"format_version":"1.2","planned_values":{},"complete":false}`, false},
		{"wrong flag type", `{"format_version":"1.2","planned_values":{},"complete":null}`, false},
		{"deferred", `{"format_version":"1.2","planned_values":{},"deferred_changes":[{}]}`, false},
		{"wrong deferred type", `{"format_version":"1.2","planned_values":{},"deferred_changes":{}}`, false},
		{"complete", `{"format_version":"1.2","planned_values":{},"complete":true,"errored":false,"deferred_changes":[]}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTerraformPlan(json.RawMessage(tt.data))
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestTerraformPlanNativeFields(t *testing.T) {
	var engine Engine
	require.NoError(t, engine.Register(CELEngine, &CEL{}))
	request := &Request{Kind: TerraformPlan, Input: json.RawMessage(`{
		"format_version":"1.2","planned_values":{},
		"resource_changes":[{"address":"aws_db_instance.main","type":"aws_db_instance",
		 "change":{"actions":["delete","create"],"before":{"tag":"old"},"after":{"tag":null},
		 "after_unknown":{"tag":true},"after_sensitive":{"password":true}}}],
		"future":{"flag":true}
	}`)}
	rules := []Rule{{ID: "native", On: TerraformPlan, Engine: CELEngine, Expression: `
		input.future.flag && input.resource_changes.exists(r,
		  r.type == 'aws_db_instance' && r.change.actions == ['delete', 'create'] &&
		  r.change.before.tag == 'old' && r.change.after.tag == null &&
		  r.change.after_unknown.tag && r.change.after_sensitive.password)`, Effects: Effects{Deny: true}}}
	decision, err := engine.Evaluate(context.Background(), request, rules)
	require.NoError(t, err)
	require.Equal(t, Deny, decision.Status)
}

func TestEnforceDecision(t *testing.T) {
	for _, status := range []DecisionStatus{Allow, NotApplicable} {
		require.NoError(t, EnforceDecision(Decision{Status: status}))
	}
	require.ErrorIs(t, EnforceDecision(Decision{Status: Deny}), errUtils.ErrPolicyDenied)
	require.ErrorIs(t, EnforceDecision(Decision{Status: EvaluationError}), errUtils.ErrPolicyEvaluation)
	for _, status := range []DecisionStatus{AwaitingApproval, "", "future"} {
		require.ErrorIs(t, EnforceDecision(Decision{Status: status}), errUtils.ErrPolicyRequirements)
	}
	for _, finding := range []Finding{
		{Status: Deny},
		{Status: Allow, Effects: Effects{Hooks: []HookRequirement{{Ref: "lint"}}}},
		{Status: Allow, Effects: Effects{Approvals: []ApprovalRequirement{{ID: "review"}}}},
	} {
		require.ErrorIs(t, EnforceDecision(Decision{Status: Allow, Findings: []Finding{finding}}), errUtils.ErrPolicyRequirements)
	}
	require.NoError(t, EnforceDecision(Decision{Status: NotApplicable, Findings: []Finding{{Status: NotApplicable}}}))
}
