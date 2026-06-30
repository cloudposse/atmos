package datafetcher

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

func TestManifestSchema_WorkflowWhenConditionForms(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":      loadEmbeddedSchemaBytes(t),
		"website":       loadWebsiteSchemaBytes(t),
		"fixture":       loadFixtureSchemaBytes(t),
		"global-config": loadSchemaFile(t, "schema/config/global/1.0.json"),
	}

	validConditions := map[string]any{
		"scalar":     "ci",
		"list":       []any{"ci", "success"},
		"all":        map[string]any{"all": []any{"ci", "success"}},
		"all-scalar": map[string]any{"all": "ci"},
		"any":        map[string]any{"any": []any{"ci", "local"}},
		"not":        map[string]any{"not": "ci"},
	}

	for schemaName, schemaData := range schemas {
		for name, condition := range validConditions {
			t.Run(schemaName+"/"+name, func(t *testing.T) {
				assertSchemaValid(t, schemaData, workflowManifestWithWhen(condition))
			})
		}

		t.Run(schemaName+"/rejects unknown predicate", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, workflowManifestWithWhen("expr"))
		})

		t.Run(schemaName+"/rejects failure predicate", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, workflowManifestWithWhen("failure"))
		})
	}
}

func TestManifestSchema_HookWhenConditionForms(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":      loadEmbeddedSchemaBytes(t),
		"website":       loadWebsiteSchemaBytes(t),
		"fixture":       loadFixtureSchemaBytes(t),
		"global-config": loadSchemaFile(t, "schema/config/global/1.0.json"),
		"stack-config":  loadStackConfigSchemaBytes(t),
	}

	validConditions := map[string]any{
		"success":    "success",
		"failure":    "failure",
		"always":     "always",
		"ci":         "ci",
		"ci-always":  []any{"ci", "always"},
		"all-scalar": map[string]any{"all": "ci"},
		"compound":   map[string]any{"all": []any{"ci", map[string]any{"not": "never"}}},
	}

	for schemaName, schemaData := range schemas {
		for name, condition := range validConditions {
			t.Run(schemaName+"/"+name, func(t *testing.T) {
				assertSchemaValid(t, schemaData, hookManifestWithWhen(condition))
			})
		}

		t.Run(schemaName+"/rejects unknown predicate", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, hookManifestWithWhen("expr"))
		})
	}
}

func TestManifestSchema_HookRetryUsesWorkflowRetrySchema(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":      loadEmbeddedSchemaBytes(t),
		"website":       loadWebsiteSchemaBytes(t),
		"fixture":       loadFixtureSchemaBytes(t),
		"global-config": loadSchemaFile(t, "schema/config/global/1.0.json"),
		"stack-config":  loadStackConfigSchemaBytes(t),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName+"/valid retry", func(t *testing.T) {
			assertSchemaValid(t, schemaData, hookManifestWithRetry(map[string]any{
				"max_attempts":  2,
				"initial_delay": "1s",
			}))
		})

		t.Run(schemaName+"/rejects unknown retry field", func(t *testing.T) {
			assertSchemaInvalid(t, schemaData, hookManifestWithRetry(map[string]any{
				"unknown": true,
			}))
		})
	}
}

func TestManifestSchema_TerraformTestFixturesHookShape(t *testing.T) {
	schemas := map[string][]byte{
		"embedded":      loadEmbeddedSchemaBytes(t),
		"website":       loadWebsiteSchemaBytes(t),
		"fixture":       loadFixtureSchemaBytes(t),
		"global-config": loadSchemaFile(t, "schema/config/global/1.0.json"),
	}

	for schemaName, schemaData := range schemas {
		t.Run(schemaName, func(t *testing.T) {
			assertSchemaValid(t, schemaData, terraformTestFixturesManifest())
		})
	}
}

func workflowManifestWithWhen(condition any) map[string]any {
	return map[string]any{
		"workflows": map[string]any{
			"test": map[string]any{
				"steps": []any{
					map[string]any{
						"command": "echo ok",
						"when":    condition,
					},
				},
			},
		},
	}
}

func hookManifestWithWhen(condition any) map[string]any {
	return map[string]any{
		"hooks": map[string]any{
			"test": map[string]any{
				"kind":    "command",
				"command": "echo",
				"when":    condition,
			},
		},
	}
}

func hookManifestWithRetry(retry any) map[string]any {
	manifest := hookManifestWithWhen("always")
	manifest["hooks"].(map[string]any)["test"].(map[string]any)["retry"] = retry
	return manifest
}

func terraformTestFixturesManifest() map[string]any {
	return map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"app": map[string]any{
					"metadata": map[string]any{
						"type": "real",
					},
					"hooks": map[string]any{
						"test-fixtures-up": map[string]any{
							"kind": "steps",
							"events": []any{
								"before.terraform.test",
							},
							"with": []any{
								map[string]any{
									"type":      "emulator",
									"component": "aws",
									"action":    "up",
								},
								map[string]any{
									"type":    "atmos",
									"command": "terraform apply vpc -s fixtures -auto-approve",
								},
							},
						},
					},
					"test": map[string]any{
						"vars": map[string]any{
							"fixture_vpc_id": "vpc-123",
						},
					},
				},
			},
		},
	}
}

func loadEmbeddedSchemaBytes(t *testing.T) []byte {
	t.Helper()

	data, err := (&atmosFetcher{}).FetchData("atmos://schema/atmos/manifest/1.0")
	require.NoError(t, err)
	return data
}

func loadWebsiteSchemaBytes(t *testing.T) []byte {
	t.Helper()
	return loadSchemaFile(t, "../../website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json")
}

func loadFixtureSchemaBytes(t *testing.T) []byte {
	t.Helper()
	return loadSchemaFile(t, "../../tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json")
}

func loadStackConfigSchemaBytes(t *testing.T) []byte {
	t.Helper()
	return loadSchemaFile(t, "schema/stacks/stack-config/1.0.json")
}

func loadSchemaFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func assertSchemaValid(t *testing.T, schemaData []byte, manifest map[string]any) {
	t.Helper()

	result := validateManifestAgainstSchema(t, schemaData, manifest)
	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Logf("validation error: %s", desc)
		}
	}
	assert.True(t, result.Valid(), "expected valid manifest")
}

func assertSchemaInvalid(t *testing.T, schemaData []byte, manifest map[string]any) {
	t.Helper()

	result := validateManifestAgainstSchema(t, schemaData, manifest)
	assert.False(t, result.Valid(), "expected invalid manifest")
}

func validateManifestAgainstSchema(t *testing.T, schemaData []byte, manifest map[string]any) *gojsonschema.Result {
	t.Helper()

	docJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schemaData),
		gojsonschema.NewBytesLoader(docJSON),
	)
	require.NoError(t, err, "schema validation should not error")
	return result
}
