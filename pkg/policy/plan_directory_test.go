package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestPlanDirectoryRejectsUnsupportedDeclarations(t *testing.T) {
	for name, document := range map[string]string{
		"unknown field":    testPlanManifest + "      enforced: false\n",
		"unknown input":    strings.ReplaceAll(testPlanManifest, "terraform.plan", "operation"),
		"unknown action":   strings.ReplaceAll(testPlanManifest, "action: deny", "action: warn"),
		"unknown engine":   testPlanManifest + "      engine: starlark\n",
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
