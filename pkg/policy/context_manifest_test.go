package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContextManifestAndOfflineFixtures(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "schemas"), 0o700))
	manifest := `apiVersion: atmos/v1
kind: Policy
metadata: {name: runtime}
spec:
  policies:
    calendar:
      on: command
      engine: jsonschema
      action: deny
      checks:
        date:
          kind: date
          path: /input/expires
          format: date
      schema:
        path: schemas/policy.schema.json
        target: context
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "policy.yaml"), []byte(manifest), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "schemas", "policy.schema.json"), []byte(`{"type":"object","properties":{"checks":{"properties":{"date":{"const":true}},"required":["date"]}},"required":["checks"]}`), 0o600))
	bundle, err := LoadDirectory(directory)
	require.NoError(t, err)
	require.Equal(t, "context", bundle.Rules[0].Schema.Target)
	cases := []TestCase{
		{Name: "valid", Request: Request{Kind: Command, Input: json.RawMessage(`{"expires":"2024-02-29"}`)}, Status: NotApplicable},
		{Name: "invalid", Request: Request{Kind: Command, Input: json.RawMessage(`{"expires":"2025-02-29"}`)}, Status: Deny},
	}
	data, err := json.Marshal(cases)
	require.NoError(t, err)
	path := filepath.Join(directory, "cases.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	results, err := RunTests(t.Context(), bundle, path)
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.True(t, results[0].Passed)
	require.True(t, results[1].Passed)
	// Offline tests must not accidentally consume this process's environment.
	t.Setenv("ATMOS_CONTEXT_OFFLINE_TEST", "present")
	bundle.Rules = []Rule{{ID: "offline", On: Command, Engine: CELEngine, Expression: `has(env.ATMOS_CONTEXT_OFFLINE_TEST)`, Effects: Effects{Deny: true}}}
	cases = cases[:1]
	data, err = json.Marshal(cases)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	_, err = RunTests(t.Context(), bundle, path)
	require.NoError(t, err)
}

func TestContextSchemaPreservesInputAndAssertsFormats(t *testing.T) {
	for _, target := range []string{"", "input"} {
		config := &SchemaConfig{Target: target, Entry: schemaBase + "schema.json", Resources: map[string]json.RawMessage{schemaBase + "schema.json": json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"date":{"type":"string","format":"date"}}}`)}}
		rule := Rule{ID: "date", On: Command, Engine: JSONSchemaEngine, Schema: config, Effects: Effects{Deny: true}}
		for _, tc := range []struct {
			date string
			want DecisionStatus
		}{{"2024-02-29", NotApplicable}, {"2025-02-29", Deny}} {
			raw, _ := json.Marshal(map[string]string{"date": tc.date})
			decision, err := NewEngine().Evaluate(t.Context(), &Request{Kind: Command, Input: raw}, []Rule{rule})
			require.NoError(t, err)
			require.Equal(t, tc.want, decision.Status)
		}
	}
}

func TestFileChecksUseSharedCIEvidence(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "input.json"), []byte(`{}`), 0o600))
	ci := &CISnapshot{Provider: "github-actions", Schema: "github-actions/v1", Event: Dataset{Status: Available, Complete: true, Data: json.RawMessage(`{"pull_request":{"labels":[{"name":"preview"}]}}`)}}
	rule := Rule{ID: "preview", On: File, Engine: CELEngine, Expression: `ci != null && ci.labels != null && 'preview' in ci.labels`, Files: &FileSelector{Include: []string{"*.json"}}, Effects: Effects{Deny: true}}
	results, err := CheckFiles(t.Context(), &Bundle{Rules: []Rule{rule}}, root, WithFileEvidence(NewEngine(), ci))
	require.Error(t, err)
	require.Len(t, results, 1)
	require.Equal(t, Deny, results[0].Decision.Status)
	results, err = CheckFiles(t.Context(), &Bundle{Rules: []Rule{rule}}, root, WithFileEvidence(nil, nil))
	require.NoError(t, err)
	require.Equal(t, NotApplicable, results[0].Decision.Status)
}

func TestOfflineChecksRequireHostFactsAcrossEngines(t *testing.T) {
	adapters := []Rule{
		{Engine: CELEngine, Expression: `checks.valid`},
		{Engine: StarlarkEngine, Expression: `return ctx.checks["valid"]`},
		{Engine: OPAEngine, OPA: &OPAConfig{Query: "data.test.deny", Modules: map[string]string{"test.rego": "package test\ndeny = data.atmos_context.checks.valid"}}},
		{Engine: JSONSchemaEngine, Schema: &SchemaConfig{Target: "context", Entry: schemaBase + "test.json", Resources: map[string]json.RawMessage{schemaBase + "test.json": json.RawMessage(`{"not":{"properties":{"checks":{"properties":{"valid":{"const":true}}}}}}`)}}},
	}
	checks := []struct {
		name    string
		check   ContextCheck
		context *RuntimeContext
		status  DecisionStatus
		error   string
	}{
		{"missing clock", ContextCheck{Kind: "schedule"}, nil, EvaluationError, "input_unavailable"},
		{"missing version", ContextCheck{Kind: "semver", Path: "/atmos/version", Constraint: ">= 1.0.0"}, nil, EvaluationError, "input_unavailable"},
		{"partial context", ContextCheck{Kind: "schedule"}, &RuntimeContext{Atmos: AtmosFacts{Version: "1.233.0"}}, EvaluationError, "input_unavailable"},
		{"provided clock", ContextCheck{Kind: "schedule"}, &RuntimeContext{Time: ClockFacts{Now: "2026-10-21T12:00:00Z"}}, Deny, ""},
		{"provided version", ContextCheck{Kind: "semver", Path: "/atmos/version", Constraint: ">= 1.0.0"}, &RuntimeContext{Atmos: AtmosFacts{Version: "1.233.0"}}, Deny, ""},
		{"missing env", ContextCheck{Kind: "date", Path: "/env/DATE"}, nil, EvaluationError, "input_unavailable"},
		{"empty env", ContextCheck{Kind: "date", Path: "/env/DATE"}, &RuntimeContext{Env: map[string]string{"DATE": ""}}, NotApplicable, ""},
		{"empty input", ContextCheck{Kind: "date", Path: "/input/date"}, nil, NotApplicable, ""},
	}
	for _, adapter := range adapters {
		for _, check := range checks {
			t.Run(adapter.Engine+"/"+check.name, func(t *testing.T) {
				rule := adapter
				rule.ID, rule.On, rule.Effects.Deny = "check", Command, true
				rule.Checks = map[string]ContextCheck{"valid": check.check}
				cases := []TestCase{{Name: check.name, Request: Request{Kind: Command, Input: json.RawMessage(`{"date":""}`), Context: check.context}, Status: check.status, Error: check.error}}
				raw, err := json.Marshal(cases)
				require.NoError(t, err)
				path := filepath.Join(t.TempDir(), "cases.json")
				require.NoError(t, os.WriteFile(path, raw, 0o600))
				results, err := RunTests(t.Context(), &Bundle{Rules: []Rule{rule}}, path)
				require.NoError(t, err)
				require.Len(t, results, 1)
				require.True(t, results[0].Passed)
			})
		}
	}
}
