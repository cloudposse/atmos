package policy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestJSONSchemaInvalidResources(t *testing.T) {
	entry := schemaBase + "schema.json"
	for _, tc := range []struct {
		name   string
		config *SchemaConfig
	}{
		{"nil", nil},
		{"missing entry", &SchemaConfig{Resources: map[string]json.RawMessage{entry: json.RawMessage(`{}`)}}},
		{"empty resources", &SchemaConfig{Entry: entry}},
		{"invalid target", &SchemaConfig{Entry: entry, Target: "env", Resources: map[string]json.RawMessage{entry: json.RawMessage(`{}`)}}},
		{"malformed JSON", &SchemaConfig{Entry: entry, Resources: map[string]json.RawMessage{entry: json.RawMessage(`{`)}}},
		{"invalid schema", &SchemaConfig{Entry: entry, Resources: map[string]json.RawMessage{entry: json.RawMessage(`{"type":42}`)}}},
		{"unmaterialized reference", &SchemaConfig{Entry: entry, Resources: map[string]json.RawMessage{entry: json.RawMessage(`{"$ref":"missing.json"}`)}}},
		{"oversized", &SchemaConfig{Entry: entry, Resources: map[string]json.RawMessage{entry: json.RawMessage(`{"description":"` + strings.Repeat("x", maxPolicyDocumentBytes) + `"}`)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (&JSONSchema{}).EvaluateDetailed(t.Context(), Evaluation{Schema: tc.config, Request: &Request{Kind: File, Input: json.RawMessage(`{}`)}})
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
			require.False(t, result.Matched)
			require.Empty(t, result.Diagnostics)
		})
	}
}

func TestOPAInvalidConfiguration(t *testing.T) {
	module := map[string]string{"policy.rego": "package test\ndeny = true"}
	for _, tc := range []struct {
		name   string
		config *OPAConfig
	}{
		{"nil", nil},
		{"empty query", &OPAConfig{Query: "  ", Modules: module}},
		{"oversized query", &OPAConfig{Query: strings.Repeat("x", maxExpressionBytes+1), Modules: module}},
		{"missing modules", &OPAConfig{Query: "data.test.deny"}},
		{"unsupported version", &OPAConfig{Query: "data.test.deny", Modules: module, RegoVersion: 2}},
		{"oversized modules", &OPAConfig{Query: "data.test.deny", Modules: map[string]string{"large.rego": strings.Repeat("x", maxPolicyDocumentBytes+1)}}},
		{"invalid source", &OPAConfig{Query: "data.test.deny", Modules: map[string]string{"broken.rego": "package test\ndeny ="}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (&OPA{}).EvaluateDetailed(t.Context(), Evaluation{OPA: tc.config, Request: &Request{Kind: File, Input: json.RawMessage(`{}`)}})
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
			require.False(t, result.Matched)
			require.Empty(t, result.Diagnostics)
		})
	}
}

func TestValidationEnginesRejectInvalidParameters(t *testing.T) {
	request := &Request{Kind: File, Input: json.RawMessage(`{}`)}
	for name, evaluator := range map[string]DetailedEvaluator{"opa": &OPA{}, "schema": &JSONSchema{}} {
		t.Run(name, func(t *testing.T) {
			result, err := evaluator.EvaluateDetailed(t.Context(), Evaluation{
				Request: request, Parameters: json.RawMessage(`{"broken":`),
				Schema: &SchemaConfig{Entry: schemaBase + "schema.json", Resources: map[string]json.RawMessage{schemaBase + "schema.json": json.RawMessage(`{}`)}},
			})
			require.ErrorIs(t, err, errUtils.ErrPolicyInputInvalid)
			require.False(t, result.Matched)
		})
	}
}

func TestOPADiagnosticObjects(t *testing.T) {
	config := &OPAConfig{Query: "data.test.deny", Modules: map[string]string{
		"policy.rego": `package test
deny = [{"message": "primary", "msg": "secondary", "path": "/owner"}, {"resource": "bucket"}]`,
	}}
	result, err := (&OPA{}).EvaluateDetailed(t.Context(), Evaluation{OPA: config, Request: &Request{Kind: File, Input: json.RawMessage(`{}`)}})
	require.NoError(t, err)
	require.True(t, result.Matched)
	require.Len(t, result.Diagnostics, 2)
	byMessage := map[string]Diagnostic{}
	for _, diagnostic := range result.Diagnostics {
		byMessage[diagnostic.Message] = diagnostic
	}
	require.Equal(t, "/owner", byMessage["primary"].Path)
	require.JSONEq(t, `{"message":"primary","msg":"secondary","path":"/owner"}`, string(byMessage["primary"].Details))
	require.JSONEq(t, `{"resource":"bucket"}`, string(byMessage[`{"resource":"bucket"}`].Details))
}
