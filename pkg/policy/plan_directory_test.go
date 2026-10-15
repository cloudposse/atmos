package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const testPlanManifest = `apiVersion: atmos/v1
kind: Policy
metadata:
  name: platform
spec:
  policies:
    no-deletes:
      on: terraform.plan
      rule: "has(input.resource_changes) && input.resource_changes.exists(r, 'delete' in r.change.actions)"
      action: deny
      reason: No resource deletions
      with:
        threshold: 3
`

func TestLoadPlanDirectory(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(testPlanManifest), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "README.md"), []byte("ignored"), 0o600))
	rules, err := LoadPlanDirectory(directory)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, "platform/no-deletes", rules[0].ID)
	require.Equal(t, CELEngine, rules[0].Engine)
	require.JSONEq(t, `{"threshold":3}`, string(rules[0].Parameters))
	require.True(t, rules[0].Effects.Deny)
	// Duplicate fully qualified identities cannot silently replace a governing rule.
	require.NoError(t, os.WriteFile(filepath.Join(directory, "duplicate.yml"), []byte(testPlanManifest), 0o600))
	_, err = LoadPlanDirectory(directory)
	require.Error(t, err)
}

func TestPolicyMetadataDescription(t *testing.T) {
	for _, tc := range []struct {
		name        string
		field       string
		description string
	}{
		{name: "omitted"},
		{name: "empty", field: "  description: ''\n"},
		{name: "plain English", field: "  description: Prevent accidental deletion of infrastructure.\n", description: "Prevent accidental deletion of infrastructure."},
		{name: "literal text", field: "  description: 'input.command == apply; {{ .env.SECRET }}'\n", description: "input.command == apply; {{ .env.SECRET }}"},
		{name: "multiline", field: "  description: |-\n    Protect resources.\n    Check the saved plan.\n", description: "Protect resources.\nCheck the saved plan."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := strings.Replace(testPlanManifest, "  name: platform\n", "  name: platform\n"+tc.field, 1)
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
			rules, err := LoadPlanDirectory(directory)
			require.NoError(t, err)
			require.Len(t, rules, 1)
			require.Equal(t, tc.description, rules[0].Description)
			require.Equal(t, "No resource deletions", rules[0].Effects.Reason)
			encoded, err := json.Marshal(rules[0])
			require.NoError(t, err)
			var decoded Rule
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, tc.description, decoded.Description)
			decision, err := testEngine(t).Evaluate(context.Background(), &Request{
				Kind: TerraformPlan, Input: json.RawMessage(`{"format_version":"1.2","planned_values":{}}`),
			}, rules)
			require.NoError(t, err)
			require.Equal(t, NotApplicable, decision.Status, "description must not change evaluation")
		})
	}
	t.Run("invalid description", func(t *testing.T) {
		manifest := strings.Replace(testPlanManifest, "  name: platform", "  name: platform\n  description: {invalid: object}", 1)
		directory := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
		_, err := LoadPlanDirectory(directory)
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	})
}

func TestPlanDirectoryRejectsUnsupportedDeclarations(t *testing.T) {
	for name, document := range map[string]string{
		"unknown field":    testPlanManifest + "      enforced: false\n",
		"unknown input":    strings.ReplaceAll(testPlanManifest, "terraform.plan", "operation"),
		"unknown action":   strings.ReplaceAll(testPlanManifest, "action: deny", "action: warn"),
		"unknown engine":   testPlanManifest + "      engine: unknown\n",
		"invalid CEL":      strings.ReplaceAll(testPlanManifest, "has(input.resource_changes)", "badfunction()"),
		"missing name":     strings.ReplaceAll(testPlanManifest, "name: platform", "name: ''"),
		"blank rule name":  strings.ReplaceAll(testPlanManifest, "no-deletes:", "'':"),
		"kind":             strings.ReplaceAll(testPlanManifest, "kind: Policy", "kind: PolicyLibrary"),
		"version":          strings.ReplaceAll(testPlanManifest, "atmos/v1", "atmos/v2"),
		"multi document":   testPlanManifest + "---\n" + testPlanManifest,
		"invalid yaml":     "[invalid",
		"empty":            "",
		"too large":        strings.Repeat(" ", maxPolicyDocumentBytes+1),
		"duplicate fields": testPlanManifest + "      action: deny\n",
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(document), 0o600))
			_, err := LoadPlanDirectory(directory)
			require.Error(t, err)
		})
	}
}

func TestPlanDirectoryFilesystemFailures(t *testing.T) {
	directory := t.TempDir()
	_, err := LoadPlanDirectory(directory)
	require.Error(t, err)
	_, err = LoadPlanDirectory(filepath.Join(directory, "missing"))
	require.Error(t, err)
	_, err = readPlanPolicyDocument(filepath.Join(directory, "missing"))
	require.Error(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(directory, "subdir.yaml"), 0o700))
	_, err = LoadPlanDirectory(directory)
	require.Error(t, err)
}

func TestPlanPolicyItemParameters(t *testing.T) {
	item := planPolicyItem{On: TerraformPlan, Rule: "false", Action: "deny"}
	rule, err := item.resolve("platform", "safe")
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(rule.Parameters))
	item.With = map[string]any{"unsupported": make(chan int)}
	_, err = item.resolve("platform", "safe")
	require.Error(t, err)
	item.Rule = strings.Repeat("x", maxExpressionBytes+1)
	_, err = item.resolve("platform", "safe")
	require.Error(t, err)
}

func TestOperationBundleValidation(t *testing.T) {
	data, err := os.ReadFile("../../examples/policies/lifecycle/policy.yaml")
	require.NoError(t, err)
	manifest := string(data)
	for name, text := range map[string]string{
		"missing hook":           strings.ReplaceAll(manifest, "ref: lint", "ref: missing"),
		"missing event":          strings.ReplaceAll(manifest, "event: after.terraform.plan", "event: ''"),
		"missing reference":      strings.ReplaceAll(manifest, "ref: lint", "ref: ''"),
		"plan hooks unsupported": strings.ReplaceAll(manifest, "on: operation", "on: terraform.plan"),
		"deny with hooks":        strings.ReplaceAll(manifest, "action: hooks", "action: deny"),
		"empty hooks": `apiVersion: atmos/v1
kind: Policy
metadata: {name: empty}
spec:
  policies:
    empty: {on: operation, action: hooks, rule: 'true'}
`,
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(text), 0o600))
			_, err := LoadDirectory(directory)
			require.Error(t, err)
		})
	}
	first := &planPolicyDocument{}
	first.APIVersion, first.Kind, first.Metadata.Name = "atmos/v1", "Policy", "first"
	first.Spec.Policies = map[string]planPolicyItem{"deny": {On: Operation, Action: "deny", Rule: "false"}}
	first.Spec.Hooks = map[string]map[string]any{"hook": {"kind": "command", "command": "true"}}
	first.Spec.Tools = map[string]string{"tool": "1.0"}
	bundle := &Bundle{Hooks: map[string]json.RawMessage{}, Tools: map[string]string{}}
	require.NoError(t, bundle.addDocument(first))
	require.Error(t, bundle.addDocument(first), "duplicate hooks are not replaced")
	first.Metadata.Name = "second"
	first.Spec.Tools["tool"] = "2.0"
	require.Error(t, bundle.addDocument(first), "conflicting pins are not merged")
	require.Empty(t, bundle.RulesFor(TerraformPlan))
	require.NotEmpty(t, bundle.RulesFor(Operation))
	t.Setenv("ATMOS_POLICY_DIR", "/protected/policies")
	directory, err := DirectoryFromEnvironment()
	require.NoError(t, err)
	require.Equal(t, "/protected/policies", directory)
}

func TestPlanDirectoryApprovalDeclarations(t *testing.T) {
	const approvals = "      approvals:\n        - id: platform\n          provider: github-actions\n          spec: {reviewers: [alice], minimum: 1}\n"
	manifest := func(action, extra string) string {
		return "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: platform}\nspec:\n  policies:\n    review:\n      on: terraform.plan\n      rule: \"true\"\n      action: " + action + "\n" + extra
	}
	for name, tc := range map[string]struct {
		document string
		wantErr  bool
	}{
		"approval requirement":          {document: manifest("require_approval", approvals)},
		"require_approval without spec": {document: manifest("require_approval", ""), wantErr: true},
		"approvals on deny":             {document: manifest("deny", approvals), wantErr: true},
		"empty reviewer spec":           {document: manifest("require_approval", "      approvals:\n        - id: platform\n          provider: github-actions\n          spec: {}\n"), wantErr: true},
		"team reviewer":                 {document: manifest("require_approval", "      approvals:\n        - id: platform\n          provider: github-actions\n          spec: {reviewers: [org/team], minimum: 1}\n"), wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(tc.document), 0o600))
			rules, err := LoadPlanDirectory(directory)
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
				return
			}
			require.NoError(t, err)
			require.Len(t, rules, 1)
			require.Len(t, rules[0].Effects.Approvals, 1)
			require.Equal(t, "platform", rules[0].Effects.Approvals[0].ID)
			require.Equal(t, "github-actions", rules[0].Effects.Approvals[0].Provider)
		})
	}
}
