package manifest

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestLoad_AcceptsPointerTypedSpec covers the pointer-unwrap branch in Load:
// a caller instantiating Load[*testSpec] (instead of Load[testSpec]) must
// still match the registered non-pointer spec type instead of being
// incorrectly rejected as a mismatch.
func TestLoad_AcceptsPointerTypedSpec(t *testing.T) {
	registerTestKind(t)

	data := []byte("apiVersion: atmos/v1\nkind: AtmosTestConfig\nmetadata:\n  name: x\nspec:\n  source: embedded\n")

	m, err := Load[*testSpec](testKind, data)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.NotNil(t, m.Spec)
	assert.Equal(t, "embedded", m.Spec.Source)
}

// TestYamlToJSONValue_InvalidYAML covers the YAML parse error branch of the
// unexported helper used by Validate to normalize a document before schema
// validation.
func TestYamlToJSONValue_InvalidYAML(t *testing.T) {
	_, err := yamlToJSONValue([]byte("key: [unclosed"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestParse)
}

// TestYamlToJSONValue_NonStringMapKeyFailsJSONMarshal covers the JSON
// marshal error branch: YAML permits non-string mapping keys, but the JSON
// Schema validator's normalized form requires JSON-marshalable values, so a
// document with e.g. an integer key must surface as a manifest parse error
// rather than panicking or silently dropping data.
func TestYamlToJSONValue_NonStringMapKeyFailsJSONMarshal(t *testing.T) {
	_, err := yamlToJSONValue([]byte("? [1, 2]\n: value\n"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestParse)
}

// TestBuildValidationError_NonSchemaError covers the fallback branch of
// buildValidationError for errors that are not a *jsonschema.ValidationError
// (e.g. a plain error surfacing from elsewhere in the validation pipeline).
func TestBuildValidationError_NonSchemaError(t *testing.T) {
	err := buildValidationError(testKind, errors.New("boom"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestValidation)
	assert.Contains(t, err.Error(), "boom")
}

// TestCompileSchema_InvalidJSON covers the AddResource error branch: a
// malformed schema document must surface as ErrManifestSchemaGenerate rather
// than a raw jsonschema library error.
func TestCompileSchema_InvalidJSON(t *testing.T) {
	_, err := compileSchema("Probe", "not valid json")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestSchemaGenerate)
}

// TestCompileSchema_UnresolvableRef covers the Compile error branch: schema
// JSON that is syntactically valid but semantically broken (a $ref that
// cannot be resolved) must also surface as ErrManifestSchemaGenerate.
func TestCompileSchema_UnresolvableRef(t *testing.T) {
	badSchema := `{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type": "object",
		"properties": {
			"foo": {"$ref": "#/does/not/exist"}
		}
	}`

	_, err := compileSchema("Probe", badSchema)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrManifestSchemaGenerate)
}

// TestKindsHint_EmptyRegistry covers the zero-kinds branch of kindsHint,
// exercised when a manifest kind lookup fails before anything has been
// registered (e.g. at process startup before init() runs, or in a test that
// resets the registry).
func TestKindsHint_EmptyRegistry(t *testing.T) {
	resetRegistry()
	t.Cleanup(resetRegistry)

	assert.Equal(t, "no manifest kinds are registered", kindsHint())
}
