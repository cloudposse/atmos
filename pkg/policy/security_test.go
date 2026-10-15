package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/policy/guard"
)

func TestSourceRootConstraints(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	require.NoError(t, os.Mkdir(allowed, 0o700))
	inside := filepath.Join(allowed, "data.txt")
	outside := filepath.Join(root, "private.txt")
	require.NoError(t, os.WriteFile(inside, []byte("allowed"), 0o600))
	require.NoError(t, os.WriteFile(outside, []byte("private"), 0o600))
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: filesystem}
spec:
  policies:
    confine:
      on: source
      allowed_roots: [allowed]
      action: deny
      rule: "input.scheme == 'file' && !input.within_allowed_roots"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "policy.yaml"), []byte(manifest), 0o600))
	bundle, err := LoadDirectory(root)
	require.NoError(t, err)
	for _, tc := range []struct {
		path string
		want DecisionStatus
	}{{inside, NotApplicable}, {outside, Deny}, {filepath.Join(allowed, "..", "private.txt"), Deny}, {filepath.Join(allowed, "missing"), Deny}} {
		input, err := guard.DescribeSource("include", tc.path, "")
		require.NoError(t, err)
		raw, err := json.Marshal(input)
		require.NoError(t, err)
		decision, err := NewEngine().Evaluate(t.Context(), &Request{Kind: Source, Input: raw}, bundle.Rules)
		require.NoError(t, err)
		require.Equal(t, tc.want, decision.Status)
	}
	link := filepath.Join(allowed, "escape")
	if err := os.Symlink(outside, link); err == nil {
		input, err := guard.DescribeSource("include", link, "")
		require.NoError(t, err)
		raw, _ := json.Marshal(input)
		decision, err := NewEngine().Evaluate(t.Context(), &Request{Kind: Source, Input: raw}, bundle.Rules)
		require.NoError(t, err)
		require.Equal(t, Deny, decision.Status)
	}
}

func TestExplainPreservesDecisions(t *testing.T) {
	var events []Explanation
	rules := []Rule{
		{ID: "deny", On: Command, Engine: CELEngine, Expression: "true", Effects: Effects{Deny: true}},
		{ID: "later", On: TerraformPlan, Engine: CELEngine, Expression: "true"},
	}
	request := &Request{Kind: Command, Input: json.RawMessage(`{"command":"secret-input"}`)}
	baseline, err := NewEngine().Evaluate(t.Context(), request, rules)
	require.NoError(t, err)
	restore := BeginExplanation(func(event Explanation) { events = append(events, event) }, rules)
	explained, err := NewEngine().Evaluate(t.Context(), request, rules)
	require.NoError(t, err)
	restore()
	require.Equal(t, baseline, explained)
	require.Contains(t, events, Explanation{Kind: Command, Rule: "deny", Status: "deny"})
	require.Contains(t, events, Explanation{Kind: TerraformPlan, Status: "not reached; no verdict"})
	raw, _ := json.Marshal(events)
	require.NotContains(t, string(raw), "secret-input")
}

func TestOfflinePolicyFixtures(t *testing.T) {
	bundle := &Bundle{Rules: []Rule{{ID: "deny", On: Command, Engine: CELEngine, Expression: "true", Effects: Effects{Deny: true}}}}
	cases := filepath.Join(t.TempDir(), "cases.json")
	require.NoError(t, os.WriteFile(cases, []byte(`[{"name":"denied","request":{"kind":"command","input":{}},"status":"deny"}]`), 0o600))
	results, err := RunTests(t.Context(), bundle, cases)
	require.NoError(t, err)
	require.Equal(t, []TestResult{{Name: "denied", Passed: true, Expected: Deny, Actual: Deny}}, results)
	require.NoError(t, os.WriteFile(cases, []byte(`[{"name":"mismatch","request":{"kind":"command","input":{}},"status":"allow"}]`), 0o600))
	results, err = RunTests(t.Context(), bundle, cases)
	require.Error(t, err)
	require.False(t, results[0].Passed)
	for _, raw := range []string{`[]`, `{} extra`, `[{"name":"x","status":"bogus"}]`, `[{"name":"x","status":"allow","unexpected":true}]`} {
		require.NoError(t, os.WriteFile(cases, []byte(raw), 0o600))
		_, err = RunTests(t.Context(), bundle, cases)
		require.Error(t, err)
	}
}

func TestImplementedInputSchemas(t *testing.T) {
	for _, kind := range []InputKind{Function, Source, Command, ComponentCommand, TerraformPlan, Operation, AtmosAffected} {
		schema, err := InputSchema(kind)
		require.NoError(t, err)
		require.NotEmpty(t, schema.Title)
	}
	_, err := InputSchema(GitChange)
	require.Error(t, err)
}
