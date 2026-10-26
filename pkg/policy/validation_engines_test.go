package policy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestOPAResults(t *testing.T) {
	for _, tc := range []struct {
		name, source, query string
		status              DecisionStatus
		messages            []string
		invalid             bool
	}{
		{"true", `deny = true`, "data.example.deny", Deny, nil, false},
		{"false", `deny = false`, "data.example.deny", NotApplicable, nil, false},
		{"empty set", `deny[msg] { input.block; msg := "blocked" }`, "data.example.deny", NotApplicable, nil, false},
		{"set", `deny[msg] { msg := "second" }
deny[msg] { msg := "first" }`, "data.example.deny", Deny, []string{"first", "second"}, false},
		{"array", `deny = ["missing tag"]`, "data.example.deny", Deny, []string{"missing tag"}, false},
		{"object finding", `deny = [{"msg": "wrong owner", "path": "/metadata/owner", "resource": "bucket"}]`, "data.example.deny", Deny, []string{"wrong owner"}, false},
		{"undefined", `deny = false`, "data.example.missing", EvaluationError, nil, true},
		{"scalar", `deny = "bad"`, "data.example.deny", EvaluationError, nil, true},
		{"invalid item", `deny = [42]`, "data.example.deny", EvaluationError, nil, true},
		{"bindings", `deny = [true, false]`, "data.example.deny[x]", EvaluationError, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := Rule{ID: "opa", On: File, Engine: OPAEngine, Effects: Effects{Deny: true}, OPA: &OPAConfig{Query: tc.query, Modules: map[string]string{"policy.rego": "package example\n" + tc.source}}}
			decision, err := NewEngine().Evaluate(t.Context(), &Request{Kind: File, Input: json.RawMessage(`{}`)}, []Rule{rule})
			if tc.invalid {
				require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.status, decision.Status)
			var messages []string
			for _, diagnostic := range decision.Findings[0].Diagnostics {
				messages = append(messages, diagnostic.Message)
			}
			assert.Equal(t, tc.messages, messages)
			if tc.name == "object finding" {
				assert.Equal(t, "/metadata/owner", decision.Findings[0].Diagnostics[0].Path)
				assert.JSONEq(t, `{"msg":"wrong owner","path":"/metadata/owner","resource":"bucket"}`, string(decision.Findings[0].Diagnostics[0].Details))
				denial := EnforceDecision(decision)
				assert.NotContains(t, denial.Error(), "wrong owner")
				assert.Equal(t, decision.Findings[0].Diagnostics, Violations(denial)[0].Diagnostics)
			}
		})
	}
}

func TestOPACapabilitiesAndCancellation(t *testing.T) {
	for _, expression := range []string{
		`http.send({"method":"GET", "url":"http://127.0.0.1"})`,
		`time.now_ns()`, `uuid.rfc4122("x")`, `opa.runtime()`, `net.lookup_ip_addr("localhost")`,
		`trace("secret")`, `print("secret")`,
	} {
		t.Run(expression, func(t *testing.T) {
			config := &OPAConfig{Query: "data.example.deny", Modules: map[string]string{"test.rego": "package example\ndeny { " + expression + " }"}}
			_, err := prepareOPA(t.Context(), config, nil)
			require.Error(t, err)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := (&OPA{}).Evaluate(ctx, Evaluation{Request: &Request{Kind: File, Input: json.RawMessage(`{}`)}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestOPARegoV1AndMetadata(t *testing.T) {
	config := &OPAConfig{Query: "data.example.deny", RegoVersion: 1, Modules: map[string]string{
		"main.rego": `package example
deny contains "owner" if { input.owner != data.atmos_context.params.owner; data.atmos_context.request.file == "owner.json" }`,
	}}
	evaluation := Evaluation{OPA: config, Parameters: json.RawMessage(`{"owner":"platform"}`), Request: &Request{Kind: File, File: "owner.json", Input: json.RawMessage(`{"owner":"other"}`)}}
	result, err := (&OPA{}).EvaluateDetailed(t.Context(), evaluation)
	require.NoError(t, err)
	require.True(t, result.Matched)
	require.Equal(t, "owner", result.Diagnostics[0].Message)
}

func TestOPARuntimeErrorDoesNotExposeInput(t *testing.T) {
	config := &OPAConfig{Query: "data.test.deny", Modules: map[string]string{
		"test.rego": "package test\ndeny = json.unmarshal(input.value)",
	}}
	_, err := (&OPA{}).Evaluate(t.Context(), Evaluation{OPA: config, Request: &Request{Kind: File, Input: json.RawMessage(`{"value":"private-not-json"}`)}})
	require.ErrorIs(t, err, errUtils.ErrPolicyEvaluation)
	assert.NotContains(t, err.Error(), "private-not-json")
}

func TestValidationEnginesConcurrent(t *testing.T) {
	engine := NewEngine()
	rules := []Rule{
		{ID: "opa", On: File, Engine: OPAEngine, Effects: Effects{Deny: true}, OPA: &OPAConfig{Query: "data.test.deny", Modules: map[string]string{"test.rego": "package test\ndeny = input.block"}}},
		{ID: "schema", On: File, Engine: JSONSchemaEngine, Effects: Effects{Deny: true}, Schema: &SchemaConfig{Entry: schemaBase + "test.json", Resources: map[string]json.RawMessage{schemaBase + "test.json": json.RawMessage(`{"properties":{"block":{"const":false}}}`)}}},
	}
	var workers sync.WaitGroup
	for index := range 8 {
		workers.Go(func() {
			input := json.RawMessage(`{"block":false}`)
			want := NotApplicable
			if index%2 == 0 {
				input = json.RawMessage(`{"block":true}`)
				want = Deny
			}
			decision, err := engine.Evaluate(t.Context(), &Request{Kind: File, Input: input}, rules)
			assert.NoError(t, err)
			assert.Equal(t, want, decision.Status)
		})
	}
	workers.Wait()
}

const optionalOwnerSchema = `{"type":"object","properties":{"metadata":{"type":"object","properties":{"owner":{"const":"platform"}}}}}`

func TestJSONSchemaOptionalOwner(t *testing.T) {
	config := &SchemaConfig{Entry: schemaBase + "owner.json", Resources: map[string]json.RawMessage{schemaBase + "owner.json": json.RawMessage(optionalOwnerSchema)}}
	for _, tc := range []struct {
		input   string
		invalid bool
	}{
		{`{}`, false},
		{`{"metadata":{}}`, false},
		{`{"metadata":{"owner":"platform"}}`, false},
		{`{"metadata":{"owner":"other"}}`, true},
		{`{"metadata":{"owner":null}}`, true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			rule := Rule{ID: "owner", On: File, Engine: JSONSchemaEngine, Schema: config, Effects: Effects{Deny: true}}
			decision, err := NewEngine().Evaluate(t.Context(), &Request{Kind: File, Input: json.RawMessage(tc.input)}, []Rule{rule})
			require.NoError(t, err)
			if tc.invalid {
				require.Equal(t, Deny, decision.Status)
				require.NotEmpty(t, decision.Findings[0].Diagnostics)
				assert.Equal(t, "/metadata/owner", decision.Findings[0].Diagnostics[0].Path)
				assert.Contains(t, decision.Findings[0].Diagnostics[0].SchemaPath, "/const")
			} else {
				assert.Equal(t, NotApplicable, decision.Status)
			}
		})
	}
}

func TestSchemaResources(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "schemas"), 0o700))
	write := func(name, contents string) {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600))
	}
	write("schemas/main.json", `{"$ref":"owner.json"}`)
	write("schemas/owner.json", optionalOwnerSchema)
	config, err := loadSchemaResources(directory, "schemas/main.json")
	require.NoError(t, err)
	require.Len(t, config.Resources, 2)
	// Evaluation uses captured resources even if the source files disappear.
	require.NoError(t, os.Remove(filepath.Join(directory, "schemas", "owner.json")))
	matched, err := (&JSONSchema{}).Evaluate(t.Context(), Evaluation{Schema: config, Request: &Request{Kind: File, Input: json.RawMessage(`{"metadata":{"owner":null}}`)}})
	require.NoError(t, err)
	require.True(t, matched)
	for _, schema := range []string{
		`{"$ref":"https://example.com/schema.json"}`, `{"$ref":"file:///etc/passwd"}`,
		`{"$ref":"missing.json"}`, `{"type":42}`, `{"$ref":"#/missing"}`,
	} {
		write("schemas/bad.json", schema)
		_, err := loadSchemaResources(directory, "schemas/bad.json")
		require.Error(t, err, schema)
	}
	_, err = loadSchemaResources(directory, "../outside.json")
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = (&JSONSchema{}).Evaluate(ctx, Evaluation{Schema: config, Request: &Request{Kind: File, Input: json.RawMessage(`{}`)}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestValidationEnginesOnTerraformPlan(t *testing.T) {
	input := json.RawMessage(`{"format_version":"1.2","planned_values":{},"resource_changes":[{"address":"test.db","change":{"actions":["delete"]}}]}`)
	rules := []Rule{
		{ID: "rego", On: TerraformPlan, Engine: OPAEngine, Effects: Effects{Deny: true}, OPA: &OPAConfig{Query: "data.example.deny", Modules: map[string]string{"main.rego": "package example\ndeny[msg] { input.resource_changes[_].change.actions[_] == \"delete\"; msg := \"deletion\" }"}}},
		{ID: "schema", On: TerraformPlan, Engine: JSONSchemaEngine, Effects: Effects{Deny: true}, Schema: &SchemaConfig{Entry: schemaBase + "plan.json", Resources: map[string]json.RawMessage{schemaBase + "plan.json": json.RawMessage(`{"properties":{"resource_changes":{"maxItems":0}}}`)}}},
	}
	decision, err := NewEngine().Evaluate(t.Context(), &Request{Kind: TerraformPlan, Input: input}, rules)
	require.NoError(t, err)
	require.Equal(t, Deny, decision.Status)
	require.Len(t, decision.Findings, 2)
	for _, finding := range decision.Findings {
		assert.Equal(t, Deny, finding.Status)
		assert.NotEmpty(t, finding.Diagnostics)
	}
}

func TestNewEngineManifestValidation(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, "sources"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "sources", "check.rego"), []byte("package example\ndeny = true"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "sources", "schema.json"), []byte(optionalOwnerSchema), 0o600))
	for _, definition := range []string{
		"engine: opa\n      opa: {query: data.example.deny, modules: [sources/check.rego]}",
		"engine: jsonschema\n      schema: {path: sources/schema.json}",
	} {
		manifest := "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: checks}\nspec:\n  policies:\n    example:\n      on: terraform.plan\n      action: deny\n      " + definition + "\n"
		manifestPath := filepath.Join(directory, "policy.yaml")
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifest), 0o600))
		bundle, err := LoadDirectory(directory)
		require.NoError(t, err)
		require.Len(t, bundle.Rules, 1)
		for _, invalid := range []string{manifest + "      rule: true\n", strings.ReplaceAll(manifest, "engine: opa", "engine: cel"), strings.ReplaceAll(manifest, "engine: jsonschema", "engine: starlark")} {
			if invalid == manifest {
				continue
			}
			require.NoError(t, os.WriteFile(manifestPath, []byte(invalid), 0o600))
			_, err := LoadDirectory(directory)
			require.Error(t, err)
		}
	}
}
