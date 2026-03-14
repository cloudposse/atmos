package datafetcher

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

// TestManifestSchema_AuthDefinitionExists verifies the embedded manifest schema
// contains the auth definition with the needs field.
func TestManifestSchema_AuthDefinitionExists(t *testing.T) {
	fetcher := &atmosFetcher{}
	data, err := fetcher.FetchData("atmos://schema/atmos/manifest/1.0")
	require.NoError(t, err, "Failed to fetch embedded manifest schema")

	var schemaMap map[string]interface{}
	err = json.Unmarshal(data, &schemaMap)
	require.NoError(t, err, "Failed to parse manifest schema JSON")

	definitions, ok := schemaMap["definitions"].(map[string]interface{})
	require.True(t, ok, "Schema should have definitions")

	// Verify component_auth definition exists.
	auth, ok := definitions["component_auth"].(map[string]interface{})
	require.True(t, ok, "Schema should have 'component_auth' definition")
	assert.Equal(t, "component_auth", auth["title"])

	// Verify auth is referenced from terraform_component_manifest.
	tfManifest, ok := definitions["terraform_component_manifest"].(map[string]interface{})
	require.True(t, ok, "Schema should have terraform_component_manifest")

	oneOf := tfManifest["oneOf"].([]interface{})
	// Find the object variant (not the !include string variant).
	var objectVariant map[string]interface{}
	for _, v := range oneOf {
		variant := v.(map[string]interface{})
		if variant["type"] == "object" {
			objectVariant = variant
			break
		}
	}
	require.NotNil(t, objectVariant, "terraform_component_manifest should have an object variant")

	props := objectVariant["properties"].(map[string]interface{})
	_, hasAuth := props["auth"]
	assert.True(t, hasAuth, "terraform_component_manifest should have 'auth' property")
}

// TestManifestSchema_AuthIdentityRequiredField verifies the auth_identity definition
// includes the required field with correct type constraint.
func TestManifestSchema_AuthIdentityRequiredField(t *testing.T) {
	fetcher := &atmosFetcher{}
	data, err := fetcher.FetchData("atmos://schema/atmos/manifest/1.0")
	require.NoError(t, err)

	var schemaMap map[string]interface{}
	err = json.Unmarshal(data, &schemaMap)
	require.NoError(t, err)

	definitions := schemaMap["definitions"].(map[string]interface{})
	identity := definitions["auth_identity"].(map[string]interface{})
	oneOf := identity["oneOf"].([]interface{})

	// Find the object variant.
	var objectVariant map[string]interface{}
	for _, v := range oneOf {
		variant := v.(map[string]interface{})
		if variant["type"] == "object" {
			objectVariant = variant
			break
		}
	}
	require.NotNil(t, objectVariant)

	props := objectVariant["properties"].(map[string]interface{})

	// Verify required field.
	requiredProp, ok := props["required"].(map[string]interface{})
	require.True(t, ok, "auth_identity should have 'required' property")
	assert.Equal(t, "boolean", requiredProp["type"], "required should be a boolean")

	// Verify other identity fields exist.
	_, hasDefault := props["default"]
	assert.True(t, hasDefault, "auth_identity should have 'default' property")

	_, hasKind := props["kind"]
	assert.True(t, hasKind, "auth_identity should have 'kind' property")
}

// TestManifestSchema_ValidAuthConfig validates a realistic auth config
// with needs against the embedded JSON schema.
func TestManifestSchema_ValidAuthConfig(t *testing.T) {
	fetcher := &atmosFetcher{}
	schemaData, err := fetcher.FetchData("atmos://schema/atmos/manifest/1.0")
	require.NoError(t, err)

	tests := []struct {
		name      string
		manifest  map[string]interface{}
		expectErr bool
	}{
		{
			name: "component with required identities",
			manifest: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"vpc-peering": map[string]interface{}{
							"vars": map[string]interface{}{
								"enabled": true,
							},
							"auth": map[string]interface{}{
								"providers": map[string]interface{}{
									"github-oidc": map[string]interface{}{
										"kind": "github/oidc",
									},
								},
								"identities": map[string]interface{}{
									"core-network/terraform": map[string]interface{}{
										"kind":     "aws/assume-role",
										"default":  true,
										"required": true,
									},
									"plat-prod/terraform": map[string]interface{}{
										"kind":     "aws/assume-role",
										"required": true,
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
			name: "component with empty auth",
			manifest: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"simple": map[string]interface{}{
							"vars": map[string]interface{}{},
							"auth": map[string]interface{}{},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "component without auth",
			manifest: map[string]interface{}{
				"components": map[string]interface{}{
					"terraform": map[string]interface{}{
						"basic": map[string]interface{}{
							"vars": map[string]interface{}{},
						},
					},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docJSON, err := json.Marshal(tt.manifest)
			require.NoError(t, err)

			schemaLoader := gojsonschema.NewBytesLoader(schemaData)
			docLoader := gojsonschema.NewBytesLoader(docJSON)

			result, err := gojsonschema.Validate(schemaLoader, docLoader)
			require.NoError(t, err, "Schema validation should not error")

			if tt.expectErr {
				assert.False(t, result.Valid(), "Expected validation errors")
			} else {
				if !result.Valid() {
					for _, desc := range result.Errors() {
						t.Logf("Validation error: %s", desc)
					}
				}
				assert.True(t, result.Valid(), "Expected valid document")
			}
		})
	}
}
