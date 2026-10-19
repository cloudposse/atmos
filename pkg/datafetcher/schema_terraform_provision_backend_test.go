package datafetcher

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

// validateManifestAgainstEmbeddedSchema validates a manifest map against the embedded manifest schema.
func validateManifestAgainstEmbeddedSchema(t *testing.T, manifest map[string]any) *gojsonschema.Result {
	t.Helper()

	docJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	schemaJSON, err := json.Marshal(loadEmbeddedSchema(t))
	require.NoError(t, err)

	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schemaJSON),
		gojsonschema.NewBytesLoader(docJSON),
	)
	require.NoError(t, err, "schema validation should not error")

	return result
}

// terraformComponentSection builds the settings shared by the component-level and
// section-level terraform fixtures below.
func terraformComponentSection(backendType any, provisionBackend map[string]any) map[string]any {
	section := map[string]any{
		"provision": map[string]any{"backend": provisionBackend},
	}
	if backendType != nil {
		section["backend_type"] = backendType
	}
	return section
}

func TestManifestSchema_ProvisionBackendBucketNamespaceScopedToS3(t *testing.T) {
	withNamespace := map[string]any{"enabled": true, "bucket_namespace": "any-value"}
	withoutNamespace := map[string]any{"enabled": true}

	tests := []struct {
		name        string
		backendType any
		backend     map[string]any
		wantValid   bool
	}{
		{name: "s3 accepts bucket_namespace", backendType: "s3", backend: withNamespace, wantValid: true},
		{name: "unset backend_type is not checked", backendType: nil, backend: withNamespace, wantValid: true},
		{name: "included backend_type is not checked", backendType: "!include backend-type.yaml", backend: withNamespace, wantValid: true},
		{name: "azurerm rejects bucket_namespace", backendType: "azurerm", backend: withNamespace, wantValid: false},
		{name: "gcs rejects bucket_namespace", backendType: "gcs", backend: withNamespace, wantValid: false},
		{name: "local rejects bucket_namespace", backendType: "local", backend: withNamespace, wantValid: false},
		// Negative path: the restriction applies only to bucket_namespace, not to other provisioning settings.
		{name: "azurerm without bucket_namespace stays valid", backendType: "azurerm", backend: withoutNamespace, wantValid: true},
		{name: "s3 rejects non-string bucket_namespace", backendType: "s3", backend: map[string]any{"bucket_namespace": 42}, wantValid: false},
	}

	for _, tt := range tests {
		t.Run("component: "+tt.name, func(t *testing.T) {
			manifest := map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": terraformComponentSection(tt.backendType, tt.backend),
					},
				},
			}

			result := validateManifestAgainstEmbeddedSchema(t, manifest)
			for _, desc := range result.Errors() {
				t.Logf("validation error: %s", desc)
			}
			assert.Equal(t, tt.wantValid, result.Valid())
		})

		t.Run("terraform section: "+tt.name, func(t *testing.T) {
			manifest := map[string]any{
				"terraform": terraformComponentSection(tt.backendType, tt.backend),
			}

			result := validateManifestAgainstEmbeddedSchema(t, manifest)
			for _, desc := range result.Errors() {
				t.Logf("validation error: %s", desc)
			}
			assert.Equal(t, tt.wantValid, result.Valid())
		})
	}
}
