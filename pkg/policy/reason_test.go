package policy

import (
	"context"
	"encoding/json"
	"fmt"
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
	t.Setenv("NO_COLOR", "1")
	for _, reason := range []string{"", " \n\t"} {
		decision := Decision{Status: Deny, Findings: []Finding{{
			RuleID: "command-deny-destroy/guard", Status: Deny, Effects: Effects{Deny: true, Reason: reason},
		}}}
		err := EnforceDecision(decision)
		require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
		output := strings.Join(strings.Fields(errUtils.Format(err, errUtils.DefaultFormatterConfig())), " ")
		require.Contains(t, output, "Denied by command-deny-destroy/guard.", "a reason-less denial must still name its rule")
	}
}

func TestDenialNamesEveryDenyingRule(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	err := EnforceDecision(Decision{Status: Deny, Findings: []Finding{
		{RuleID: "first/guard", Status: Deny, Effects: Effects{Deny: true}},
		{RuleID: "allowed/rule", Status: Allow},
		{RuleID: "second/guard", Status: Deny, Effects: Effects{Deny: true, Reason: "Use the reviewed workflow."}},
		{RuleID: "third/guard", Status: Deny, Effects: Effects{Deny: true}},
	}})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	output := strings.Join(strings.Fields(errUtils.Format(err, errUtils.DefaultFormatterConfig())), " ")
	require.Contains(t, output, "Denied by first/guard.")
	require.Contains(t, output, "second/guard: Use the reviewed workflow.")
	require.Contains(t, output, "Denied by third/guard.")
	require.NotContains(t, output, "allowed/rule", "rules that did not deny must not be named")
}

func TestDenialExitCode(t *testing.T) {
	t.Run("denial carries the dedicated exit code", func(t *testing.T) {
		for _, reason := range []string{"", "Because."} {
			err := EnforceDecision(Decision{Status: Deny, Findings: []Finding{{
				RuleID: "guard", Status: Deny, Effects: Effects{Deny: true, Reason: reason},
			}}})
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Equal(t, errUtils.ExitCodePolicyDenied, errUtils.GetExitCode(err))
			wrapped := fmt.Errorf("%w: wrapped by a caller", err)
			require.ErrorIs(t, wrapped, errUtils.ErrPolicyDenied)
			require.Equal(t, errUtils.ExitCodePolicyDenied, errUtils.GetExitCode(wrapped))
		}
	})
	t.Run("non-denial outcomes keep the default exit code", func(t *testing.T) {
		for _, decision := range []Decision{
			{Status: EvaluationError},
			{Status: AwaitingApproval},
		} {
			err := EnforceDecision(decision)
			require.Error(t, err)
			require.NotErrorIs(t, err, errUtils.ErrPolicyDenied)
			require.Equal(t, 1, errUtils.GetExitCode(err))
		}
	})
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

func TestDenialViolations(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	err := EnforceDecision(Decision{Status: Deny, Findings: []Finding{
		{RuleID: "allow-plan", Status: Allow},
		{RuleID: "protect-eip/guard", Status: Deny, Effects: Effects{Deny: true, Reason: "  EIPs cannot be destroyed.  "}},
		{RuleID: "no-destroy/guard", Status: Deny, Effects: Effects{Deny: true}},
	}})
	require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
	require.Equal(t, errUtils.ExitCodePolicyDenied, errUtils.GetExitCode(err))

	want := []Violation{
		{RuleID: "protect-eip/guard", Reason: "EIPs cannot be destroyed."},
		{RuleID: "no-destroy/guard"},
	}
	require.Equal(t, want, Violations(err))

	// Wrapping keeps the violations reachable, and callers receive a copy.
	wrapped := fmt.Errorf("plan failed: %w", err)
	got := Violations(wrapped)
	require.Equal(t, want, got)
	got[0].RuleID = "mutated"
	require.Equal(t, want, Violations(err))

	// Carrying violations leaves the rendered explanation intact.
	output := strings.Join(strings.Fields(errUtils.Format(wrapped, errUtils.DefaultFormatterConfig())), " ")
	require.Contains(t, output, "protect-eip/guard: EIPs cannot be destroyed.")
	require.Contains(t, output, "Denied by no-destroy/guard.")
}

func TestViolationsIgnoresOtherErrors(t *testing.T) {
	require.Nil(t, Violations(nil))
	require.Nil(t, Violations(errUtils.ErrPolicyDenied))
	require.Nil(t, Violations(EnforceDecision(Decision{Status: EvaluationError})))
	require.Nil(t, Violations(EnforceDecision(Decision{Status: AwaitingApproval})))
}
