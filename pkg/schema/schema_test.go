package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestAtmosConfigurationWorksWithOpa(t *testing.T) {
	yamlString := `
schemas:
  opa:
    base_path: "some/random/path"
`
	atmosConfig := &AtmosConfiguration{}
	err := yaml.Unmarshal([]byte(yamlString), atmosConfig)
	assert.NoError(t, err)
	resourcePath := atmosConfig.GetResourcePath("opa")
	assert.Equal(t, "some/random/path", resourcePath.BasePath)
}

func TestAtmosConfigurationWithSchemas(t *testing.T) {
	yamlString := `
schemas:
  atmos:
    manifest: "some/random/path"
    matches:
      - hello
      - world
`
	atmosConfig := &AtmosConfiguration{}
	err := yaml.Unmarshal([]byte(yamlString), atmosConfig)
	assert.NoError(t, err)
	schemas := atmosConfig.GetSchemaRegistry("atmos")
	assert.Equal(t, "some/random/path", schemas.Manifest)
	assert.Equal(t, []string{"hello", "world"}, schemas.Matches)
}

// TestGetSchemaRegistry tests the GetSchemaRegistry method
func TestGetSchemaRegistry(t *testing.T) {
	config := AtmosConfiguration{
		Schemas: map[string]interface{}{
			"test": SchemaRegistry{
				Manifest: "test-manifest",
				Matches:  []string{"pattern1", "pattern2"},
			},
			"invalid": "not-a-schema",
		},
	}

	// Test valid schema
	schema := config.GetSchemaRegistry("test")
	assert.Equal(t, "test-manifest", schema.Manifest, "Manifest should match")
	assert.Equal(t, []string{"pattern1", "pattern2"}, schema.Matches, "Matches should match")

	// Test invalid schema
	schema = config.GetSchemaRegistry("invalid")
	assert.Equal(t, SchemaRegistry{}, schema, "Should return empty schema for invalid type")

	// Test non-existent key
	schema = config.GetSchemaRegistry("nonexistent")
	assert.Equal(t, SchemaRegistry{}, schema, "Should return empty schema for non-existent key")
}

// TestGetResourcePath tests the GetResourcePath method
func TestGetResourcePath(t *testing.T) {
	config := AtmosConfiguration{
		Schemas: map[string]interface{}{
			"opa":     ResourcePath{BasePath: "/opa/path"},
			"invalid": "not-a-resource",
		},
	}

	// Test valid resource path
	path := config.GetResourcePath("opa")
	assert.Equal(t, "/opa/path", path.BasePath, "Should return correct resource path")

	// Test invalid resource path
	path = config.GetResourcePath("invalid")
	assert.Equal(t, ResourcePath{}, path, "Should return empty resource path for invalid type")

	// Test non-existent key
	path = config.GetResourcePath("nonexistent")
	assert.Equal(t, ResourcePath{}, path, "Should return empty resource path for non-existent key")
}

// TestProcessSchemas tests the ProcessSchemas method
func TestProcessSchemas(t *testing.T) {
	config := AtmosConfiguration{
		Schemas: map[string]interface{}{
			"opa": map[string]interface{}{
				"base_path": "/opa/path",
			},
			"jsonschema": map[string]interface{}{
				"base_path": "/json/path",
			},
			"custom": map[string]interface{}{
				"manifest": "test-manifest",
				"matches":  []string{"pattern1", "pattern2"},
			},
		},
	}

	config.ProcessSchemas()

	// Verify opa schema
	opa, ok := config.Schemas["opa"].(ResourcePath)
	assert.True(t, ok, "opa should be ResourcePath")
	assert.Equal(t, "/opa/path", opa.BasePath, "opa base_path should match")

	// Verify jsonschema
	jsonSchema, ok := config.Schemas["jsonschema"].(ResourcePath)
	assert.True(t, ok, "jsonschema should be ResourcePath")
	assert.Equal(t, "/json/path", jsonSchema.BasePath, "jsonschema base_path should match")

	// Verify custom schema
	custom, ok := config.Schemas["custom"].(SchemaRegistry)
	assert.True(t, ok, "custom should be SchemaRegistry")
	assert.Equal(t, "test-manifest", custom.Manifest, "custom manifest should match")
	assert.Equal(t, []string{"pattern1", "pattern2"}, custom.Matches, "custom matches should match")
}

// TestProcessSchemasWithInvalidData tests ProcessSchemas with invalid schema data
func TestProcessSchemasWithInvalidData(t *testing.T) {
	config := AtmosConfiguration{
		Schemas: map[string]interface{}{
			"opa": []string{"invalid-data"},
			"custom": map[string]interface{}{
				"manifest": []string{"data"},
			},
		},
	}

	config.ProcessSchemas()

	// Verify schemas remain unchanged or are processed gracefully
	_, ok := config.Schemas["opa"].(ResourcePath)
	assert.False(t, ok, "opa should not be ResourcePath due to invalid data")

	_, ok = config.Schemas["custom"].(SchemaRegistry)
	assert.False(t, ok, "custom should not be SchemaRegistry due to invalid data")
}

// TestProcessSchemasWithEmptySchemas tests ProcessSchemas with empty schemas
func TestProcessSchemasWithEmptySchemas(t *testing.T) {
	config := AtmosConfiguration{
		Schemas: map[string]interface{}{},
	}

	config.ProcessSchemas()
	assert.Empty(t, config.Schemas, "Schemas should remain empty")
}
