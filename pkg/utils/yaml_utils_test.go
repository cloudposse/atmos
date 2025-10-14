package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestUnmarshalYAMLFromFileWithPositions_BasicParsing tests basic YAML unmarshaling behavior.
func TestUnmarshalYAMLFromFileWithPositions_BasicParsing(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := `
foo: bar
baz:
  qux: 123
list:
  - item1
  - item2
`

	type TestStruct struct {
		Foo  string         `yaml:"foo"`
		Baz  map[string]any `yaml:"baz"`
		List []string       `yaml:"list"`
	}

	result, _, err := UnmarshalYAMLFromFileWithPositions[TestStruct](atmosConfig, input, "test.yaml")

	require.NoError(t, err)
	assert.Equal(t, "bar", result.Foo)
	assert.Equal(t, 123, result.Baz["qux"])
	assert.Equal(t, []string{"item1", "item2"}, result.List)
}

// TestUnmarshalYAMLFromFileWithPositions_WithDifferentContent tests that different content produces different results.
// This indirectly validates the cache key includes content hash (P8.1 optimization).
func TestUnmarshalYAMLFromFileWithPositions_WithDifferentContent(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// First call with content "value1"
	input1 := `key: value1`
	result1, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input1, "test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "value1", result1["key"])

	// Second call with SAME file path but DIFFERENT content "value2"
	// This tests the P8.1 fix: cache key should include content hash
	input2 := `key: value2`
	result2, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input2, "test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "value2", result2["key"], "Different content should produce different results, not cached result")
}

// TestUnmarshalYAMLFromFileWithPositions_WithSameContent tests that same content is handled correctly.
// This validates cache hits work properly when content is identical.
func TestUnmarshalYAMLFromFileWithPositions_WithSameContent(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := `
foo: bar
number: 42
`

	// First call
	result1, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "bar", result1["foo"])
	assert.Equal(t, 42, result1["number"])

	// Second call with same content - should get same result (may use cache internally)
	result2, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "bar", result2["foo"])
	assert.Equal(t, 42, result2["number"])
}

// TestUnmarshalYAMLFromFileWithPositions_EmptyInput tests handling of empty input.
func TestUnmarshalYAMLFromFileWithPositions_EmptyInput(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := ``

	result, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test.yaml")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestUnmarshalYAMLFromFileWithPositions_InvalidYAML tests error handling for invalid YAML.
func TestUnmarshalYAMLFromFileWithPositions_InvalidYAML(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Invalid YAML with unclosed quote
	input := `foo: "bar`

	_, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test.yaml")
	assert.Error(t, err, "Invalid YAML should return an error")
}

// TestUnmarshalYAMLFromFileWithPositions_NilConfig tests nil config handling.
func TestUnmarshalYAMLFromFileWithPositions_NilConfig(t *testing.T) {
	input := `foo: bar`

	_, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](nil, input, "test.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "atmosConfig cannot be nil")
}

// TestGenerateParsedYAMLCacheKey_Basic tests cache key generation behavior.
func TestGenerateParsedYAMLCacheKey_Basic(t *testing.T) {
	// Test with valid inputs
	key1 := generateParsedYAMLCacheKey("file1.yaml", "content1")
	assert.NotEmpty(t, key1)
	assert.Contains(t, key1, "file1.yaml")
	assert.Contains(t, key1, ":")

	// Same file, different content should produce different keys
	key2 := generateParsedYAMLCacheKey("file1.yaml", "content2")
	assert.NotEmpty(t, key2)
	assert.NotEqual(t, key1, key2, "Different content should produce different cache keys")

	// Different file, same content should produce different keys
	key3 := generateParsedYAMLCacheKey("file2.yaml", "content1")
	assert.NotEmpty(t, key3)
	assert.NotEqual(t, key1, key3, "Different files should produce different cache keys")

	// Same file and content should produce same key
	key4 := generateParsedYAMLCacheKey("file1.yaml", "content1")
	assert.Equal(t, key1, key4, "Same file and content should produce same cache key")
}

// TestGenerateParsedYAMLCacheKey_EmptyInputs tests cache key generation with empty inputs.
func TestGenerateParsedYAMLCacheKey_EmptyInputs(t *testing.T) {
	// Empty file path
	key1 := generateParsedYAMLCacheKey("", "content")
	assert.Empty(t, key1)

	// Empty content
	key2 := generateParsedYAMLCacheKey("file.yaml", "")
	assert.Empty(t, key2)

	// Both empty
	key3 := generateParsedYAMLCacheKey("", "")
	assert.Empty(t, key3)
}

// TestHasCustomTags_WithCustomTags tests detection of custom Atmos YAML tags.
func TestHasCustomTags_WithCustomTags(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// YAML with custom !env tag
	input := `
foo: bar
secret: !env SECRET_VAR
`

	// Parse the YAML to get a node
	result, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test.yaml")
	require.NoError(t, err)

	// The result should contain the processed tag
	// Note: We're testing the output, not internal hasCustomTags function
	assert.Contains(t, result, "foo")
	assert.Contains(t, result, "secret")
}

// TestHasCustomTags_WithoutCustomTags tests handling of YAML without custom tags.
func TestHasCustomTags_WithoutCustomTags(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Regular YAML without custom tags
	input := `
foo: bar
baz: qux
nested:
  key: value
`

	result, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test.yaml")
	require.NoError(t, err)
	assert.Equal(t, "bar", result["foo"])
	assert.Equal(t, "qux", result["baz"])
}

// TestUnmarshalYAML_BasicFunctionality tests the basic UnmarshalYAML function.
func TestUnmarshalYAML_BasicFunctionality(t *testing.T) {
	input := `
name: test
value: 123
`

	type TestStruct struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	result, err := UnmarshalYAML[TestStruct](input)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 123, result.Value)
}

// TestUnmarshalYAMLFromFile_BasicFunctionality tests the UnmarshalYAMLFromFile function.
func TestUnmarshalYAMLFromFile_BasicFunctionality(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := `
enabled: true
items:
  - one
  - two
  - three
`

	type TestStruct struct {
		Enabled bool     `yaml:"enabled"`
		Items   []string `yaml:"items"`
	}

	result, err := UnmarshalYAMLFromFile[TestStruct](atmosConfig, input, "test.yaml")
	require.NoError(t, err)
	assert.True(t, result.Enabled)
	assert.Equal(t, []string{"one", "two", "three"}, result.Items)
}

// TestUnmarshalYAMLFromFile_NilConfig tests nil config handling.
func TestUnmarshalYAMLFromFile_NilConfig(t *testing.T) {
	input := `foo: bar`

	_, err := UnmarshalYAMLFromFile[map[string]any](nil, input, "test.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "atmosConfig cannot be nil")
}
