package policy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestDenialReasons(t *testing.T) {
	for _, engine := range []string{CELEngine, StarlarkEngine} {
		t.Run(engine, func(t *testing.T) {
			bundle, err := LoadDirectory("../../policy-library/function-disable-exec")
			require.NoError(t, err)
			rule := bundle.Rules[0]
			rule.Engine = engine
			if engine == StarlarkEngine {
				rule.Expression = `return ctx.input["name"] == "exec"`
			}
			request := &Request{Kind: Function, Input: json.RawMessage(`{"name":"exec","secret":"private-input"}`)}
			decision, err := NewEngine().Evaluate(context.Background(), request, []Rule{rule})
			require.NoError(t, err)
			err = EnforceDecision(decision)
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			output := strings.Join(strings.Fields(errUtils.Format(err, errUtils.DefaultFormatterConfig())), " ")
			require.Contains(t, output, rule.ID)
			require.Contains(t, output, "stack configuration must not run shell commands")
			require.Contains(t, output, "Pass precomputed values through environment variables")
			require.NotContains(t, output, "private-input")
			encoded, err := json.Marshal(decision)
			require.NoError(t, err)
			require.Contains(t, string(encoded), rule.Effects.Reason)

			request.Input = json.RawMessage(`{"name":"env"}`)
			decision, err = NewEngine().Evaluate(context.Background(), request, []Rule{rule})
			require.NoError(t, err)
			require.NoError(t, EnforceDecision(decision))
			require.Empty(t, decision.Findings[0].Effects.Reason)
		})
	}
}

func TestDenialReasonIsOptional(t *testing.T) {
	for _, reason := range []string{"", " \n\t"} {
		decision := Decision{Status: Deny, Findings: []Finding{{
			RuleID: "optional", Status: Deny, Effects: Effects{Deny: true, Reason: reason},
		}}}
		require.Equal(t, errUtils.ErrPolicyDenied, EnforceDecision(decision))
	}
}

func TestDenialReasonsPreserveAllBlockingRules(t *testing.T) {
	decision := Decision{Status: Deny, Findings: []Finding{
		{RuleID: "first", Status: Deny, Effects: Effects{Reason: "Use !env instead."}},
		{RuleID: "second", Status: Deny, Effects: Effects{Reason: "Static {{ .env.SECRET }} text."}},
		{RuleID: "skipped", Status: NotApplicable, Effects: Effects{Reason: "Do not display."}},
	}}
	err := EnforceDecision(decision)
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	output := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	require.Contains(t, output, "first:")
	require.Contains(t, output, "Use !env instead.")
	require.Contains(t, output, "second:")
	require.Contains(t, output, "{{ .env.SECRET }}")
	require.NotContains(t, output, "Do not display")
	decision.Status = EvaluationError
	require.Equal(t, errUtils.ErrPolicyEvaluation, EnforceDecision(decision))
}
