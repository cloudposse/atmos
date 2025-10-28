package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvenanceEntry(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		line     int
		column   int
		typ      ProvenanceType
		value    any
		depth    int
		expected *ProvenanceEntry
	}{
		{
			name:   "basic entry with string value",
			file:   "config.yaml",
			line:   10,
			column: 5,
			typ:    ProvenanceTypeInline,
			value:  "test-value",
			depth:  0,
			expected: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: hashValue("test-value"),
				Depth:     0,
			},
		},
		{
			name:   "entry with zero column",
			file:   "base.yaml",
			line:   1,
			column: 0,
			typ:    ProvenanceTypeImport,
			value:  123,
			depth:  1,
			expected: &ProvenanceEntry{
				File:      "base.yaml",
				Line:      1,
				Column:    0,
				Type:      ProvenanceTypeImport,
				ValueHash: hashValue(123),
				Depth:     1,
			},
		},
		{
			name:   "entry with nil value",
			file:   "override.yaml",
			line:   25,
			column: 10,
			typ:    ProvenanceTypeOverride,
			value:  nil,
			depth:  2,
			expected: &ProvenanceEntry{
				File:      "override.yaml",
				Line:      25,
				Column:    10,
				Type:      ProvenanceTypeOverride,
				ValueHash: "",
				Depth:     2,
			},
		},
		{
			name:   "computed entry",
			file:   "template.yaml",
			line:   50,
			column: 15,
			typ:    ProvenanceTypeComputed,
			value:  map[string]any{"key": "value"},
			depth:  0,
			expected: &ProvenanceEntry{
				File:      "template.yaml",
				Line:      50,
				Column:    15,
				Type:      ProvenanceTypeComputed,
				ValueHash: hashValue(map[string]any{"key": "value"}),
				Depth:     0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewProvenanceEntry(ProvenanceEntryParams{
				File:   tt.file,
				Line:   tt.line,
				Column: tt.column,
				Type:   tt.typ,
				Value:  tt.value,
				Depth:  tt.depth,
			})

			assert.Equal(t, tt.expected.File, result.File)
			assert.Equal(t, tt.expected.Line, result.Line)
			assert.Equal(t, tt.expected.Column, result.Column)
			assert.Equal(t, tt.expected.Type, result.Type)
			assert.Equal(t, tt.expected.ValueHash, result.ValueHash)
			assert.Equal(t, tt.expected.Depth, result.Depth)
		})
	}
}

func TestProvenanceEntry_String(t *testing.T) {
	tests := []struct {
		name     string
		entry    *ProvenanceEntry
		expected string
	}{
		{
			name: "with column and depth 0",
			entry: &ProvenanceEntry{
				File:   "config.yaml",
				Line:   10,
				Column: 5,
				Type:   ProvenanceTypeInline,
				Depth:  0,
			},
			expected: "config.yaml:10:5 [0] (inline)",
		},
		{
			name: "without column and depth 1",
			entry: &ProvenanceEntry{
				File:   "base.yaml",
				Line:   1,
				Column: 0,
				Type:   ProvenanceTypeImport,
				Depth:  1,
			},
			expected: "base.yaml:1 [1] (import)",
		},
		{
			name: "override type with depth 2",
			entry: &ProvenanceEntry{
				File:   "override.yaml",
				Line:   25,
				Column: 10,
				Type:   ProvenanceTypeOverride,
				Depth:  2,
			},
			expected: "override.yaml:25:10 [2] (override)",
		},
		{
			name:     "nil entry",
			entry:    nil,
			expected: "<nil>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProvenanceEntry_Equals(t *testing.T) {
	tests := []struct {
		name     string
		entry1   *ProvenanceEntry
		entry2   *ProvenanceEntry
		expected bool
	}{
		{
			name: "identical entries",
			entry1: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
				Depth:     0,
			},
			entry2: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
				Depth:     0,
			},
			expected: true,
		},
		{
			name: "different files",
			entry1: &ProvenanceEntry{
				File:      "config1.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			entry2: &ProvenanceEntry{
				File:      "config2.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			expected: false,
		},
		{
			name: "different lines",
			entry1: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			entry2: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      11,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			expected: false,
		},
		{
			name: "different columns",
			entry1: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			entry2: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    6,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			expected: false,
		},
		{
			name: "different types",
			entry1: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			entry2: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeOverride,
				ValueHash: "abcd1234",
			},
			expected: false,
		},
		{
			name: "different value hashes",
			entry1: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
				Depth:     0,
			},
			entry2: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "efgh5678",
				Depth:     0,
			},
			expected: false,
		},
		{
			name: "different depths",
			entry1: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
				Depth:     0,
			},
			entry2: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
				Depth:     1,
			},
			expected: false,
		},
		{
			name:     "both nil",
			entry1:   nil,
			entry2:   nil,
			expected: true,
		},
		{
			name:   "first nil",
			entry1: nil,
			entry2: &ProvenanceEntry{
				File: "config.yaml",
				Line: 10,
			},
			expected: false,
		},
		{
			name: "second nil",
			entry1: &ProvenanceEntry{
				File: "config.yaml",
				Line: 10,
			},
			entry2:   nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry1.Equals(tt.entry2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProvenanceEntry_Clone(t *testing.T) {
	tests := []struct {
		name  string
		entry *ProvenanceEntry
	}{
		{
			name: "clone non-nil entry",
			entry: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
				Depth:     0,
			},
		},
		{
			name:  "clone nil entry",
			entry: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloned := tt.entry.Clone()

			if tt.entry == nil {
				assert.Nil(t, cloned)
				return
			}

			require.NotNil(t, cloned)

			// Verify values are equal.
			assert.True(t, tt.entry.Equals(cloned))

			// Verify it's a deep copy (different pointer).
			assert.NotSame(t, tt.entry, cloned)

			// Modify original and verify clone is unaffected.
			tt.entry.File = "modified.yaml"
			assert.NotEqual(t, tt.entry.File, cloned.File)
		})
	}
}

func TestProvenanceEntry_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		entry    *ProvenanceEntry
		expected bool
	}{
		{
			name: "valid entry",
			entry: &ProvenanceEntry{
				File: "config.yaml",
				Line: 10,
			},
			expected: true,
		},
		{
			name: "valid entry with all fields",
			entry: &ProvenanceEntry{
				File:      "config.yaml",
				Line:      10,
				Column:    5,
				Type:      ProvenanceTypeInline,
				ValueHash: "abcd1234",
			},
			expected: true,
		},
		{
			name: "empty file",
			entry: &ProvenanceEntry{
				File: "",
				Line: 10,
			},
			expected: false,
		},
		{
			name: "zero line",
			entry: &ProvenanceEntry{
				File: "config.yaml",
				Line: 0,
			},
			expected: false,
		},
		{
			name: "negative line",
			entry: &ProvenanceEntry{
				File: "config.yaml",
				Line: -1,
			},
			expected: false,
		},
		{
			name:     "nil entry",
			entry:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHashValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{
			name:  "string value",
			value: "test-string",
		},
		{
			name:  "integer value",
			value: 42,
		},
		{
			name:  "boolean value",
			value: true,
		},
		{
			name:  "map value",
			value: map[string]any{"key": "value"},
		},
		{
			name:  "slice value",
			value: []string{"a", "b", "c"},
		},
		{
			name:  "nil value",
			value: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := hashValue(tt.value)
			hash2 := hashValue(tt.value)

			if tt.value == nil {
				assert.Empty(t, hash1)
				assert.Empty(t, hash2)
			} else {
				// Hash should be consistent.
				assert.Equal(t, hash1, hash2)

				// Hash should be non-empty.
				assert.NotEmpty(t, hash1)

				// Hash should be hex string of fixed length (16 characters for 8 bytes).
				assert.Len(t, hash1, 16)
			}
		})
	}
}

func TestHashValue_DifferentValues(t *testing.T) {
	// Different values should produce different hashes.
	hash1 := hashValue("value1")
	hash2 := hashValue("value2")

	assert.NotEqual(t, hash1, hash2)
}

func TestProvenanceType_Constants(t *testing.T) {
	// Verify all constants are defined and have unique values.
	types := []ProvenanceType{
		ProvenanceTypeImport,
		ProvenanceTypeInline,
		ProvenanceTypeOverride,
		ProvenanceTypeComputed,
		ProvenanceTypeDefault,
	}

	// Check uniqueness.
	seen := make(map[ProvenanceType]bool)
	for _, typ := range types {
		assert.False(t, seen[typ], "duplicate provenance type: %s", typ)
		seen[typ] = true
	}

	// Check expected string values.
	assert.Equal(t, ProvenanceType("import"), ProvenanceTypeImport)
	assert.Equal(t, ProvenanceType("inline"), ProvenanceTypeInline)
	assert.Equal(t, ProvenanceType("override"), ProvenanceTypeOverride)
	assert.Equal(t, ProvenanceType("computed"), ProvenanceTypeComputed)
	assert.Equal(t, ProvenanceType("default"), ProvenanceTypeDefault)
}
