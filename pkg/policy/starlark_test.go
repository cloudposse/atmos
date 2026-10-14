package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestStarlarkPredicates(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"match", `return ctx.input["command"] == "plan"`, true},
		{"no match", `return ctx.input["command"] == "apply"`, false},
		{"parameter", `return ctx.params["allowed"] == ctx.input["command"]`, true},
		{"optional", `return ctx.input.get("missing") == None`, true},
		{"no CI", `return ctx.ci == None`, true},
		{"kind", `return ctx.request["kind"] == "operation"`, true},
		{"large integers", `return ctx.params["id"] == 9007199254740993 and ctx.params["unsigned"] == 18446744073709551615`, true},
		{"helpers", "def count(items):\n    return len([x for x in items if x > 2])\nreturn count([1, 2, 3]) == 1", true},
		{"aggregation", "total = 0\nfor n in [1, 2, 3]:\n    total += n\nreturn total == 6", true},
		{"recursion", "def count(n):\n    if n == 0:\n        return 0\n    return 1 + count(n - 1)\nreturn count(5) == 5", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matched, err := (&Starlark{}).Evaluate(t.Context(), Evaluation{
				Expression: tc.source, Request: testRequest(),
				Parameters: json.RawMessage(`{"allowed":"plan","id":9007199254740993,"unsigned":18446744073709551615}`),
			})
			require.NoError(t, err)
			require.Equal(t, tc.want, matched)
		})
	}
}

func TestStarlarkIsolationAndErrors(t *testing.T) {
	for _, source := range []string{
		`return exec.run(["echo"])`, `return fs.read_file("secret")`, `return atmos.run("version")`,
		`return steps.input()`, `return ci.summary("secret")`, "load('local.star', 'x')\nreturn x",
		`print(ctx.input["secret"])`, `fail(ctx.input["secret"])`, `return ctx.input["missing"]`,
		`ctx.input["secret"] = "changed"`, `ctx.input["items"].append("changed")`,
		`return "true"`, `return 1`, `return None`, `return []`, `x = True`, `return lambda x: x`,
		`return ctx.input["secret"] / 2`, `return unknown_secret_name`, `return (`, "",
	} {
		t.Run(source, func(t *testing.T) {
			request := &Request{Kind: Operation, Input: json.RawMessage(`{"secret":"sensitive-test-value","items":[1]}`)}
			original := string(request.Input)
			matched, err := (&Starlark{}).Evaluate(t.Context(), Evaluation{Expression: source, Request: request})
			require.Error(t, err)
			require.False(t, matched)
			require.NotContains(t, fmt.Sprintf("%+v", err), "sensitive-test-value")
			require.NotContains(t, fmt.Sprintf("%+v", err), "unknown_secret_name")
			require.Equal(t, original, string(request.Input))
		})
	}
}

func TestStarlarkLimits(t *testing.T) {
	for _, source := range []string{"while True:\n    pass", "def recurse():\n    return recurse()\nreturn recurse()"} {
		matched, err := (&Starlark{}).Evaluate(t.Context(), Evaluation{Expression: source, Request: testRequest()})
		require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
		require.False(t, matched)
	}
	_, err := (&Starlark{}).Evaluate(t.Context(), Evaluation{Expression: strings.Repeat(" ", maxExpressionBytes+1), Request: testRequest()})
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = (&Starlark{}).Evaluate(ctx, Evaluation{Expression: "return True", Request: testRequest()})
	require.ErrorIs(t, err, context.Canceled)
	_, err = (&Starlark{}).Evaluate(t.Context(), Evaluation{Expression: "return True", Request: testRequest(), Parameters: json.RawMessage(`[]`)})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
}

func TestStarlarkConcurrentSnapshots(t *testing.T) {
	t.Parallel()

	adapter := &Starlark{}
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			request := testRequest()
			request.CI = &CISnapshot{Provider: "github-actions", Metadata: Dataset{Data: json.RawMessage(`{"GITHUB_RUN_ID":"123"}`), Status: Available, Complete: true}}
			matched, err := adapter.Evaluate(t.Context(), Evaluation{
				Request:    request,
				Expression: `return ctx.ci["metadata"]["data"]["GITHUB_RUN_ID"] == "123" and ctx.params["id"] == ` + fmt.Sprint(i),
				Parameters: json.RawMessage(fmt.Sprintf(`{"id":%d}`, i)),
			})
			require.NoError(t, err)
			require.True(t, matched)
		})
	}
}

func TestStarlarkManifestAndMixedEngines(t *testing.T) {
	directory := t.TempDir()
	document := `apiVersion: atmos/v1
kind: Policy
metadata: {name: mixed}
spec:
  policies:
    cel:
      on: operation
      rule: "false"
      action: deny
    starlark:
      on: operation
      engine: starlark
      rule: |
        return ctx.input["command"] == "plan"
      action: deny
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(document), 0o600))
	bundle, err := LoadDirectory(directory)
	require.NoError(t, err)
	decision, err := NewEngine().Evaluate(t.Context(), testRequest(), bundle.Rules)
	require.NoError(t, err)
	require.Equal(t, Deny, decision.Status)
	require.Equal(t, NotApplicable, decision.Findings[0].Status)
	require.Equal(t, Deny, decision.Findings[1].Status)
	for _, source := range []string{"return fs.read_file('x')", "return (", ""} {
		_, err := compileStarlark(source, nil)
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	}
}

func TestStarlarkEffectsAndRequiredFacts(t *testing.T) {
	rule := Rule{
		ID: "test", On: Operation, Engine: StarlarkEngine, Expression: "return True",
		Effects: Effects{Hooks: []HookRequirement{{Ref: "lint", Event: "after.terraform.plan", Required: true}}},
	}
	decision, err := NewEngine().Evaluate(t.Context(), testRequest(), []Rule{rule})
	require.NoError(t, err)
	require.Equal(t, rule.Effects, decision.Findings[0].Effects)
	require.ErrorIs(t, EnforceDecision(decision), errUtils.ErrPolicyRequirements)
	rule.Expression = "return False"
	rule.RequiredCI = []CIRequirement{{Provider: "github-actions", Schema: "github-actions/v1", Dataset: "event"}}
	decision, err = NewEngine().Evaluate(t.Context(), testRequest(), []Rule{rule})
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	require.Equal(t, EvaluationError, decision.Status)
}
