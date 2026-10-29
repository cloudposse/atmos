package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

func errorPolicyDocument(on, extra string) string {
	return "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: recovery}\nspec:\n  policies:\n    transient:\n      description: Retry transient failures.\n      on: " + on + "\n      action: retry\n      rule: input.output.contains('transient')\n      with:\n        max_attempts: 3\n        initial_delay: 2s\n" + extra
}

func loadErrorPolicy(t *testing.T, text string) (*Bundle, error) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "retry.yaml"), []byte(text), 0o600))
	return LoadDirectory(dir)
}

func TestErrorPolicySelectors(t *testing.T) {
	for _, on := range []string{"terraform.init.error", "[terraform.init.error, terraform.plan.error]", "terraform.*.error", "[terraform.*.error, terraform.init.error]"} {
		t.Run(on, func(t *testing.T) {
			bundle, err := loadErrorPolicy(t, errorPolicyDocument(on, ""))
			require.NoError(t, err)
			require.Len(t, bundle.RulesFor("terraform.init.error"), 1)
			assert.Equal(t, "Retry transient failures.", bundle.Rules[0].Description)
			assert.Empty(t, bundle.RulesFor("packer.init.error"))
			raw, err := json.Marshal(ErrorInput{Output: "transient network error"})
			require.NoError(t, err)
			decision, err := NewEngine().Evaluate(context.Background(), &Request{Kind: "terraform.init.error", Input: raw}, bundle.Rules)
			require.NoError(t, err)
			assert.Equal(t, Allow, decision.Status)
			require.Len(t, decision.Findings, 1)
			require.NotNil(t, decision.Findings[0].Effects.Retry)
			assert.Equal(t, 3, *decision.Findings[0].Effects.Retry.MaxAttempts)
		})
	}
}

func TestErrorPolicyValidation(t *testing.T) {
	for _, on := range []string{"[]", "[operation]", "[terraform.init.error, operation]", "[terraform.init.error, 1]", "'*.plan.error'", "terraform.p*.error", "operation", "operation.error", "null"} {
		t.Run(on, func(t *testing.T) {
			_, err := loadErrorPolicy(t, errorPolicyDocument(on, ""))
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
		})
	}
	for _, extra := range []string{"        delay: 2s\n", "        conditions: [transient]\n", "        backoff_strategy: invalid\n", "        multiplier: 0\n"} {
		_, err := loadErrorPolicy(t, errorPolicyDocument("terraform.plan.error", extra))
		require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	}
}

func TestCanonicalErrorEvents(t *testing.T) {
	for _, tc := range []struct{ kind, command, event string }{
		{"terraform", "deploy", "terraform.apply.error"},
		{"terraform", "workspace-select", "terraform.workspace.select.error"},
		{"terraform", "workspace select", "terraform.workspace.select.error"},
		{"helmfile", "sync", "helmfile.apply.error"},
		{"helm", "destroy", "helm.delete.error"},
		{"kubernetes", "plan", "kubernetes.diff.error"},
		{"aws/cloudformation", "deploy", "aws/cloudformation.apply.error"},
		{"custom", "run", "custom.run.error"},
	} {
		assert.Equal(t, InputKind(tc.event), ErrorEvent(tc.kind, tc.command))
		assert.True(t, IsErrorEvent(InputKind(tc.event)))
	}
	bundle, err := loadErrorPolicy(t, errorPolicyDocument("[helmfile.sync.error, terraform.deploy.error]", ""))
	require.NoError(t, err)
	assert.Len(t, bundle.RulesFor("helmfile.apply.error"), 1)
	assert.Len(t, bundle.RulesFor("terraform.apply.error"), 1)
	_, err = InputSchema("kubernetes.apply.error")
	require.NoError(t, err)
}

func TestErrorPolicyDocumentSchemaAndPublishedExamples(t *testing.T) {
	raw, err := os.ReadFile("../datafetcher/schema/policy/policy/1.0.json")
	require.NoError(t, err)
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("policy.json", strings.NewReader(string(raw))))
	schema, err := compiler.Compile("policy.json")
	require.NoError(t, err)
	doc, err := os.ReadFile(filepath.Join("..", "..", "website", "docs", "policies", "retries.mdx"))
	require.NoError(t, err)
	_, yamlBlock, found := strings.Cut(string(doc), "```yaml title=\"policy.d/retries.yaml\"\n")
	require.True(t, found)
	yamlBlock, _, found = strings.Cut(yamlBlock, "```")
	require.True(t, found)
	var example map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(yamlBlock), &example))
	require.NoError(t, schema.Validate(example))
	bundle, err := loadErrorPolicy(t, yamlBlock)
	require.NoError(t, err)
	require.Len(t, bundle.Rules, 4)

	for _, on := range []string{"terraform.init.error", "[terraform.init.error, terraform.output.error]", "terraform.*.error"} {
		var document map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(errorPolicyDocument(on, "")), &document))
		require.NoError(t, schema.Validate(document))
		rule := document["spec"].(map[string]any)["policies"].(map[string]any)["transient"].(map[string]any)
		rule["with"] = map[string]any{"max_attempts": 0}
		require.Error(t, schema.Validate(document))
		rule["with"] = map[string]any{"max_attempts": 3, "conditions": []any{"timeout"}}
		require.Error(t, schema.Validate(document))
		rule["with"] = map[string]any{}
		require.Error(t, schema.Validate(document))
		rule["with"] = map[string]any{"max_attempts": 3}
		rule["on"] = "operation"
		require.Error(t, schema.Validate(document))
	}
}

func TestErrorPolicyExplanationTracksSelectors(t *testing.T) {
	bundle, err := loadErrorPolicy(t, errorPolicyDocument("[terraform.*.error, packer.build.error]", ""))
	require.NoError(t, err)
	var events []Explanation
	finish := BeginExplanation(func(event Explanation) { events = append(events, event) }, bundle.Rules)
	Explain("terraform.workspace.select.error", "recovery/transient", "allow")
	finish()
	require.Equal(t, []Explanation{
		{Kind: "terraform.workspace.select.error", Rule: "recovery/transient", Status: "allow"},
		{Kind: "packer.build.error", Status: "not reached; no verdict"},
	}, events)
}

func TestErrorPolicyActionRestrictions(t *testing.T) {
	for _, tc := range []struct {
		action string
		want   string
	}{
		{"hooks", "error events support retry or deny"},
		{"require_approval", "error events support retry or deny"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			document := strings.Replace(errorPolicyDocument("terraform.init.error", ""), "action: retry", "action: "+tc.action, 1)
			_, err := loadErrorPolicy(t, document)
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
			require.ErrorContains(t, err, tc.want)
		})
	}
	document := strings.Replace(errorPolicyDocument("terraform.init.error", ""), "max_attempts: 3", "max_attempts: 0", 1)
	_, err := loadErrorPolicy(t, document)
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
}
