package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestSourceRequestWithinAllowedRoots(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "data.txt")

	t.Run("no roots leaves the request untouched", func(t *testing.T) {
		request := &Request{Kind: Source, Input: json.RawMessage(`{"path":"relative"}`)}
		got, err := sourceRequest(request, nil)
		require.NoError(t, err)
		assert.Same(t, request, got)
	})

	t.Run("malformed input is rejected", func(t *testing.T) {
		_, err := sourceRequest(&Request{Kind: Source, Input: json.RawMessage(`[1]`)}, []string{root})
		require.Error(t, err)
	})

	for name, tc := range map[string]struct {
		input string
		want  bool
	}{
		"resolved path inside":              {input: `{"path":` + quoteJSON(t, inside) + `,"path_resolved":true}`, want: true},
		"resolved path equal to root":       {input: `{"path":` + quoteJSON(t, root) + `,"path_resolved":true}`, want: true},
		"unresolved path is never inside":   {input: `{"path":` + quoteJSON(t, inside) + `,"path_resolved":false}`, want: false},
		"missing resolution flag":           {input: `{"path":` + quoteJSON(t, inside) + `}`, want: false},
		"relative path is never inside":     {input: `{"path":"data.txt","path_resolved":true}`, want: false},
		"parent of the root":                {input: `{"path":` + quoteJSON(t, filepath.Dir(root)) + `,"path_resolved":true}`, want: false},
		"non-string path":                   {input: `{"path":7,"path_resolved":true}`, want: false},
		"sibling sharing the root's prefix": {input: `{"path":` + quoteJSON(t, root+"-sibling") + `,"path_resolved":true}`, want: false},
	} {
		t.Run(name, func(t *testing.T) {
			request := &Request{Kind: Source, Input: json.RawMessage(tc.input)}
			got, err := sourceRequest(request, []string{root})
			require.NoError(t, err)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(got.Input, &decoded))
			assert.Equal(t, tc.want, decoded["within_allowed_roots"])
			// The caller's request is never mutated.
			assert.NotContains(t, string(request.Input), "within_allowed_roots")
		})
	}
}

func quoteJSON(t *testing.T, value string) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func TestResolveRoots(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(base, "relative"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(base, "file.txt"), []byte("x"), 0o600))
	canonicalBase, err := filepath.EvalSymlinks(base)
	require.NoError(t, err)

	t.Run("relative and absolute roots are canonicalized", func(t *testing.T) {
		got, err := resolveRoots([]string{"relative", base}, base)
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(canonicalBase, "relative"), canonicalBase}, got)
	})

	t.Run("no roots yields no constraint", func(t *testing.T) {
		got, err := resolveRoots(nil, base)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	for name, root := range map[string]string{
		"empty root":        "",
		"blank root":        "  ",
		"nonexistent root":  "missing",
		"root is not a dir": "file.txt",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveRoots([]string{root}, base)
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		})
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

func TestRunTestsRejectsUnusableFixtures(t *testing.T) {
	bundle := &Bundle{Rules: []Rule{{ID: "deny", On: Command, Engine: CELEngine, Expression: "true", Effects: Effects{Deny: true}}}}
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	require.NoError(t, os.WriteFile(valid, []byte(`[{"name":"denied","request":{"kind":"command","input":{}},"status":"deny"}]`), 0o600))

	t.Run("missing bundle", func(t *testing.T) {
		results, err := RunTests(t.Context(), nil, valid)
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		assert.Nil(t, results)
	})

	t.Run("missing cases file", func(t *testing.T) {
		results, err := RunTests(t.Context(), bundle, filepath.Join(dir, "missing.json"))
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		assert.Nil(t, results)
	})

	t.Run("cases path is a directory", func(t *testing.T) {
		_, err := RunTests(t.Context(), bundle, dir)
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	})

	t.Run("oversized cases file", func(t *testing.T) {
		large := filepath.Join(dir, "large.json")
		require.NoError(t, os.WriteFile(large, []byte(strings.Repeat(" ", maxTestCaseBytes+1)), 0o600))
		_, err := RunTests(t.Context(), bundle, large)
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		assert.Contains(t, err.Error(), "too large")
	})

	for name, raw := range map[string]string{
		"duplicate names":       `[{"name":"a","request":{"kind":"command","input":{}},"status":"deny"},{"name":"a","request":{"kind":"command","input":{}},"status":"deny"}]`,
		"empty name":            `[{"name":"","request":{"kind":"command","input":{}},"status":"deny"}]`,
		"unknown error kind":    `[{"name":"a","request":{"kind":"command","input":{}},"status":"deny","error":"bogus"}]`,
		"not an array":          `{"name":"a"}`,
		"trailing second value": `[{"name":"a","request":{"kind":"command","input":{}},"status":"deny"}] []`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, "case.json")
			require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
			_, err := RunTests(t.Context(), bundle, path)
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		})
	}
}

func TestRunTestsMatchesExpectedErrorsAndEffects(t *testing.T) {
	hook := HookRequirement{Ref: "platform/lint", Event: "before.terraform.plan", Required: true}
	approval := ApprovalRequirement{ID: "review", Provider: "github-actions", Spec: json.RawMessage(`{"reviewers":["alice"],"minimum":1}`)}
	bundle := &Bundle{Rules: []Rule{
		{ID: "hooked", On: Operation, Engine: CELEngine, Expression: "true", Effects: Effects{Hooks: []HookRequirement{hook}}},
		{ID: "attached", On: ComponentCommand, Engine: CELEngine, Expression: "true", Match: &Selector{Stacks: []string{"prod"}}, Effects: Effects{Deny: true}},
		{ID: "unknown-engine", On: Function, Engine: "nope", Expression: "true"},
		{ID: "review", On: Source, Engine: CELEngine, Expression: "true", Effects: Effects{Approvals: []ApprovalRequirement{approval}}},
	}}
	run := func(t *testing.T, raw string) ([]TestResult, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "cases.json")
		require.NoError(t, os.WriteFile(path, []byte(raw), 0o600))
		return RunTests(t.Context(), bundle, path)
	}
	hookJSON := `{"ref":"platform/lint","event":"before.terraform.plan","required":true,"require_success":false}`

	for name, tc := range map[string]struct {
		cases      string
		wantPassed bool
	}{
		"input unavailable is expected":  {`[{"name":"x","request":{"kind":"component.command","input":{}},"status":"error","error":"input_unavailable"}]`, true},
		"evaluation error is expected":   {`[{"name":"x","request":{"kind":"function","input":{}},"status":"error","error":"evaluation"}]`, true},
		"unexpected error fails":         {`[{"name":"x","request":{"kind":"function","input":{}},"status":"error"}]`, false},
		"wrong error kind fails":         {`[{"name":"x","request":{"kind":"function","input":{}},"status":"error","error":"input_unavailable"}]`, false},
		"hook effects match":             {`[{"name":"x","request":{"kind":"operation","input":{}},"status":"allow","hooks":[` + hookJSON + `]}]`, true},
		"hook effects missing":           {`[{"name":"x","request":{"kind":"operation","input":{}},"status":"allow"}]`, false},
		"approvals match when declared":  {`[{"name":"x","request":{"kind":"source","input":{}},"status":"awaiting_approval","approvals":[{"id":"review","provider":"github-actions","spec":{"reviewers":["alice"],"minimum":1}}]}]`, true},
		"approvals mismatch fails":       {`[{"name":"x","request":{"kind":"source","input":{}},"status":"awaiting_approval","approvals":[{"id":"other","provider":"github-actions","spec":{}}]}]`, false},
		"approvals unchecked when unset": {`[{"name":"x","request":{"kind":"source","input":{}},"status":"awaiting_approval"}]`, true},
	} {
		t.Run(name, func(t *testing.T) {
			results, err := run(t, tc.cases)
			require.Len(t, results, 1)
			assert.Equal(t, tc.wantPassed, results[0].Passed, "results: %+v err: %v", results, err)
			assert.Equal(t, !tc.wantPassed, errors.Is(err, errUtils.ErrPolicyEvaluation))
		})
	}
}

func TestTestErrorMatches(t *testing.T) {
	unavailable := fmt.Errorf("%w: rule", errUtils.ErrPolicyInputUnavailable)
	evaluation := fmt.Errorf("%w: rule", errUtils.ErrPolicyEvaluation)
	for name, tc := range map[string]struct {
		expected string
		err      error
		want     bool
	}{
		"no error expected and none raised":    {"", nil, true},
		"no error expected but one raised":     {"", evaluation, false},
		"unavailable input expected":           {"input_unavailable", unavailable, true},
		"unavailable input expected, none":     {"input_unavailable", nil, false},
		"unavailable input expected, other":    {"input_unavailable", evaluation, false},
		"evaluation expected":                  {"evaluation", evaluation, true},
		"evaluation expected, other":           {"evaluation", errors.New("unrelated"), false},
		"unknown expectation never matches":    {"bogus", evaluation, false},
		"unknown expectation, no error raised": {"bogus", nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, testErrorMatches(tc.expected, tc.err))
		})
	}
}

func TestImplementedInputSchemas(t *testing.T) {
	for _, kind := range []InputKind{Function, Source, Plugin, Command, ComponentCommand, TerraformPlan, Operation, AtmosAffected} {
		schema, err := InputSchema(kind)
		require.NoError(t, err)
		require.NotEmpty(t, schema.Title)
	}
	_, err := InputSchema(GitChange)
	require.Error(t, err)
}

func TestPluginInputSchemaListsPathAndProfiles(t *testing.T) {
	schema, err := InputSchema(Plugin)
	require.NoError(t, err)
	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	var document struct {
		Defs map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	properties := document.Defs["PluginInput"].Properties
	for _, field := range []string{"path", "profiles", "origin", "development"} {
		assert.Contains(t, properties, field)
	}
	assert.Contains(t, string(properties["profiles"]), `"array"`)
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
