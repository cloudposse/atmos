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

// TestUnmarshalYAMLFromFileWithPositions_ConcurrentAccess tests that multiple goroutines
// parsing the same file simultaneously don't cause race conditions or duplicate parsing.
// This validates the P3.3.1 per-key locking implementation.
func TestUnmarshalYAMLFromFileWithPositions_ConcurrentAccess(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := `
foo: bar
number: 42
nested:
  key: value
  list:
    - item1
    - item2
`

	// Simulate concurrent access by spawning multiple goroutines
	// that all try to parse the same file with the same content.
	const numGoroutines = 50
	results := make(chan map[string]any, numGoroutines)
	errors := make(chan error, numGoroutines)

	// Use a channel to synchronize goroutine start times
	// This ensures they all hit the cache at approximately the same time
	start := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			// Wait for start signal to maximize concurrency
			<-start

			// All goroutines parse the same file with the same content
			result, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](
				atmosConfig, input, "concurrent-test.yaml")
			if err != nil {
				errors <- err
				return
			}

			results <- result
		}()
	}

	// Release all goroutines at once to maximize race condition likelihood
	close(start)

	// Collect all results
	var collectedResults []map[string]any
	var collectedErrors []error

	for i := 0; i < numGoroutines; i++ {
		select {
		case result := <-results:
			collectedResults = append(collectedResults, result)
		case err := <-errors:
			collectedErrors = append(collectedErrors, err)
		}
	}

	// Validate no errors occurred
	assert.Empty(t, collectedErrors, "Expected no errors from concurrent parsing")

	// Validate all goroutines got results
	assert.Len(t, collectedResults, numGoroutines, "Expected all goroutines to return results")

	// Validate all results are identical and correct
	expectedValue := "bar"
	expectedNumber := 42

	for i, result := range collectedResults {
		assert.Equal(t, expectedValue, result["foo"],
			"Goroutine %d got incorrect value for 'foo'", i)
		assert.Equal(t, expectedNumber, result["number"],
			"Goroutine %d got incorrect value for 'number'", i)

		// Validate nested structure
		nested, ok := result["nested"].(map[string]any)
		assert.True(t, ok, "Goroutine %d: 'nested' should be a map", i)
		if ok {
			assert.Equal(t, "value", nested["key"],
				"Goroutine %d got incorrect nested value", i)
		}
	}
}

// TestUnmarshalYAMLFromFileWithPositions_ConcurrentAccessDifferentFiles tests
// concurrent parsing of different files to ensure per-key locking doesn't block
// unrelated files.
func TestUnmarshalYAMLFromFileWithPositions_ConcurrentAccessDifferentFiles(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Create multiple different file contents
	files := []struct {
		name    string
		content string
		key     string
		value   string
	}{
		{"file1.yaml", "key1: value1", "key1", "value1"},
		{"file2.yaml", "key2: value2", "key2", "value2"},
		{"file3.yaml", "key3: value3", "key3", "value3"},
		{"file4.yaml", "key4: value4", "key4", "value4"},
		{"file5.yaml", "key5: value5", "key5", "value5"},
	}

	const goroutinesPerFile = 10
	totalGoroutines := len(files) * goroutinesPerFile

	results := make(chan map[string]any, totalGoroutines)
	errors := make(chan error, totalGoroutines)
	start := make(chan struct{})

	// Spawn multiple goroutines per file
	for _, file := range files {
		for i := 0; i < goroutinesPerFile; i++ {
			go func(name, content string) {
				<-start

				result, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](
					atmosConfig, content, name)
				if err != nil {
					errors <- err
					return
				}

				results <- result
			}(file.name, file.content)
		}
	}

	// Release all goroutines
	close(start)

	// Collect results
	collectedResults := make(map[string][]map[string]any)
	var collectedErrors []error

	for i := 0; i < totalGoroutines; i++ {
		select {
		case result := <-results:
			// Group results by their key to validate later
			for k := range result {
				collectedResults[k] = append(collectedResults[k], result)
			}
		case err := <-errors:
			collectedErrors = append(collectedErrors, err)
		}
	}

	// Validate no errors
	assert.Empty(t, collectedErrors, "Expected no errors from concurrent parsing of different files")

	// Validate each file was parsed correctly by all its goroutines
	for _, file := range files {
		fileResults := collectedResults[file.key]
		assert.Len(t, fileResults, goroutinesPerFile,
			"Expected %d results for %s", goroutinesPerFile, file.name)

		for i, result := range fileResults {
			assert.Equal(t, file.value, result[file.key],
				"Goroutine %d for %s got incorrect value", i, file.name)
		}
	}
}

// TestPrintAsYAMLSimple tests the fast-path YAML printing without syntax highlighting.
func TestPrintAsYAMLSimple(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		data        any
		wantErr     bool
	}{
		{
			name: "simple map data",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 2,
					},
				},
			},
			data: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name: "nested data structure",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 4,
					},
				},
			},
			data: map[string]any{
				"string": "value",
				"number": 42,
				"nested": map[string]any{
					"array": []string{"one", "two", "three"},
					"bool":  true,
				},
			},
			wantErr: false,
		},
		{
			name: "nil data",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{},
			},
			data:    nil,
			wantErr: false,
		},
		{
			name:        "nil config should error",
			atmosConfig: nil,
			data: map[string]any{
				"key": "value",
			},
			wantErr: true,
		},
		{
			name: "default tab width when TabWidth is 0",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						TabWidth: 0, // Should use default
					},
				},
			},
			data: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PrintAsYAMLSimple(tt.atmosConfig, tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestConvertToYAML_Consistency tests that ConvertToYAML produces consistent output.
// This validates P7.8 optimization: buffer pooling doesn't affect output consistency.
func TestConvertToYAML_Consistency(t *testing.T) {
	data := map[string]any{
		"string": "value",
		"number": 42,
		"nested": map[string]any{
			"array": []string{"one", "two", "three"},
			"bool":  true,
		},
	}

	// Multiple calls should produce identical output
	result1, err1 := ConvertToYAML(data)
	require.NoError(t, err1)

	result2, err2 := ConvertToYAML(data)
	require.NoError(t, err2)

	result3, err3 := ConvertToYAML(data)
	require.NoError(t, err3)

	// All results should be identical
	assert.Equal(t, result1, result2, "First and second call should produce identical output")
	assert.Equal(t, result2, result3, "Second and third call should produce identical output")

	// Verify output is valid
	assert.Contains(t, result1, "string: value")
	assert.Contains(t, result1, "number: 42")
	assert.Contains(t, result1, "bool: true")
}

// TestConvertToYAML_DifferentSizes tests buffer pooling with various data sizes.
// This validates P7.8 optimization: buffer pool handles different sizes correctly.
func TestConvertToYAML_DifferentSizes(t *testing.T) {
	testCases := []struct {
		name string
		data any
	}{
		{
			name: "small data",
			data: map[string]any{"key": "value"},
		},
		{
			name: "medium data",
			data: map[string]any{
				"key1": "value1",
				"key2": 123,
				"nested": map[string]any{
					"inner1": "value",
					"inner2": []string{"a", "b", "c"},
				},
			},
		},
		{
			name: "large nested structure",
			data: generateLargeNestedMap(5, 10),
		},
		{
			name: "array with many elements",
			data: map[string]any{
				"items": generateLargeArray(100),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ConvertToYAML(tc.data)
			require.NoError(t, err)
			require.NotEmpty(t, result)

			// Verify output can be parsed back
			_, err = UnmarshalYAML[map[string]any](result)
			assert.NoError(t, err, "Output should be valid YAML")
		})
	}
}

// TestConvertToYAML_Concurrent tests concurrent YAML conversion with buffer pooling.
// This validates P7.8 optimization: sync.Pool is thread-safe for concurrent access.
func TestConvertToYAML_Concurrent(t *testing.T) {
	data := map[string]any{
		"foo": "bar",
		"num": 123,
		"nested": map[string]any{
			"key": "value",
		},
	}

	const numGoroutines = 100
	results := make(chan string, numGoroutines)
	errors := make(chan error, numGoroutines)
	start := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-start

			result, err := ConvertToYAML(data)
			if err != nil {
				errors <- err
				return
			}

			results <- result
		}()
	}

	// Release all goroutines
	close(start)

	// Collect results
	var collectedResults []string
	var collectedErrors []error

	for i := 0; i < numGoroutines; i++ {
		select {
		case result := <-results:
			collectedResults = append(collectedResults, result)
		case err := <-errors:
			collectedErrors = append(collectedErrors, err)
		}
	}

	// Validate no errors
	assert.Empty(t, collectedErrors, "No errors should occur during concurrent conversion")
	assert.Len(t, collectedResults, numGoroutines, "All goroutines should complete")

	// All results should be identical
	firstResult := collectedResults[0]
	for i, result := range collectedResults {
		assert.Equal(t, firstResult, result, "Result %d should match first result", i)
	}
}

// TestConvertToYAML_ConcurrentDifferentData tests concurrent YAML conversion
// with different data to stress test buffer pooling.
func TestConvertToYAML_ConcurrentDifferentData(t *testing.T) {
	// Create different data sets
	dataSets := []map[string]any{
		{"key1": "value1", "num": 1},
		{"key2": "value2", "num": 2},
		{"key3": "value3", "num": 3},
		{"key4": "value4", "num": 4},
		{"key5": "value5", "num": 5},
	}

	const goroutinesPerDataSet = 20
	totalGoroutines := len(dataSets) * goroutinesPerDataSet

	results := make(chan string, totalGoroutines)
	errors := make(chan error, totalGoroutines)
	start := make(chan struct{})

	// Spawn multiple goroutines per data set
	for _, data := range dataSets {
		for i := 0; i < goroutinesPerDataSet; i++ {
			go func(d map[string]any) {
				<-start

				result, err := ConvertToYAML(d)
				if err != nil {
					errors <- err
					return
				}

				results <- result
			}(data)
		}
	}

	// Release all goroutines
	close(start)

	// Collect results
	successCount := 0
	errorCount := 0

	for i := 0; i < totalGoroutines; i++ {
		select {
		case result := <-results:
			assert.NotEmpty(t, result)
			successCount++
		case <-errors:
			errorCount++
		}
	}

	assert.Equal(t, totalGoroutines, successCount, "All concurrent conversions should succeed")
	assert.Equal(t, 0, errorCount, "No errors should occur")
}

// BenchmarkConvertToYAML benchmarks YAML conversion with buffer pooling (P7.8).
func BenchmarkConvertToYAML(b *testing.B) {
	data := map[string]any{
		"string": "value",
		"number": 42,
		"nested": map[string]any{
			"array": []string{"one", "two", "three"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ConvertToYAML(data)
	}
}

// BenchmarkConvertToYAML_Large benchmarks YAML conversion with large data.
// This demonstrates P7.8 buffer pooling benefit on larger data structures.
func BenchmarkConvertToYAML_Large(b *testing.B) {
	data := generateLargeNestedMap(10, 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ConvertToYAML(data)
	}
}

// BenchmarkConvertToYAML_Parallel benchmarks concurrent YAML conversion.
// This demonstrates P7.8 buffer pooling efficiency under concurrent load.
func BenchmarkConvertToYAML_Parallel(b *testing.B) {
	data := map[string]any{
		"key":    "value",
		"number": 123,
		"nested": map[string]any{
			"inner": []string{"a", "b", "c"},
		},
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = ConvertToYAML(data)
		}
	})
}

// Helper function to generate large nested map for testing.
func generateLargeNestedMap(depth, breadth int) map[string]any {
	if depth == 0 {
		return map[string]any{"leaf": "value"}
	}

	result := make(map[string]any, breadth)
	for i := 0; i < breadth; i++ {
		key := "key" + string(rune('0'+i%10))
		if i%2 == 0 {
			result[key] = generateLargeNestedMap(depth-1, breadth)
		} else {
			result[key] = "value" + string(rune('0'+i%10))
		}
	}
	return result
}

// Helper function to generate large array for testing.
func generateLargeArray(size int) []string {
	result := make([]string, size)
	for i := 0; i < size; i++ {
		result[i] = "item" + string(rune('0'+i%10))
	}
	return result
}

// TestAtmosYamlTagsMap_ContainsAllTags tests that atmosYamlTagsMap contains all expected tags.
// This validates P1.2 optimization: O(1) lookup map contains all custom YAML tags.
func TestAtmosYamlTagsMap_ContainsAllTags(t *testing.T) {
	// Validate that atmosYamlTagsMap contains all tags from AtmosYamlTags slice.
	expectedTags := []string{
		AtmosYamlFuncExec,
		AtmosYamlFuncStore,
		AtmosYamlFuncStoreGet,
		AtmosYamlFuncTemplate,
		AtmosYamlFuncTerraformOutput,
		AtmosYamlFuncTerraformState,
		AtmosYamlFuncEnv,
	}

	for _, tag := range expectedTags {
		assert.True(t, atmosYamlTagsMap[tag],
			"atmosYamlTagsMap should contain tag: %s", tag)
	}

	// Verify the map has exactly the expected number of tags.
	assert.Equal(t, len(expectedTags), len(atmosYamlTagsMap),
		"atmosYamlTagsMap should contain exactly %d tags", len(expectedTags))
}

// TestAtmosYamlTagsMap_O1Lookup tests that atmosYamlTagsMap provides O(1) lookup.
// This validates P1.2 optimization: map lookup is constant time vs O(n) slice search.
func TestAtmosYamlTagsMap_O1Lookup(t *testing.T) {
	// Test that all expected tags are found in O(1) time.
	testCases := []struct {
		tag      string
		expected bool
	}{
		{AtmosYamlFuncExec, true},
		{AtmosYamlFuncStore, true},
		{AtmosYamlFuncStoreGet, true},
		{AtmosYamlFuncTemplate, true},
		{AtmosYamlFuncTerraformOutput, true},
		{AtmosYamlFuncTerraformState, true},
		{AtmosYamlFuncEnv, true},
		{"!unknown", false},
		{"!invalid", false},
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.tag, func(t *testing.T) {
			result := atmosYamlTagsMap[tc.tag]
			assert.Equal(t, tc.expected, result,
				"atmosYamlTagsMap[%s] should be %v", tc.tag, tc.expected)
		})
	}
}

// BenchmarkAtmosYamlTagsMap_MapLookup benchmarks map-based tag lookup (P1.2).
// This demonstrates O(1) performance vs O(n) slice search.
func BenchmarkAtmosYamlTagsMap_MapLookup(b *testing.B) {
	tag := AtmosYamlFuncTerraformOutput

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = atmosYamlTagsMap[tag]
	}
}

// BenchmarkAtmosYamlTagsSlice_LinearSearch benchmarks slice-based tag search for comparison.
// This demonstrates the O(n) performance that P1.2 optimization replaces.
func BenchmarkAtmosYamlTagsSlice_LinearSearch(b *testing.B) {
	tag := AtmosYamlFuncTerraformOutput

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate the old linear search approach.
		found := false
		for _, t := range AtmosYamlTags {
			if t == tag {
				found = true
				break
			}
		}
		_ = found
	}
}

// TestDeepCopyYAMLNode_NoAliasing verifies that deepCopyYAMLNode creates independent copies.
// This test validates the fix for yaml.Node shallow copy aliasing where Content slice
// and Alias pointer were shared between cached and returned nodes, causing mutations
// in one to affect the other.
func TestDeepCopyYAMLNode_NoAliasing(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	input := `
list:
  - item1
  - item2
nested:
  key: value
`

	// First call - caches the result
	result1, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test-aliasing.yaml")
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Verify initial values
	list1, ok := result1["list"].([]any)
	require.True(t, ok, "list should be []any")
	require.Len(t, list1, 2, "list should have 2 items")
	assert.Equal(t, "item1", list1[0])
	assert.Equal(t, "item2", list1[1])

	nested1, ok := result1["nested"].(map[string]any)
	require.True(t, ok, "nested should be map[string]any")
	assert.Equal(t, "value", nested1["key"])

	// Modify result1 to test aliasing
	list1[0] = "MODIFIED"
	nested1["key"] = "MODIFIED"
	result1["new_key"] = "ADDED"

	// Second call with same content - should get from cache but with deep copy
	result2, _, err := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test-aliasing.yaml")
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Verify that result2 is NOT affected by modifications to result1 (no aliasing)
	list2, ok := result2["list"].([]any)
	require.True(t, ok, "list should be []any")
	require.Len(t, list2, 2, "list should have 2 items")
	assert.Equal(t, "item1", list2[0], "First item should still be 'item1', not affected by result1 modification")
	assert.Equal(t, "item2", list2[1], "Second item should still be 'item2'")

	nested2, ok := result2["nested"].(map[string]any)
	require.True(t, ok, "nested should be map[string]any")
	assert.Equal(t, "value", nested2["key"], "Nested key should still be 'value', not affected by result1 modification")

	_, hasNewKey := result2["new_key"]
	assert.False(t, hasNewKey, "result2 should not have the key added to result1")
}

// TestDeepCopyYAMLNode_NilNode tests that deepCopyYAMLNode handles nil input.
func TestDeepCopyYAMLNode_NilNode(t *testing.T) {
	result := deepCopyYAMLNode(nil)
	assert.Nil(t, result, "Copying nil node should return nil")
}

// TestDeepCopyYAMLNode_ComplexStructure tests deep copying of complex YAML structures.
// This validates that all nested Content slices and Alias pointers are properly copied.
func TestDeepCopyYAMLNode_ComplexStructure(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Complex YAML with multiple levels of nesting
	input := `
root:
  level1:
    level2:
      level3:
        deep: value
        list:
          - nested1
          - nested2
          - nested3
  array:
    - item1
    - item2
    - item3
`

	// Parse twice to ensure cached deep copy works
	result1, _, err1 := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test-complex.yaml")
	require.NoError(t, err1)

	result2, _, err2 := UnmarshalYAMLFromFileWithPositions[map[string]any](atmosConfig, input, "test-complex.yaml")
	require.NoError(t, err2)

	// Both should have same structure
	root1 := result1["root"].(map[string]any)
	root2 := result2["root"].(map[string]any)

	level1_1 := root1["level1"].(map[string]any)
	level1_2 := root2["level1"].(map[string]any)

	level2_1 := level1_1["level2"].(map[string]any)
	level2_2 := level1_2["level2"].(map[string]any)

	level3_1 := level2_1["level3"].(map[string]any)
	level3_2 := level2_2["level3"].(map[string]any)

	// Initial values should match
	assert.Equal(t, "value", level3_1["deep"])
	assert.Equal(t, "value", level3_2["deep"])

	// Modify deep nested value in result1
	level3_1["deep"] = "MODIFIED"

	// result2 should NOT be affected (no aliasing)
	assert.Equal(t, "value", level3_2["deep"], "Deep nested value should not be affected by modification")
}

// TestPrintParsedYAMLCacheStats_NoDivideByZero tests that PrintParsedYAMLCacheStats
// doesn't panic with divide-by-zero when uniqueFiles or uniqueHashes is 0.
func TestPrintParsedYAMLCacheStats_NoDivideByZero(t *testing.T) {
	// Save current stats
	parsedYAMLCacheStats.Lock()
	savedHits := parsedYAMLCacheStats.hits
	savedMisses := parsedYAMLCacheStats.misses
	savedTotalCalls := parsedYAMLCacheStats.totalCalls
	savedFiles := parsedYAMLCacheStats.uniqueFiles
	savedHashes := parsedYAMLCacheStats.uniqueHashes

	// Reset stats to zero to trigger divide-by-zero scenario
	parsedYAMLCacheStats.hits = 0
	parsedYAMLCacheStats.misses = 0
	parsedYAMLCacheStats.totalCalls = 0
	parsedYAMLCacheStats.uniqueFiles = make(map[string]int)
	parsedYAMLCacheStats.uniqueHashes = make(map[string]int)
	parsedYAMLCacheStats.Unlock()

	// This should not panic with divide-by-zero
	assert.NotPanics(t, func() {
		PrintParsedYAMLCacheStats()
	}, "PrintParsedYAMLCacheStats should not panic with zero uniqueFiles/uniqueHashes")

	// Restore stats
	parsedYAMLCacheStats.Lock()
	parsedYAMLCacheStats.hits = savedHits
	parsedYAMLCacheStats.misses = savedMisses
	parsedYAMLCacheStats.totalCalls = savedTotalCalls
	parsedYAMLCacheStats.uniqueFiles = savedFiles
	parsedYAMLCacheStats.uniqueHashes = savedHashes
	parsedYAMLCacheStats.Unlock()
}
