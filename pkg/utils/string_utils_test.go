package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSplitStringByDelimiter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		delimiter rune
		expected  []string
		expectErr bool
	}{
		{
			name:      "Simple split by space",
			input:     "foo bar baz",
			delimiter: ' ',
			expected:  []string{"foo", "bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Split with quoted sections",
			input:     `"foo bar" baz`,
			delimiter: ' ',
			expected:  []string{"foo bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Empty input string",
			input:     "",
			delimiter: ' ',
			expected:  []string{},
			expectErr: true,
		},
		{
			name:      "Delimiter not present",
			input:     "foobar",
			delimiter: ',',
			expected:  []string{"foobar"},
			expectErr: false,
		},
		{
			name:      "Multiple spaces as delimiter",
			input:     "foo: !env      FOO",
			delimiter: ' ',
			expected:  []string{"foo:", "!env", "FOO"},
			expectErr: false,
		},
		{
			name:      "Single quoted value with nested double quotes",
			input:     "core '.security.users[\"github-dependabot\"].access.key.id'",
			delimiter: ' ',
			expected:  []string{"core", ".security.users[\"github-dependabot\"].access.key.id"},
			expectErr: false,
		},
		{
			name:      "Single quoted value with escaped single quotes",
			input:     "core '.security.users[''github-dependabot''].access.key.id'",
			delimiter: ' ',
			expected:  []string{"core", ".security.users['github-dependabot'].access.key.id"},
			expectErr: false,
		},
		{
			name: "Double quoted value with escaped double quotes",
			// If the parser sees "" (two consecutive double quotes inside a quoted string), according to CSV/Excel-like
			// conventions, a "" inside quotes means a literal " character in the final value.
			input:     "\"foo\"\"bar\" baz",
			delimiter: ' ',
			expected:  []string{"foo\"bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Quoted empty values are removed",
			input:     "foo '' \"\" bar",
			delimiter: ' ',
			expected:  []string{"foo", "bar"},
			expectErr: false,
		},
		{
			name:      "Unmatched leading quote is preserved",
			input:     "foo 'bar",
			delimiter: ' ',
			expected:  []string{"foo", "'bar"},
			expectErr: false,
		},

		{
			name:      "Error case with invalid CSV format",
			input:     `"foo,bar`,
			delimiter: ',',
			expected:  nil,
			expectErr: true,
		},
		{
			name:      "Bare quote triggers LazyQuotes retry",
			input:     `foo b"ar baz`,
			delimiter: ' ',
			expected:  []string{"foo", "b\"ar", "baz"},
			expectErr: false,
		},
		{
			name:      "Multiple bare quotes with LazyQuotes fallback",
			input:     `a"b c"d`,
			delimiter: ' ',
			expected:  []string{"a\"b", "c\"d"},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SplitStringByDelimiter(tt.input, tt.delimiter)
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got: %v", tt.expectErr, err)
			}
			if !tt.expectErr && !equalSlices(t, result, tt.expected) {
				t.Errorf("expected: %v, got: %v", tt.expected, result)
			}
		})
	}
}

func equalSlices(t *testing.T, a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			t.Logf("mismatch at index %d: expected %s, got %s", i, b[i], a[i])
			return false
		}
	}
	return true
}

// TestUniqueStrings tests the UniqueStrings function.
func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "No duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "With duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "Empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "All duplicates",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
		{
			name:     "Nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "Single element",
			input:    []string{"single"},
			expected: []string{"single"},
		},
		{
			name:     "Order preservation - first occurrence kept",
			input:    []string{"third", "first", "second", "first", "third"},
			expected: []string{"third", "first", "second"},
		},
		{
			name:     "Empty strings are preserved",
			input:    []string{"", "a", "", "b"},
			expected: []string{"", "a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UniqueStrings(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTrimMatchingQuotes tests the trimMatchingQuotes function.
func TestTrimMatchingQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Double quotes removed",
			input:    `"value"`,
			expected: "value",
		},
		{
			name:     "Single quotes removed",
			input:    "'value'",
			expected: "value",
		},
		{
			name:     "Escaped double quotes normalized",
			input:    `"val""ue"`,
			expected: `val"ue`,
		},
		{
			name:     "Escaped single quotes normalized",
			input:    "'val''ue'",
			expected: "val'ue",
		},
		{
			name:     "Mismatched quotes preserved",
			input:    `"value'`,
			expected: `"value'`,
		},
		{
			name:     "No quotes preserved",
			input:    "value",
			expected: "value",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Single character",
			input:    "a",
			expected: "a",
		},
		{
			name:     "Only quotes",
			input:    `""`,
			expected: "",
		},
		{
			name:     "Leading quote only",
			input:    `"value`,
			expected: `"value`,
		},
		{
			name:     "Trailing quote only",
			input:    `value"`,
			expected: `value"`,
		},
		{
			name:     "Multiple escaped quotes",
			input:    `"a""b""c"`,
			expected: `a"b"c`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimMatchingQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSplitStringAtFirstOccurrence tests the SplitStringAtFirstOccurrence function.
func TestSplitStringAtFirstOccurrence(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		separator string
		expected  [2]string
	}{
		{
			name:      "Split with separator present",
			input:     "key=value",
			separator: "=",
			expected:  [2]string{"key", "value"},
		},
		{
			name:      "Split with multiple separators",
			input:     "key=value=extra",
			separator: "=",
			expected:  [2]string{"key", "value=extra"},
		},
		{
			name:      "No separator present",
			input:     "keyvalue",
			separator: "=",
			expected:  [2]string{"keyvalue", ""},
		},
		{
			name:      "Empty string",
			input:     "",
			separator: "=",
			expected:  [2]string{"", ""},
		},
		{
			name:      "Separator at start",
			input:     "=value",
			separator: "=",
			expected:  [2]string{"", "value"},
		},
		{
			name:      "Separator at end",
			input:     "key=",
			separator: "=",
			expected:  [2]string{"key", ""},
		},
		{
			name:      "Multi-character separator",
			input:     "key::value::extra",
			separator: "::",
			expected:  [2]string{"key", "value::extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitStringAtFirstOccurrence(tt.input, tt.separator)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIntern_BasicFunctionality tests basic string interning behavior.
func TestIntern_BasicFunctionality(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	// First intern of a string.
	s1 := Intern(atmosConfig, "test-string")
	assert.Equal(t, "test-string", s1)

	// Second intern of the same string should return the same instance.
	s2 := Intern(atmosConfig, "test-string")
	assert.Equal(t, "test-string", s2)

	// Verify they compare equal by value. Implementation may deduplicate storage,
	// but this test asserts logical equality only.
	assert.Equal(t, s1, s2)

	// Different string should be different.
	s3 := Intern(atmosConfig, "different-string")
	assert.Equal(t, "different-string", s3)
	assert.NotEqual(t, s1, s3)
}

// TestIntern_EmptyString tests that empty strings are not interned.
func TestIntern_EmptyString(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	empty := Intern(atmosConfig, "")
	assert.Equal(t, "", empty)

	// Empty strings should not affect statistics.
	stats := GetInternStats()
	assert.Equal(t, int64(0), stats.Requests, "Empty strings should not count as requests")
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, int64(0), stats.SavedBytes)
}

// TestIntern_Statistics tests that interning statistics are tracked correctly.
func TestIntern_Statistics(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	// First intern - should be a miss.
	Intern(atmosConfig, "vars")
	stats1 := GetInternStats()
	assert.Equal(t, int64(1), stats1.Requests, "Should have 1 request")
	assert.Equal(t, int64(0), stats1.Hits, "Should have 0 hits")
	assert.Equal(t, int64(1), stats1.Misses, "Should have 1 miss")

	// Second intern of same string - should be a hit.
	Intern(atmosConfig, "vars")
	stats2 := GetInternStats()
	assert.Equal(t, int64(2), stats2.Requests, "Should have 2 requests")
	assert.Equal(t, int64(1), stats2.Hits, "Should have 1 hit")
	assert.Equal(t, int64(1), stats2.Misses, "Should still have 1 miss")
	assert.Equal(t, int64(4), stats2.SavedBytes, "Should have saved 4 bytes (length of 'vars')")

	// Third intern of same string - another hit.
	Intern(atmosConfig, "vars")
	stats3 := GetInternStats()
	assert.Equal(t, int64(3), stats3.Requests, "Should have 3 requests")
	assert.Equal(t, int64(2), stats3.Hits, "Should have 2 hits")
	assert.Equal(t, int64(1), stats3.Misses, "Should still have 1 miss")
	assert.Equal(t, int64(8), stats3.SavedBytes, "Should have saved 8 bytes (2 hits × 4 bytes)")

	// Intern a different string - should be another miss.
	Intern(atmosConfig, "settings")
	stats4 := GetInternStats()
	assert.Equal(t, int64(4), stats4.Requests, "Should have 4 requests")
	assert.Equal(t, int64(2), stats4.Hits, "Should still have 2 hits")
	assert.Equal(t, int64(2), stats4.Misses, "Should have 2 misses")
}

// TestIntern_ConcurrentAccess tests that string interning is thread-safe.
func TestIntern_ConcurrentAccess(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	// Common strings that might appear in Atmos configs.
	commonStrings := []string{
		"vars", "settings", "metadata", "env", "backend",
		"terraform", "helmfile", "us-east-1", "us-west-2",
		"production", "staging", "development",
	}

	const numGoroutines = 100
	done := make(chan struct{}, numGoroutines)
	start := make(chan struct{})

	// Spawn many goroutines that all intern the same strings.
	for i := 0; i < numGoroutines; i++ {
		go func() {
			<-start
			for _, s := range commonStrings {
				interned := Intern(atmosConfig, s)
				assert.NotEmpty(t, interned)
			}
			done <- struct{}{}
		}()
	}

	// Release all goroutines at once.
	close(start)

	// Wait for all to complete.
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify statistics make sense.
	stats := GetInternStats()
	expectedRequests := int64(numGoroutines * len(commonStrings))
	assert.Equal(t, expectedRequests, stats.Requests, "Should have correct number of requests")

	// We should have at least len(commonStrings) misses (first occurrence of each).
	assert.GreaterOrEqual(t, stats.Misses, int64(len(commonStrings)), "Should have at least one miss per unique string")

	// Hits + misses should equal total requests.
	assert.Equal(t, stats.Requests, stats.Hits+stats.Misses, "Hits + misses should equal requests")

	// We should have saved some memory from deduplication.
	assert.Greater(t, stats.SavedBytes, int64(0), "Should have saved some memory")
}

// TestInternSlice_BasicFunctionality tests string slice interning.
func TestInternSlice_BasicFunctionality(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	input := []string{"vars", "settings", "vars", "metadata", "vars"}
	result := InternSlice(atmosConfig, input)

	// Result should have same values.
	assert.Equal(t, input, result)

	// After interning, the strings should be deduplicated in the pool.
	stats := GetInternStats()
	assert.Equal(t, int64(5), stats.Requests, "Should have 5 intern requests")
}

// TestInternSlice_EmptySlice tests that empty slices are handled correctly.
func TestInternSlice_EmptySlice(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	result := InternSlice(atmosConfig, []string{})
	assert.Empty(t, result)

	stats := GetInternStats()
	assert.Equal(t, int64(0), stats.Requests, "Empty slice should not generate requests")
}

// TestInternMapKeys_BasicFunctionality tests map key interning.
func TestInternMapKeys_BasicFunctionality(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	input := map[string]any{
		"vars":     123,
		"settings": "value",
		"metadata": true,
	}

	result := InternMapKeys(atmosConfig, input)

	// Result should have same keys and values.
	assert.Equal(t, input["vars"], result["vars"])
	assert.Equal(t, input["settings"], result["settings"])
	assert.Equal(t, input["metadata"], result["metadata"])

	// Should have interned 3 keys.
	stats := GetInternStats()
	assert.Equal(t, int64(3), stats.Requests, "Should have 3 intern requests for keys")
}

// TestInternStringsInMap_NestedStructure tests recursive string interning in nested maps.
func TestInternStringsInMap_NestedStructure(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	input := map[string]any{
		"vars": map[string]any{
			"region":      "us-east-1",
			"environment": "production",
			"nested": map[string]any{
				"key": "value",
			},
		},
		"settings": map[string]any{
			"enabled": true,
		},
	}

	result := InternStringsInMap(atmosConfig, input)

	// Verify structure is preserved.
	resultMap := result.(map[string]any)
	assert.Contains(t, resultMap, "vars")
	assert.Contains(t, resultMap, "settings")

	varsMap := resultMap["vars"].(map[string]any)
	assert.Equal(t, "us-east-1", varsMap["region"])
	assert.Equal(t, "production", varsMap["environment"])

	nestedMap := varsMap["nested"].(map[string]any)
	assert.Equal(t, "value", nestedMap["key"])

	// Should have interned all string keys and string values.
	stats := GetInternStats()
	// Keys: vars, settings, region, environment, nested, key, enabled
	// String values: us-east-1, production, value
	// Total: 7 keys + 3 string values = 10.
	assert.Equal(t, int64(10), stats.Requests, "Should have interned all string keys and values")
}

// TestInternStringsInMap_WithArrays tests that arrays are handled correctly.
func TestInternStringsInMap_WithArrays(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	input := map[string]any{
		"components": []any{
			"vpc",
			"eks",
			"vpc", // Duplicate.
		},
		"regions": []any{
			"us-east-1",
			"us-west-2",
		},
	}

	result := InternStringsInMap(atmosConfig, input)

	resultMap := result.(map[string]any)
	components := resultMap["components"].([]any)
	assert.Equal(t, "vpc", components[0])
	assert.Equal(t, "eks", components[1])
	assert.Equal(t, "vpc", components[2])

	regions := resultMap["regions"].([]any)
	assert.Equal(t, "us-east-1", regions[0])
	assert.Equal(t, "us-west-2", regions[1])

	// Keys: components, regions (2).
	// Array string values: vpc, eks, vpc (3), us-east-1, us-west-2 (2).
	// Total: 2 + 5 = 7.
	stats := GetInternStats()
	assert.Equal(t, int64(7), stats.Requests, "Should have interned keys and array string values")
}

// TestInternStringsInMap_PreservesNonStringTypes tests that non-string types are not modified.
func TestInternStringsInMap_PreservesNonStringTypes(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	input := map[string]any{
		"number":  42,
		"boolean": true,
		"float":   3.14,
		"null":    nil,
		"string":  "test",
	}

	result := InternStringsInMap(atmosConfig, input)

	resultMap := result.(map[string]any)
	assert.Equal(t, 42, resultMap["number"])
	assert.Equal(t, true, resultMap["boolean"])
	assert.Equal(t, 3.14, resultMap["float"])
	assert.Nil(t, resultMap["null"])
	assert.Equal(t, "test", resultMap["string"])

	// Keys: number, boolean, float, null, string (5).
	// String values: test (1).
	// Total: 5 + 1 = 6.
	stats := GetInternStats()
	assert.Equal(t, int64(6), stats.Requests, "Should only intern string keys and string values")
}

// TestInternStringsInMap_CommonAtmosKeys tests interning with typical Atmos configuration keys.
func TestInternStringsInMap_CommonAtmosKeys(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	// Simulate a typical Atmos component configuration.
	input := map[string]any{
		"vars": map[string]any{
			"namespace":   "cp",
			"environment": "dev",
			"stage":       "dev",
			"name":        "vpc",
		},
		"settings": map[string]any{
			"spacelift": map[string]any{
				"workspace_enabled": true,
			},
		},
		"env": map[string]any{
			"AWS_REGION": "us-east-1",
		},
		"backend": map[string]any{
			"bucket": "tfstate-bucket",
			"key":    "vpc.tfstate",
			"region": "us-east-1", // Duplicate of AWS_REGION value.
		},
	}

	result := InternStringsInMap(atmosConfig, input)

	resultMap := result.(map[string]any)
	varsMap := resultMap["vars"].(map[string]any)
	assert.Equal(t, "cp", varsMap["namespace"])
	assert.Equal(t, "dev", varsMap["environment"])

	backendMap := resultMap["backend"].(map[string]any)
	assert.Equal(t, "us-east-1", backendMap["region"])

	// With duplicate "us-east-1", we should see cache hits.
	stats := GetInternStats()
	assert.Greater(t, stats.Requests, int64(0))
	assert.Greater(t, stats.Hits, int64(0), "Should have cache hits for duplicate strings")
	assert.Greater(t, stats.SavedBytes, int64(0), "Should have saved memory from deduplication")
}

// TestClearInternPool tests that clearing the pool works correctly.
func TestClearInternPool(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Intern some strings.
	Intern(atmosConfig, "test1")
	Intern(atmosConfig, "test2")
	Intern(atmosConfig, "test1") // Duplicate.

	stats1 := GetInternStats()
	assert.Equal(t, int64(3), stats1.Requests)
	assert.Equal(t, int64(1), stats1.Hits)

	// Clear the pool.
	ClearInternPool()

	// Stats should be reset.
	stats2 := GetInternStats()
	assert.Equal(t, int64(0), stats2.Requests)
	assert.Equal(t, int64(0), stats2.Hits)
	assert.Equal(t, int64(0), stats2.Misses)
	assert.Equal(t, int64(0), stats2.SavedBytes)

	// Interning again should start fresh.
	Intern(atmosConfig, "test1")
	stats3 := GetInternStats()
	assert.Equal(t, int64(1), stats3.Requests)
	assert.Equal(t, int64(1), stats3.Misses, "Should be a miss after clearing pool")
}

// TestResetInternStats tests that resetting statistics works without clearing the pool.
func TestResetInternStats(t *testing.T) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	// Intern some strings.
	Intern(atmosConfig, "test1")
	Intern(atmosConfig, "test2")

	stats1 := GetInternStats()
	assert.Equal(t, int64(2), stats1.Requests)

	// Reset stats only.
	ResetInternStats()

	// Stats should be reset.
	stats2 := GetInternStats()
	assert.Equal(t, int64(0), stats2.Requests)
	assert.Equal(t, int64(0), stats2.Hits)
	assert.Equal(t, int64(0), stats2.Misses)
	assert.Equal(t, int64(0), stats2.SavedBytes)

	// But the pool should still have the strings (this would be a hit).
	Intern(atmosConfig, "test1")
	stats3 := GetInternStats()
	assert.Equal(t, int64(1), stats3.Hits, "Should be a hit - pool was not cleared")
}

// BenchmarkIntern_WithDuplicates benchmarks string interning with many duplicates (typical Atmos scenario).
func BenchmarkIntern_WithDuplicates(b *testing.B) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	// Common strings in Atmos configs (high duplication rate).
	commonStrings := []string{
		"vars", "settings", "metadata", "env", "backend",
		"terraform", "helmfile", "component", "stack",
		"us-east-1", "us-west-2", "production", "staging",
		"true", "false", "enabled", "disabled",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate typical usage: intern the same strings many times.
		for _, s := range commonStrings {
			Intern(atmosConfig, s)
		}
	}
	b.StopTimer()

	// Report statistics.
	stats := GetInternStats()
	hitRate := float64(stats.Hits) / float64(stats.Requests) * 100.0
	b.ReportMetric(hitRate, "hit_rate_%")
	b.ReportMetric(float64(stats.SavedBytes), "saved_bytes")
}

// BenchmarkIntern_WithoutDuplicates benchmarks string interning with unique strings (worst case).
func BenchmarkIntern_WithoutDuplicates(b *testing.B) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Generate unique strings - worst case for interning.
		uniqueString := "string-" + string(rune(i%10000))
		Intern(atmosConfig, uniqueString)
	}
	b.StopTimer()

	stats := GetInternStats()
	hitRate := float64(stats.Hits) / float64(stats.Requests) * 100.0
	b.ReportMetric(hitRate, "hit_rate_%")
	b.ReportMetric(float64(stats.Misses), "cache_misses")
}

// BenchmarkInternStringsInMap_TypicalAtmosConfig benchmarks interning a typical Atmos config structure.
func BenchmarkInternStringsInMap_TypicalAtmosConfig(b *testing.B) {
	ClearInternPool()
	defer ClearInternPool()

	atmosConfig := &schema.AtmosConfiguration{}

	// Simulate a typical Atmos component configuration.
	typicalConfig := map[string]any{
		"vars": map[string]any{
			"namespace":   "cp",
			"environment": "prod",
			"stage":       "prod",
			"region":      "us-east-1",
		},
		"settings": map[string]any{
			"spacelift": map[string]any{
				"workspace_enabled": true,
			},
		},
		"env": map[string]any{
			"AWS_REGION": "us-east-1",
		},
		"backend": map[string]any{
			"bucket": "tfstate-bucket",
			"region": "us-east-1",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		InternStringsInMap(atmosConfig, typicalConfig)
	}
	b.StopTimer()

	stats := GetInternStats()
	hitRate := float64(stats.Hits) / float64(stats.Requests) * 100.0
	b.ReportMetric(hitRate, "hit_rate_%")
	b.ReportMetric(float64(stats.SavedBytes), "saved_bytes")
}
