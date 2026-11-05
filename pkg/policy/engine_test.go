package policy

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	engine := &Engine{}
	require.NoError(t, engine.Register(CELEngine, &CEL{}))
	return engine
}

func testRule(id, expression string, effects Effects) Rule {
	return Rule{ID: id, On: Operation, Engine: CELEngine, Expression: expression, Effects: effects}
}

func testRequest() *Request {
	return &Request{Kind: Operation, Input: json.RawMessage(`{"command":"plan","status":"success"}`)}
}

func TestDecisionEffects(t *testing.T) {
	approval := ApprovalRequirement{ID: "platform", Provider: "github-actions", Spec: json.RawMessage(`{"users":["alice"]}`)}
	hook := HookRequirement{Ref: "lint", Event: "after.terraform.plan", Required: true, RequireSuccess: true}
	cases := []struct {
		name      string
		predicate string
		effects   Effects
		status    DecisionStatus
	}{
		{"allow", "true", Effects{}, Allow},
		{"deny", "true", Effects{Deny: true, Reason: "frozen"}, Deny},
		{"not applicable", "false", Effects{Deny: true}, NotApplicable},
		{"approval", "true", Effects{Approvals: []ApprovalRequirement{approval}}, AwaitingApproval},
		{"required hook", "true", Effects{Hooks: []HookRequirement{hook}}, Allow},
		{"independent obligations", "true", Effects{Deny: true, Approvals: []ApprovalRequirement{approval}, Hooks: []HookRequirement{hook}}, Deny},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := testEngine(t).Evaluate(t.Context(), testRequest(), []Rule{testRule("guard", tc.predicate, tc.effects)})
			require.NoError(t, err)
			assert.Equal(t, tc.status, decision.Status)
			wantEffects := tc.effects
			if tc.status == NotApplicable {
				wantEffects = Effects{}
			}
			assert.Equal(t, []Finding{{RuleID: "guard", Status: tc.status, Effects: wantEffects}}, decision.Findings)
		})
	}
}

func TestDecisionPrecedence(t *testing.T) {
	rules := []Rule{
		testRule("broken", "input.missing", Effects{}),
		testRule("blocked", "true", Effects{Deny: true}),
		testRule("approved-operation", "true", Effects{}),
		testRule("needs-review", "true", Effects{Approvals: []ApprovalRequirement{{ID: "review", Provider: "github-actions", Spec: json.RawMessage(`{}`)}}}),
		testRule("skipped", "false", Effects{Deny: true}),
	}
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "reverse"}[reverse], func(t *testing.T) {
			ordered := append([]Rule(nil), rules...)
			if reverse {
				for left, right := 0, len(ordered)-1; left < right; left, right = left+1, right-1 {
					ordered[left], ordered[right] = ordered[right], ordered[left]
				}
			}
			decision, err := testEngine(t).Evaluate(t.Context(), testRequest(), ordered)
			require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
			assert.Equal(t, EvaluationError, decision.Status)
			byID := map[string]DecisionStatus{}
			for _, finding := range decision.Findings {
				byID[finding.RuleID] = finding.Status
			}
			assert.Equal(t, map[string]DecisionStatus{"broken": EvaluationError, "blocked": Deny, "approved-operation": Allow, "needs-review": AwaitingApproval, "skipped": NotApplicable}, byID)
		})
	}
}

func TestExplicitInputDispatch(t *testing.T) {
	rule := testRule("affected", "input.files.size() > 0", Effects{Deny: true})
	rule.On = AtmosAffected
	engine := testEngine(t)
	decision, err := engine.Evaluate(t.Context(), testRequest(), []Rule{rule})
	require.NoError(t, err)
	assert.Equal(t, Decision{Status: NotApplicable, Findings: []Finding{}}, decision)
	request := &Request{Kind: AtmosAffected, Input: json.RawMessage(`{"complete":true,"files":[{"path":"stack.yaml","action":"modify"}],"components":[],"instances":[],"stacks":[],"changes":[]}`)}
	decision, err = engine.Evaluate(t.Context(), request, []Rule{rule})
	require.NoError(t, err)
	assert.Equal(t, []Finding{{RuleID: "affected", Status: Deny, Effects: rule.Effects}}, decision.Findings)
}

func TestEngineRegistry(t *testing.T) {
	engine := &Engine{}
	require.ErrorIs(t, engine.Register("", &CEL{}), errUtils.ErrPolicyInvalid)
	require.ErrorIs(t, engine.Register("cel", nil), errUtils.ErrPolicyInvalid)
	require.NoError(t, engine.Register("custom", &CEL{}))
	require.ErrorIs(t, engine.Register("custom", &CEL{}), errUtils.ErrPolicyEngineRegistered)
	for _, name := range []string{"custom", "starlark", "rego"} {
		rule := testRule(name, "true", Effects{})
		rule.Engine = name
		decision, err := engine.Evaluate(t.Context(), testRequest(), []Rule{rule})
		if name == "custom" {
			require.NoError(t, err)
			assert.Equal(t, Allow, decision.Status)
		} else {
			require.ErrorIs(t, err, errUtils.ErrPolicyEngineUnknown)
			assert.Equal(t, EvaluationError, decision.Status)
		}
	}
}

func TestInvalidRules(t *testing.T) {
	valid := testRule("guard", "true", Effects{})
	cases := []struct {
		name   string
		change func(*Rule)
	}{
		{"id", func(r *Rule) { r.ID = "" }},
		{"kind", func(r *Rule) { r.On = "" }},
		{"engine", func(r *Rule) { r.Engine = "" }},
		{"expression", func(r *Rule) { r.Expression = " " }},
		{"parameters", func(r *Rule) { r.Parameters = json.RawMessage(`[]`) }},
		{"approval identity", func(r *Rule) { r.Effects.Approvals = []ApprovalRequirement{{}} }},
		{"approval spec", func(r *Rule) { r.Effects.Approvals = []ApprovalRequirement{{ID: "review", Provider: "github"}} }},
		{"hook", func(r *Rule) { r.Effects.Hooks = []HookRequirement{{Ref: "lint"}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := valid
			tc.change(&rule)
			decision, err := testEngine(t).Evaluate(t.Context(), testRequest(), []Rule{rule})
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
			assert.Equal(t, EvaluationError, decision.Status)
		})
	}
	decision, err := testEngine(t).Evaluate(t.Context(), testRequest(), []Rule{valid, valid})
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	assert.Equal(t, EvaluationError, decision.Status)
}

func TestInvalidRequests(t *testing.T) {
	for _, request := range []*Request{nil, {Input: json.RawMessage(`{}`)}, {Kind: Operation}, {Kind: Operation, Input: json.RawMessage(`null`)}} {
		decision, err := testEngine(t).Evaluate(t.Context(), request, nil)
		require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
		assert.Equal(t, EvaluationError, decision.Status)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	decision, err := testEngine(t).Evaluate(ctx, testRequest(), nil)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, EvaluationError, decision.Status)
}

func TestSnapshotIsolation(t *testing.T) {
	adapter := NewMockEvaluator(gomock.NewController(t))
	engine := &Engine{}
	require.NoError(t, engine.Register("mock", adapter))
	request := testRequest()
	request.CI = &CISnapshot{Provider: "github-actions", Facts: map[string]Dataset{"reviews": {Data: json.RawMessage(`[]`)}}}
	rule := testRule("isolated", "true", Effects{Approvals: []ApprovalRequirement{{ID: "review", Provider: "github-actions", Spec: json.RawMessage(`{"users":["alice"]}`)}}})
	rule.Engine = "mock"
	var retained Evaluation
	adapter.EXPECT().Evaluate(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, value Evaluation) (bool, error) {
		retained = value
		value.Request.CI.Provider = "changed"
		value.Request.CI.Facts["reviews"] = Dataset{Data: json.RawMessage(`[1]`)}
		value.Request.Input[0] = ' '
		return true, nil
	})
	decision, err := engine.Evaluate(t.Context(), request, []Rule{rule})
	require.NoError(t, err)
	assert.Equal(t, "github-actions", request.CI.Provider)
	assert.JSONEq(t, `[]`, string(request.CI.Facts["reviews"].Data))
	assert.Equal(t, byte('{'), request.Input[0])
	decision.Findings[0].Effects.Approvals[0].Spec[0] = ' '
	assert.Equal(t, byte('{'), rule.Effects.Approvals[0].Spec[0])
	rule.Effects.Approvals[0].ID = "new-source"
	assert.Equal(t, "review", decision.Findings[0].Effects.Approvals[0].ID)
	request.Input[1] = ' '
	request.CI.Facts["reviews"] = Dataset{Data: json.RawMessage(`[2]`)}
	assert.Equal(t, byte('"'), retained.Request.Input[1])
	assert.JSONEq(t, `[1]`, string(retained.Request.CI.Facts["reviews"].Data))
}

func TestConcurrentEvaluation(t *testing.T) {
	engine := testEngine(t)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			decision, err := engine.Evaluate(t.Context(), testRequest(), []Rule{testRule("guard", "input.command == 'plan'", Effects{})})
			assert.NoError(t, err)
			assert.Equal(t, Allow, decision.Status)
		})
	}
	group.Wait()
}
