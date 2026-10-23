package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPlanManifestSchema(t *testing.T) {
	data, err := os.ReadFile("../datafetcher/schema/policy/policy/1.0.json")
	require.NoError(t, err)
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("policy.json", strings.NewReader(string(data))))
	schema, err := compiler.Compile("policy.json")
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(testPlanManifest), &document))
	require.NoError(t, schema.Validate(document))
	rule := document["spec"].(map[string]any)["policies"].(map[string]any)["no-deletes"].(map[string]any)
	rule["match"] = map[string]any{"labels": map[string]any{"stage": "prod"}, "tags": []any{"pci"}}
	require.NoError(t, schema.Validate(document))
	rule["match"] = map[string]any{"tag_mode": "invalid"}
	require.Error(t, schema.Validate(document))
	delete(rule, "match")
	rule["on"] = "atmos.affected"
	require.NoError(t, schema.Validate(document))
	rule["match"] = map[string]any{}
	require.Error(t, schema.Validate(document), "affected rules inspect before/after explicitly")
	delete(rule, "match")
	rule["on"] = "plugin"
	require.NoError(t, schema.Validate(document))
	rule["match"] = map[string]any{}
	require.Error(t, schema.Validate(document), "plugin rules inspect plugin metadata without an instance selector")
	delete(rule, "match")
	rule["on"] = "terraform.plan"
	document["metadata"].(map[string]any)["description"] = "Protect resources from accidental deletion."
	require.NoError(t, schema.Validate(document))
	document["metadata"].(map[string]any)["description"] = map[string]any{"invalid": "object"}
	require.Error(t, schema.Validate(document))
	root, entries := policyCatalogEntries(t)
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(root, entry, "policy.yaml"))
		require.NoError(t, err)
		var manifest map[string]any
		require.NoError(t, yaml.Unmarshal(data, &manifest))
		require.NoError(t, schema.Validate(manifest), entry)
	}
	for _, family := range []string{"terraform", "github", "lifecycle"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "examples", "policies", family, "policy.yaml"))
		require.NoError(t, err)
		var sample map[string]any
		require.NoError(t, yaml.Unmarshal(data, &sample))
		require.NoError(t, schema.Validate(sample), family)
	}
	for name, manifest := range map[string]string{
		"unknown field":  testPlanManifest + "      enforced: true\n",
		"unknown engine": testPlanManifest + "      engine: unknown\n",
		"warn":           strings.ReplaceAll(testPlanManifest, "action: deny", "action: warn"),
		"missing rule":   "apiVersion: atmos/v1\nkind: Policy\nmetadata: {name: test}\nspec: {policies: {}}",
	} {
		t.Run(name, func(t *testing.T) {
			var invalid map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(manifest), &invalid))
			require.Error(t, schema.Validate(invalid))
		})
	}
}
