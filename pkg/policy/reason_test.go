package policy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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
			var decoded Decision
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, rule.Effects.Reason, decoded.Findings[0].Effects.Reason)

			request.Input = json.RawMessage(`{"name":"env"}`)
			decision, err = NewEngine().Evaluate(context.Background(), request, []Rule{rule})
			require.NoError(t, err)
			require.NoError(t, EnforceDecision(decision))
			require.Empty(t, decision.Findings[0].Effects.Reason)
		})
	}
}

func TestDenialReasonMarkdown(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("ATMOS_FORCE_COLOR", "true")
	t.Setenv("ATMOS_FORCE_TTY", "true")
	reason := "**Shell execution is disabled.**\n\nInstead:\n\n- Read values with `!env`.\n- Use a [reviewed workflow](https://atmos.tools/workflows)."
	err := EnforceDecision(Decision{Status: Deny, Findings: []Finding{{
		RuleID: "guard", Status: Deny, Effects: Effects{Reason: reason},
	}}})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	output := errUtils.Format(err, errUtils.DefaultFormatterConfig())
	require.Contains(t, output, "\x1b[", "TTY output should retain Markdown styling")
	plain := ansi.Strip(output)
	require.Contains(t, plain, "Shell execution is disabled.")
	require.NotContains(t, plain, "**Shell execution is disabled.**")
	require.Contains(t, plain, "!env")
	require.NotContains(t, plain, "`!env`")
	require.Contains(t, plain, "reviewed workflow")
	require.Contains(t, plain, "https://atmos.tools/workflows")
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
