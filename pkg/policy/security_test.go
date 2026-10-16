package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestIsDestroyInvocation(t *testing.T) {
	tf := func(sub string) []string { return []string{"terraform", sub} }
	tests := []struct {
		name string
		path []string
		args []string
		want bool
	}{
		{name: "empty path", args: []string{"-destroy"}},
		{name: "terraform destroy command", path: tf("destroy"), want: true},
		{name: "terraform destroy ignores false flag", path: tf("destroy"), args: []string{"-destroy=false"}, want: true},
		{name: "helmfile destroy command", path: []string{"helmfile", "destroy"}, want: true},
		{name: "helmfile apply", path: []string{"helmfile", "apply"}},
		{name: "helmfile ignores destroy flag", path: []string{"helmfile", "apply"}, args: []string{"-destroy"}},
		{name: "helm delete is its own command", path: []string{"helm", "delete"}},
		{name: "kubernetes delete is its own command", path: []string{"kubernetes", "delete"}},
		{name: "packer ignores destroy flag", path: []string{"packer", "build"}, args: []string{"--destroy"}},
		{name: "terraform root only", path: []string{"terraform"}},
		{name: "terraform root with flag", path: []string{"terraform"}, args: []string{"-destroy"}, want: true},
		{name: "plain apply", path: tf("apply"), args: []string{"apply", "vpc", "-s", "dev", "-auto-approve"}},
		{name: "plain plan", path: tf("plan"), args: []string{"plan", "vpc", "-s", "dev"}},
		{name: "no args", path: tf("apply")},
		{name: "apply single dash", path: tf("apply"), args: []string{"apply", "vpc", "-s", "dev", "-destroy", "-auto-approve"}, want: true},
		{name: "apply double dash", path: tf("apply"), args: []string{"--destroy"}, want: true},
		{name: "plan single dash", path: tf("plan"), args: []string{"-destroy"}, want: true},
		{name: "deploy single dash", path: tf("deploy"), args: []string{"-destroy"}, want: true},
		{name: "any other subcommand", path: tf("refresh"), args: []string{"-destroy"}, want: true},
		{name: "after separator", path: tf("apply"), args: []string{"apply", "vpc", "-s", "dev", "--", "-destroy"}, want: true},
		{name: "after separator with value", path: tf("plan"), args: []string{"--", "--destroy=true"}, want: true},
		{name: "single dash true", path: tf("apply"), args: []string{"-destroy=true"}, want: true},
		{name: "double dash true", path: tf("apply"), args: []string{"--destroy=true"}, want: true},
		{name: "value 1", path: tf("apply"), args: []string{"-destroy=1"}, want: true},
		{name: "value t", path: tf("apply"), args: []string{"-destroy=t"}, want: true},
		{name: "value T", path: tf("apply"), args: []string{"-destroy=T"}, want: true},
		{name: "value TRUE", path: tf("apply"), args: []string{"-destroy=TRUE"}, want: true},
		{name: "value True", path: tf("apply"), args: []string{"--destroy=True"}, want: true},
		{name: "value false", path: tf("apply"), args: []string{"-destroy=false"}},
		{name: "value 0", path: tf("apply"), args: []string{"-destroy=0"}},
		{name: "value f", path: tf("plan"), args: []string{"--destroy=f"}},
		{name: "value F", path: tf("plan"), args: []string{"--destroy=F"}},
		{name: "value FALSE", path: tf("plan"), args: []string{"-destroy=FALSE"}},
		{name: "false after separator", path: tf("apply"), args: []string{"--", "-destroy=false"}},
		{name: "unparseable value fails closed", path: tf("apply"), args: []string{"-destroy=maybe"}, want: true},
		{name: "empty value fails closed", path: tf("apply"), args: []string{"-destroy="}, want: true},
		{name: "true then false fails closed", path: tf("apply"), args: []string{"-destroy", "-destroy=false"}, want: true},
		{name: "false then true", path: tf("apply"), args: []string{"-destroy=false", "-destroy"}, want: true},
		{name: "false only twice", path: tf("apply"), args: []string{"-destroy=false", "--destroy=0"}},
		{name: "variable named destroy", path: tf("apply"), args: []string{"-var", "destroy=true"}},
		{name: "var assignment", path: tf("apply"), args: []string{"-var=destroy=true"}},
		{name: "positional named destroy", path: tf("apply"), args: []string{"destroy"}},
		{name: "similar flag", path: tf("apply"), args: []string{"-destroyed", "--no-destroy", "-destroy-all"}},
		{name: "triple dash is not a flag", path: tf("apply"), args: []string{"---destroy"}},
		{name: "flags are case sensitive", path: tf("apply"), args: []string{"-Destroy"}},
		{name: "separator alone", path: tf("apply"), args: []string{"--"}},
		{name: "lone dash", path: tf("apply"), args: []string{"-"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsDestroyInvocation(tc.path, tc.args))
		})
	}
}
