package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProcessSchemas_ResourceSchemas tests ProcessSchemas with resource schema keys (cue, opa, jsonschema).
func TestProcessSchemas_ResourceSchemas(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"cue": map[string]any{
				"base_path": "/path/to/cue",
			},
			"opa": map[string]any{
				"base_path": "/path/to/opa",
			},
			"jsonschema": map[string]any{
				"base_path": "/path/to/jsonschema",
			},
		},
	}

	atmosConfig.ProcessSchemas()

	// Verify cue was processed as ResourcePath
	cueResource, ok := atmosConfig.Schemas["cue"].(ResourcePath)
	assert.True(t, ok, "cue should be converted to ResourcePath")
	assert.Equal(t, "/path/to/cue", cueResource.BasePath)

	// Verify opa was processed as ResourcePath
	opaResource, ok := atmosConfig.Schemas["opa"].(ResourcePath)
	assert.True(t, ok, "opa should be converted to ResourcePath")
	assert.Equal(t, "/path/to/opa", opaResource.BasePath)

	// Verify jsonschema was processed as ResourcePath
	jsonschemaResource, ok := atmosConfig.Schemas["jsonschema"].(ResourcePath)
	assert.True(t, ok, "jsonschema should be converted to ResourcePath")
	assert.Equal(t, "/path/to/jsonschema", jsonschemaResource.BasePath)
}

// TestProcessSchemas_ManifestSchemas tests ProcessSchemas with manifest schema keys (atmos, vendor, etc.).
func TestProcessSchemas_ManifestSchemas(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"atmos": map[string]any{
				"manifest": "/path/to/atmos/manifest",
				"schema":   "/path/to/atmos/schema",
				"matches":  []any{"*.yaml", "*.yml"},
			},
			"vendor": map[string]any{
				"manifest": "/path/to/vendor/manifest",
				"matches":  []any{"vendor/*.yaml"},
			},
		},
	}

	atmosConfig.ProcessSchemas()

	// Verify atmos was processed as SchemaRegistry
	atmosRegistry, ok := atmosConfig.Schemas["atmos"].(SchemaRegistry)
	assert.True(t, ok, "atmos should be converted to SchemaRegistry")
	assert.Equal(t, "/path/to/atmos/manifest", atmosRegistry.Manifest)
	assert.Equal(t, "/path/to/atmos/schema", atmosRegistry.Schema)
	assert.Equal(t, []string{"*.yaml", "*.yml"}, atmosRegistry.Matches)

	// Verify vendor was processed as SchemaRegistry
	vendorRegistry, ok := atmosConfig.Schemas["vendor"].(SchemaRegistry)
	assert.True(t, ok, "vendor should be converted to SchemaRegistry")
	assert.Equal(t, "/path/to/vendor/manifest", vendorRegistry.Manifest)
	assert.Equal(t, []string{"vendor/*.yaml"}, vendorRegistry.Matches)
}

// TestProcessSchemas_MixedSchemas tests ProcessSchemas with both resource and manifest schemas.
func TestProcessSchemas_MixedSchemas(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"opa": map[string]any{
				"base_path": "/path/to/opa",
			},
			"atmos": map[string]any{
				"manifest": "/path/to/atmos",
				"matches":  []any{"*.yaml"},
			},
			"cue": map[string]any{
				"base_path": "/path/to/cue",
			},
		},
	}

	atmosConfig.ProcessSchemas()

	// Verify resource schemas
	opaResource, ok := atmosConfig.Schemas["opa"].(ResourcePath)
	assert.True(t, ok)
	assert.Equal(t, "/path/to/opa", opaResource.BasePath)

	cueResource, ok := atmosConfig.Schemas["cue"].(ResourcePath)
	assert.True(t, ok)
	assert.Equal(t, "/path/to/cue", cueResource.BasePath)

	// Verify manifest schema
	atmosRegistry, ok := atmosConfig.Schemas["atmos"].(SchemaRegistry)
	assert.True(t, ok)
	assert.Equal(t, "/path/to/atmos", atmosRegistry.Manifest)
}

// TestProcessSchemas_EmptySchemas tests ProcessSchemas with empty Schemas map.
func TestProcessSchemas_EmptySchemas(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{},
	}

	// Should not panic
	atmosConfig.ProcessSchemas()

	assert.Empty(t, atmosConfig.Schemas)
}

// TestProcessSchemas_NilSchemas tests ProcessSchemas with nil Schemas map.
func TestProcessSchemas_NilSchemas(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: nil,
	}

	// Should not panic
	atmosConfig.ProcessSchemas()
}

// TestProcessManifestSchemas_NonExistentKey tests processManifestSchemas when key doesn't exist.
func TestProcessManifestSchemas_NonExistentKey(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"other": map[string]any{
				"manifest": "/path/to/other",
			},
		},
	}

	// Call with non-existent key - should return early without error
	atmosConfig.processManifestSchemas("nonexistent")

	// Original schema should be unchanged
	assert.Contains(t, atmosConfig.Schemas, "other")
	assert.NotContains(t, atmosConfig.Schemas, "nonexistent")
}

// TestProcessManifestSchemas_MarshalError tests processManifestSchemas with unmarshalable data.
func TestProcessManifestSchemas_MarshalError(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"atmos": make(chan int), // Channels cannot be marshaled to JSON
		},
	}

	// Should return early on marshal error without panicking
	atmosConfig.processManifestSchemas("atmos")

	// Schema should remain as channel (unchanged)
	_, ok := atmosConfig.Schemas["atmos"].(chan int)
	assert.True(t, ok, "Schema should remain unchanged on marshal error")
}

// TestProcessManifestSchemas_UnmarshalError tests processManifestSchemas with invalid JSON structure.
func TestProcessManifestSchemas_UnmarshalError(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"atmos": map[string]any{
				"manifest": 12345, // Invalid type - should be string
				"matches":  "not-an-array",
			},
		},
	}

	// Should return early on unmarshal error without panicking
	atmosConfig.processManifestSchemas("atmos")

	// Schema should be transformed - JSON unmarshal is actually quite lenient
	// It will convert numbers to empty strings and strings to empty arrays
	registry, ok := atmosConfig.Schemas["atmos"].(SchemaRegistry)
	if ok {
		// If conversion succeeded (JSON is lenient), verify the result
		assert.Empty(t, registry.Manifest)
	} else {
		// If conversion failed, the original map should remain
		_, isMap := atmosConfig.Schemas["atmos"].(map[string]any)
		assert.True(t, isMap, "Schema should remain as map on unmarshal error")
	}
}

// TestProcessResourceSchema_NonExistentKey tests processResourceSchema when key doesn't exist.
func TestProcessResourceSchema_NonExistentKey(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"opa": map[string]any{
				"base_path": "/path/to/opa",
			},
		},
	}

	// Call with non-existent key - should return early without error
	atmosConfig.processResourceSchema("nonexistent")

	// Original schema should be unchanged
	assert.Contains(t, atmosConfig.Schemas, "opa")
	assert.NotContains(t, atmosConfig.Schemas, "nonexistent")
}

// TestProcessResourceSchema_MarshalError tests processResourceSchema with unmarshalable data.
func TestProcessResourceSchema_MarshalError(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"cue": make(chan int), // Channels cannot be marshaled to JSON
		},
	}

	// Should return early on marshal error without panicking
	atmosConfig.processResourceSchema("cue")

	// Schema should remain as channel (unchanged)
	_, ok := atmosConfig.Schemas["cue"].(chan int)
	assert.True(t, ok, "Schema should remain unchanged on marshal error")
}

// TestProcessResourceSchema_UnmarshalError tests processResourceSchema with invalid JSON structure.
func TestProcessResourceSchema_UnmarshalError(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"opa": map[string]any{
				"base_path": []string{"not", "a", "string"}, // Invalid type - should be string
			},
		},
	}

	// Should return early on unmarshal error without panicking
	atmosConfig.processResourceSchema("opa")

	// Schema should be transformed - JSON unmarshal is lenient and converts arrays to empty strings
	resource, ok := atmosConfig.Schemas["opa"].(ResourcePath)
	if ok {
		// If conversion succeeded (JSON is lenient), verify the result
		assert.Empty(t, resource.BasePath)
	} else {
		// If conversion failed, the original map should remain
		_, isMap := atmosConfig.Schemas["opa"].(map[string]any)
		assert.True(t, isMap, "Schema should remain as map on unmarshal error")
	}
}

// TestProcessResourceSchema_ValidData tests processResourceSchema with valid data.
func TestProcessResourceSchema_ValidData(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"jsonschema": map[string]any{
				"base_path": "/schemas/jsonschema",
			},
		},
	}

	atmosConfig.processResourceSchema("jsonschema")

	resource, ok := atmosConfig.Schemas["jsonschema"].(ResourcePath)
	assert.True(t, ok)
	assert.Equal(t, "/schemas/jsonschema", resource.BasePath)
}

// TestProcessManifestSchemas_ValidData tests processManifestSchemas with valid data.
func TestProcessManifestSchemas_ValidData(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"custom": map[string]any{
				"manifest": "/custom/manifest.yaml",
				"schema":   "/custom/schema.json",
				"matches":  []any{"custom/*.yaml", "custom/*.json"},
			},
		},
	}

	atmosConfig.processManifestSchemas("custom")

	registry, ok := atmosConfig.Schemas["custom"].(SchemaRegistry)
	assert.True(t, ok)
	assert.Equal(t, "/custom/manifest.yaml", registry.Manifest)
	assert.Equal(t, "/custom/schema.json", registry.Schema)
	assert.Equal(t, []string{"custom/*.yaml", "custom/*.json"}, registry.Matches)
}

// TestProcessSchemas_AllResourceTypes tests all three resource schema types.
func TestProcessSchemas_AllResourceTypes(t *testing.T) {
	atmosConfig := &AtmosConfiguration{
		Schemas: map[string]any{
			"cue": map[string]any{
				"base_path": "/cue/path",
			},
			"opa": map[string]any{
				"base_path": "/opa/path",
			},
			"jsonschema": map[string]any{
				"base_path": "/jsonschema/path",
			},
		},
	}

	atmosConfig.ProcessSchemas()

	// Verify all three resource schemas
	cue := atmosConfig.Schemas["cue"].(ResourcePath)
	assert.Equal(t, "/cue/path", cue.BasePath)

	opa := atmosConfig.Schemas["opa"].(ResourcePath)
	assert.Equal(t, "/opa/path", opa.BasePath)

	jsonschema := atmosConfig.Schemas["jsonschema"].(ResourcePath)
	assert.Equal(t, "/jsonschema/path", jsonschema.BasePath)
}
