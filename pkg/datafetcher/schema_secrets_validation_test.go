package datafetcher

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

// TestManifestSchema_SecretsDefinitionsExist verifies the embedded manifest schema contains the
// secrets definitions and references them from the component manifests. This mirrors the auth
// coverage in schema_auth_validation_test.go.
func TestManifestSchema_SecretsDefinitionsExist(t *testing.T) {
	data, err := (&atmosFetcher{}).FetchData("atmos://schema/atmos/manifest/1.0")
	require.NoError(t, err, "failed to fetch embedded manifest schema")

	var schemaMap map[string]any
	require.NoError(t, json.Unmarshal(data, &schemaMap), "failed to parse manifest schema JSON")

	definitions, ok := schemaMap["definitions"].(map[string]any)
	require.True(t, ok, "schema should have definitions")

	for _, def := range []string{
		"component_secrets",
		"secret_providers",
		"secret_provider",
		"secret_vars",
		"secret_declaration",
	} {
		_, ok := definitions[def].(map[string]any)
		assert.Truef(t, ok, "schema should have %q definition", def)
	}

	// Every component manifest must reference the secrets section, otherwise component-level
	// `secrets:` is rejected by additionalProperties:false.
	for _, manifest := range []string{
		"terraform_component_manifest",
		"helmfile_component_manifest",
		"packer_component_manifest",
	} {
		assert.Truef(t, componentManifestHasProperty(t, definitions, manifest, "secrets"),
			"%s should reference the 'secrets' property", manifest)
	}
}

// TestManifestSchema_ValidSecretsConfig validates realistic secrets configurations against the
// embedded schema: component-level declarations and stack-level providers must be accepted, while
// malformed secrets must be rejected.
func TestManifestSchema_ValidSecretsConfig(t *testing.T) {
	schemaData, err := (&atmosFetcher{}).FetchData("atmos://schema/atmos/manifest/1.0")
	require.NoError(t, err)

	tests := []struct {
		name      string
		manifest  map[string]any
		expectErr bool
	}{
		{
			name: "component declarations (store+reference, sops, required)",
			manifest: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"api": map[string]any{
							"secrets": map[string]any{
								"vars": map[string]any{
									"DATADOG_API_KEY": map[string]any{
										"description": "Datadog API key",
										"store":       "op",
										"reference":   "op://Shared/Datadog/api_key",
										"required":    true,
									},
									"REDIS_URL": map[string]any{
										"sops": "dev-sops",
									},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "stack-level sops provider",
			manifest: map[string]any{
				"secrets": map[string]any{
					"providers": map[string]any{
						"dev-sops": map[string]any{
							"kind": "sops/age",
							"spec": map[string]any{
								"file":         "secrets/dev.enc.yaml",
								"age_key_file": "secrets/keys.txt",
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "unknown key inside a secret declaration is rejected",
			manifest: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"api": map[string]any{
							"secrets": map[string]any{
								"vars": map[string]any{
									"BAD": map[string]any{
										"store":   "op",
										"unknown": "nope", // additionalProperties:false on secret_declaration.
									},
								},
							},
						},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "secret provider missing required kind is rejected",
			manifest: map[string]any{
				"secrets": map[string]any{
					"providers": map[string]any{
						"broken": map[string]any{
							"spec": map[string]any{
								"file": "secrets/dev.enc.yaml",
							},
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docJSON, err := json.Marshal(tt.manifest)
			require.NoError(t, err)

			result, err := gojsonschema.Validate(
				gojsonschema.NewBytesLoader(schemaData),
				gojsonschema.NewBytesLoader(docJSON),
			)
			require.NoError(t, err, "schema validation should not error")

			if tt.expectErr {
				assert.False(t, result.Valid(), "expected validation errors")
				return
			}
			if !result.Valid() {
				for _, desc := range result.Errors() {
					t.Logf("validation error: %s", desc)
				}
			}
			assert.True(t, result.Valid(), "expected valid document")
		})
	}
}

// componentManifestHasProperty reports whether the object variant of a component manifest
// definition declares the given property.
func componentManifestHasProperty(t *testing.T, definitions map[string]any, manifest, property string) bool {
	t.Helper()
	def, ok := definitions[manifest].(map[string]any)
	require.Truef(t, ok, "schema should define %s", manifest)
	_, present := objectVariantProps(def)[property]
	return present
}
