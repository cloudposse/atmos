package datafetcher

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

func TestManifestSchema_MetadataDescriptionExists(t *testing.T) {
	schemaMap := loadEmbeddedSchema(t)

	definitions, ok := schemaMap["definitions"].(map[string]any)
	require.True(t, ok, "schema should have definitions")

	metadata, ok := definitions["metadata"].(map[string]any)
	require.True(t, ok, "schema should define metadata")

	props := objectVariantProps(metadata)
	_, hasDescription := props["description"]
	assert.True(t, hasDescription, "metadata should allow description")
}

func TestManifestSchema_ValidMetadataDescription(t *testing.T) {
	manifest := map[string]any{
		"components": map[string]any{
			"terraform": map[string]any{
				"vpc": map[string]any{
					"metadata": map[string]any{
						"component":   "vpc",
						"description": "Virtual Private Cloud with subnets and NAT gateway",
					},
					"vars": map[string]any{
						"enabled": true,
					},
				},
			},
		},
	}

	docJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	schemaJSON, err := json.Marshal(loadEmbeddedSchema(t))
	require.NoError(t, err)

	result, err := gojsonschema.Validate(
		gojsonschema.NewBytesLoader(schemaJSON),
		gojsonschema.NewBytesLoader(docJSON),
	)
	require.NoError(t, err, "schema validation should not error")

	if !result.Valid() {
		for _, desc := range result.Errors() {
			t.Logf("validation error: %s", desc)
		}
	}
	assert.True(t, result.Valid(), "metadata.description should be valid")
}
