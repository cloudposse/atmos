package doctor

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/datafetcher"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLocalManifestSchemaRecommendationBeforeAfter(t *testing.T) {
	for _, fromEnv := range []bool{false, true} {
		t.Run(map[bool]string{false: "configuration", true: "environment"}[fromEnv], func(t *testing.T) {
			root := newProject(t)
			t.Chdir(root)
			t.Setenv("ATMOS_SCHEMAS_ATMOS_MANIFEST", "")
			current, err := datafetcher.NewDataFetcher(&schema.AtmosConfiguration{}).GetData(stackSchema)
			require.NoError(t, err)
			var old map[string]any
			require.NoError(t, json.Unmarshal(current, &old))
			defs := old["definitions"].(map[string]any)
			props := defs["global_metadata"].(map[string]any)["oneOf"].([]any)[1].(map[string]any)["properties"].(map[string]any)
			require.Contains(t, props, "terraform_workspace_template")
			delete(props, "terraform_workspace_template")
			outdated, err := json.Marshal(old)
			require.NoError(t, err)
			local := projectFile(t, root, "manifest.json", string(outdated))
			config := "stacks:\n  base_path: stacks\nschemas:\n  custom:\n    schema: constraints.json\n    matches: [custom.yaml]\n"
			projectFile(t, root, "constraints.json", `{"type":"object","required":["required_by_custom_validator"]}`)
			projectFile(t, root, "custom.yaml", "unrelated: true\n")
			original := config
			if fromEnv {
				t.Setenv("ATMOS_SCHEMAS_ATMOS_MANIFEST", "manifest.json")
			} else {
				original += "  atmos:\n    manifest: manifest.json\n"
			}
			configPath := projectFile(t, root, "atmos.yaml", original)
			stackPath := projectFile(t, root, "stacks/dev.yaml", "metadata:\n  terraform_workspace_template: '{{ .atmos_component }}'\ncomponents:\n  terraform:\n    app: {}\n")
			e := testEngine(t)
			o := Options{Root: root, Only: []string{ruleSchema, ruleLocalAtmosSchema}, Offline: true, Fix: true}
			before, err := e.Run(context.Background(), o)
			require.NoError(t, err)
			findings := ecFindings(&before, ruleLocalAtmosSchema)
			require.Len(t, findings, 1)
			require.Contains(t, findings[0].Reason, "differs from the bundled schema")
			require.False(t, findings[0].Fixable)
			require.Empty(t, before.ChangedFiles)
			require.NotEmpty(t, ecFindings(&before, ruleSchema))
			bytes, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.Equal(t, original, string(bytes))
			// Apply the recommendation manually: neither the local copy nor unrelated rules are removed.
			t.Setenv("ATMOS_SCHEMAS_ATMOS_MANIFEST", "")
			require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))
			after, err := e.Run(context.Background(), o)
			require.NoError(t, err)
			require.Empty(t, ecFindings(&after, ruleLocalAtmosSchema))
			require.FileExists(t, local)
			customStillFails := false
			for _, f := range ecFindings(&after, ruleSchema) {
				require.NotContains(t, f.File, "stacks/dev.yaml")
				if f.File == "custom.yaml" {
					customStillFails = true
				}
			}
			require.True(t, customStillFails, "%+v", after.Findings)
			require.NoError(t, os.WriteFile(stackPath, []byte("metadata:\n  inherits: [invalid-at-global-scope]\n"), 0o600))
			invalid, err := e.Run(context.Background(), o)
			require.NoError(t, err)
			found := false
			for _, f := range ecFindings(&invalid, ruleSchema) {
				if f.File == "stacks/dev.yaml" {
					found = true
				}
			}
			require.True(t, found, "Bundled validation must still reject invalid manifests")
		})
	}
}

func TestLocalSchemaIdentityAndCustomConstraints(t *testing.T) {
	for _, source := range []string{stackSchema, configSchema} {
		t.Run(source, func(t *testing.T) {
			root := newProject(t)
			content, err := datafetcher.NewDataFetcher(&schema.AtmosConfiguration{}).GetData(source)
			require.NoError(t, err)
			projectFile(t, root, "schema.json", string(content))
			e := testEngine(t)
			require.NoError(t, e.configure(&Options{Root: root}))
			p := &project{root: root, base: root}
			f := e.localSchemaFinding(p, "schema.json", source)
			require.Contains(t, f.Reason, "identical to the bundled schema")
			projectFile(t, root, "schema.json", `{"$id":"https://example.org/custom","type":"object","required":["owner"]}`)
			f = e.localSchemaFinding(p, "schema.json", source)
			require.Contains(t, f.Message, "Custom local schema")
			require.Contains(t, f.Remediation, "keep this file")
			require.False(t, f.Fixable)
		})
	}
}

func TestLocalConfigSchemaRecommendationPreservesMatches(t *testing.T) {
	root := newProject(t)
	t.Chdir(root)
	current, err := datafetcher.NewDataFetcher(&schema.AtmosConfiguration{}).GetData(configSchema)
	require.NoError(t, err)
	var old map[string]any
	require.NoError(t, json.Unmarshal(current, &old))
	// A pinned schema without a supported section rejects a valid configuration fragment.
	old["properties"].(map[string]any)["stacks"] = false
	content, err := json.Marshal(old)
	require.NoError(t, err)
	local := projectFile(t, root, "config-schema.json", string(content))
	original := "schemas:\n  config:\n    schema: config-schema.json\n    matches: [extra.yaml]\n  jsonschema:\n    base_path: component-schemas\n  opa:\n    base_path: policies\n"
	config := projectFile(t, root, "atmos.yaml", original)
	fragment := projectFile(t, root, "extra.yaml", "stacks:\n  name_template: '{{ .vars.stage }}'\n")
	e := testEngine(t)
	o := Options{Root: root, Only: []string{ruleSchema, ruleLocalAtmosSchema}, Offline: true, Fix: true}
	before, err := e.Run(context.Background(), o)
	require.NoError(t, err)
	require.Len(t, ecFindings(&before, ruleLocalAtmosSchema), 1)
	require.NotEmpty(t, ecFindings(&before, ruleSchema))
	require.Empty(t, before.ChangedFiles)
	// Explicit bundled URI retains custom file matching and independent policies.
	replacement := strings.Replace(original, "schema: config-schema.json", "schema: "+configSchema, 1)
	require.NoError(t, os.WriteFile(config, []byte(replacement), 0o600))
	after, err := e.Run(context.Background(), o)
	require.NoError(t, err)
	require.Empty(t, ecFindings(&after, ruleLocalAtmosSchema))
	require.Empty(t, ecFindings(&after, ruleSchema), "%+v", after.Findings)
	require.FileExists(t, local)
	require.NoError(t, os.WriteFile(fragment, []byte("stacks:\n  name_template: [invalid]\n"), 0o600))
	invalid, err := e.Run(context.Background(), o)
	require.NoError(t, err)
	require.NotEmpty(t, ecFindings(&invalid, ruleSchema), "Custom matches must remain active after the recommendation")
}
