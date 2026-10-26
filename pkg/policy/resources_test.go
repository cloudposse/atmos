package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestLocalResourceLimits(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "data.json"), []byte(`{}`), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "directory"), 0o700))
	for _, tc := range []struct {
		name, directory, path string
		limit                 int64
	}{
		{"missing root", filepath.Join(root, "missing"), "data.json", 2},
		{"missing file", root, "missing.json", 2},
		{"directory", root, "directory", 2},
		{"oversized", root, "data.json", 1},
		{"absolute path", root, filepath.Join(root, "data.json"), 2},
		{"parent traversal", root, "../data.json", 2},
		{"backslash", root, `nested\data.json`, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := readLocalResource(tc.directory, tc.path, tc.limit)
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
			require.Nil(t, raw)
		})
	}
	raw, err := readLocalResource(root, "data.json", 2)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(raw))
}

func TestEvaluatorResourceFailures(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "schema.json"), []byte(`{}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "invalid.rego"), []byte("not a policy"), 0o600))
	for _, tc := range []struct {
		name, engine string
		item         planPolicyItem
	}{
		{"missing OPA config", OPAEngine, planPolicyItem{}},
		{"missing schema config", JSONSchemaEngine, planPolicyItem{}},
		{"OPA config on CEL", CELEngine, planPolicyItem{OPA: &opaPolicyConfig{}}},
		{"schema config on CEL", CELEngine, planPolicyItem{Schema: &schemaPolicyConfig{}}},
		{"expression and OPA", OPAEngine, planPolicyItem{Rule: "true", OPA: &opaPolicyConfig{}}},
		{"expression and schema", JSONSchemaEngine, planPolicyItem{Rule: "true", Schema: &schemaPolicyConfig{}}},
		{"missing module", OPAEngine, planPolicyItem{OPA: &opaPolicyConfig{Query: "data.test.deny", Modules: []string{"missing.rego"}}}},
		{"invalid module", OPAEngine, planPolicyItem{OPA: &opaPolicyConfig{Query: "data.test.deny", Modules: []string{"invalid.rego"}}}},
		{"missing schema", JSONSchemaEngine, planPolicyItem{Schema: &schemaPolicyConfig{Path: "missing.json"}}},
		{"invalid schema target", JSONSchemaEngine, planPolicyItem{Schema: &schemaPolicyConfig{Path: "schema.json", Target: "env"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := Rule{Engine: tc.engine}
			require.ErrorIs(t, tc.item.resolveEvaluator(root, &rule), errUtils.ErrPolicyInvalid)
		})
	}
}

func TestMaterializedResourceBudgets(t *testing.T) {
	root := t.TempDir()
	module := "package test\ndeny = false\n#" + strings.Repeat("x", maxPolicyDocumentBytes/2)
	for _, name := range []string{"first.rego", "second.rego"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(module), 0o600))
	}
	_, err := loadOPAResources(root, &opaPolicyConfig{Query: "data.test.deny", Modules: []string{"first.rego", "second.rego"}})
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	require.ErrorContains(t, err, "modules exceed size limit")

	raw := []byte(`{"description":"` + strings.Repeat("x", maxPolicyDocumentBytes/2) + `"}`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "second.json"), raw, 0o600))
	config := &SchemaConfig{Resources: map[string]json.RawMessage{schemaBase + "first.json": raw}}
	reader, err := config.loadResource(root, schemaBase+"second.json")
	require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
	require.ErrorContains(t, err, "resources exceed size limit")
	require.Nil(t, reader)
	require.NotContains(t, config.Resources, schemaBase+"second.json")
}

func TestSchemaReferenceConfinement(t *testing.T) {
	root := t.TempDir()
	for _, uri := range []string{
		"%", "http://atmos-policy.invalid/schema.json", "file:///schema.json",
		"https://example.com/schema.json", schemaBase + "schema.json?query=1",
		"https://user@atmos-policy.invalid/schema.json", schemaBase + "../schema.json",
	} {
		t.Run(uri, func(t *testing.T) {
			config := &SchemaConfig{Resources: map[string]json.RawMessage{}}
			reader, err := config.loadResource(root, uri)
			require.ErrorIs(t, err, errUtils.ErrPolicyInvalid)
			require.Nil(t, reader)
			require.Empty(t, config.Resources)
		})
	}
}
