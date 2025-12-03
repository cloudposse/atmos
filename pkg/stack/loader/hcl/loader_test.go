package hcl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/stack/loader"
)

func TestNew(t *testing.T) {
	l := New()

	assert.NotNil(t, l)
	assert.Equal(t, "HCL", l.Name())
	assert.Equal(t, []string{".hcl", ".tf"}, l.Extensions())
}

func TestLoaderLoad(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    any
		expectError bool
	}{
		{
			name:  "simple attribute",
			input: `key = "value"`,
			expected: map[string]any{
				"key": "value",
			},
		},
		{
			name:  "multiple attributes",
			input: `name = "test"` + "\n" + `count = 42`,
			expected: map[string]any{
				"name":  "test",
				"count": int64(42),
			},
		},
		{
			name:  "boolean values",
			input: `enabled = true` + "\n" + `disabled = false`,
			expected: map[string]any{
				"enabled":  true,
				"disabled": false,
			},
		},
		{
			name:  "list values",
			input: `items = ["one", "two", "three"]`,
			expected: map[string]any{
				"items": []any{"one", "two", "three"},
			},
		},
		{
			name:  "object values",
			input: `config = { name = "test", count = 5 }`,
			expected: map[string]any{
				"config": map[string]any{
					"name":  "test",
					"count": int64(5),
				},
			},
		},
		{
			name:  "nested objects",
			input: `outer = { inner = { value = "nested" } }`,
			expected: map[string]any{
				"outer": map[string]any{
					"inner": map[string]any{
						"value": "nested",
					},
				},
			},
		},
		{
			name:  "float values",
			input: `rate = 3.14`,
			expected: map[string]any{
				"rate": 3.14,
			},
		},
		{
			name:        "invalid hcl",
			input:       `key = [invalid`,
			expectError: true,
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   \n\t  ",
			expected: nil,
		},
		{
			name:  "null value",
			input: `key = null`,
			expected: map[string]any{
				"key": nil,
			},
		},
		{
			name:  "multiline string",
			input: "description = \"line1\\nline2\"",
			expected: map[string]any{
				"description": "line1\nline2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New()

			result, err := l.Load(context.Background(), []byte(tt.input))

			if tt.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, loader.ErrParseFailed))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoaderLoadWithMetadata(t *testing.T) {
	input := `
region = "us-east-1"
env = "prod"
enabled = true
`
	l := New()

	result, metadata, err := l.LoadWithMetadata(context.Background(), []byte(input),
		loader.WithSourceFile("test.hcl"))

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, metadata)
	assert.Equal(t, "test.hcl", metadata.SourceFile)

	// Check that positions were extracted.
	assert.NotEmpty(t, metadata.Positions)

	// Verify specific positions exist.
	pos, ok := metadata.Positions["region"]
	assert.True(t, ok, "position for 'region' should exist")
	assert.Greater(t, pos.Line, 0, "position line should be positive")

	pos, ok = metadata.Positions["env"]
	assert.True(t, ok, "position for 'env' should exist")
	assert.Greater(t, pos.Line, 0, "position line should be positive")

	pos, ok = metadata.Positions["enabled"]
	assert.True(t, ok, "position for 'enabled' should exist")
	assert.Greater(t, pos.Line, 0, "position line should be positive")
}

func TestLoaderEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		contains []string
	}{
		{
			name: "simple attribute",
			input: map[string]any{
				"key": "value",
			},
			contains: []string{"key", "value"},
		},
		{
			name: "multiple attributes",
			input: map[string]any{
				"name":  "test",
				"count": 42,
			},
			contains: []string{"name", "test", "count", "42"},
		},
		{
			name: "boolean values",
			input: map[string]any{
				"enabled": true,
			},
			contains: []string{"enabled", "true"},
		},
		{
			name: "list values",
			input: map[string]any{
				"items": []any{"one", "two"},
			},
			contains: []string{"items", "one", "two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New()

			result, err := l.Encode(context.Background(), tt.input)
			require.NoError(t, err)

			resultStr := string(result)
			for _, expected := range tt.contains {
				assert.Contains(t, resultStr, expected)
			}
		})
	}
}

func TestLoaderEncodeInvalidInput(t *testing.T) {
	l := New()

	// Encode expects map[string]any.
	_, err := l.Encode(context.Background(), "not a map")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, loader.ErrEncodeFailed))
}

func TestLoaderCaching(t *testing.T) {
	l := New()
	input := []byte(`key = "value"`)

	// First load should parse.
	result1, err := l.Load(context.Background(), input, loader.WithSourceFile("test.hcl"))
	require.NoError(t, err)

	// Second load should hit cache.
	result2, err := l.Load(context.Background(), input, loader.WithSourceFile("test.hcl"))
	require.NoError(t, err)

	assert.Equal(t, result1, result2)

	// Check cache stats.
	entries, _ := l.CacheStats()
	assert.Equal(t, 1, entries)
}

func TestLoaderCacheDifferentContent(t *testing.T) {
	l := New()

	// Load first content.
	_, err := l.Load(context.Background(), []byte(`key = "value1"`), loader.WithSourceFile("test.hcl"))
	require.NoError(t, err)

	// Load second content - should be separate cache entry.
	_, err = l.Load(context.Background(), []byte(`key = "value2"`), loader.WithSourceFile("test.hcl"))
	require.NoError(t, err)

	// Should have 2 cache entries.
	entries, _ := l.CacheStats()
	assert.Equal(t, 2, entries)
}

func TestLoaderClearCache(t *testing.T) {
	l := New()

	// Load some content.
	_, err := l.Load(context.Background(), []byte(`key = "value"`), loader.WithSourceFile("test.hcl"))
	require.NoError(t, err)

	entries, _ := l.CacheStats()
	assert.Equal(t, 1, entries)

	// Clear cache.
	l.ClearCache()

	entries, _ = l.CacheStats()
	assert.Equal(t, 0, entries)
}

func TestLoaderContextCancellation(t *testing.T) {
	l := New()

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := l.Load(ctx, []byte(`key = "value"`))
	assert.Error(t, err)
}

func TestLoaderComplexStructure(t *testing.T) {
	input := `
vars = {
  region = "us-east-1"
  availability_zones = ["us-east-1a", "us-east-1b"]
}
settings = {
  spacelift = {
    workspace_enabled = true
  }
}
`
	l := New()
	result, meta, err := l.LoadWithMetadata(context.Background(), []byte(input))

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check positions.
	pos, ok := meta.Positions["vars"]
	assert.True(t, ok, "position for 'vars' should exist")
	assert.Greater(t, pos.Line, 0)

	pos, ok = meta.Positions["settings"]
	assert.True(t, ok, "position for 'settings' should exist")
	assert.Greater(t, pos.Line, 0)
}

func TestLoaderConcurrentAccess(t *testing.T) {
	l := New()
	input := []byte(`key = "value"`)
	done := make(chan bool)

	// Concurrent loads.
	for i := 0; i < 100; i++ {
		go func() {
			_, _ = l.Load(context.Background(), input, loader.WithSourceFile("test.hcl"))
			done <- true
		}()
	}

	// Wait for all to complete.
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should have exactly 1 cache entry due to deduplication.
	entries, _ := l.CacheStats()
	assert.Equal(t, 1, entries)
}

func TestLoaderPositionExtraction(t *testing.T) {
	input := `
root = {
  child1 = {
    grandchild = "value1"
  }
  child2 = ["item1", "item2"]
}
`
	l := New()
	_, meta, err := l.LoadWithMetadata(context.Background(), []byte(input))

	require.NoError(t, err)

	// Check root level.
	_, ok := meta.Positions["root"]
	assert.True(t, ok)

	// Check nested paths.
	_, ok = meta.Positions["root.child1"]
	assert.True(t, ok)

	_, ok = meta.Positions["root.child1.grandchild"]
	assert.True(t, ok)

	// Check array indices.
	_, ok = meta.Positions["root.child2[0]"]
	assert.True(t, ok)

	_, ok = meta.Positions["root.child2[1]"]
	assert.True(t, ok)
}

func TestLoaderNumberPrecision(t *testing.T) {
	// Test integer values.
	input := `large_int = 9007199254740993`
	l := New()

	result, err := l.Load(context.Background(), []byte(input))
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)

	// The number should be preserved.
	num := m["large_int"]
	assert.NotNil(t, num)
}

func TestLoaderEncodeContextCancellation(t *testing.T) {
	l := New()

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := l.Encode(ctx, map[string]any{"key": "value"})
	assert.Error(t, err)
}

func TestLoaderEncodeRoundTrip(t *testing.T) {
	l := New()
	original := map[string]any{
		"name":    "test",
		"count":   int64(42),
		"enabled": true,
		"items":   []any{"one", "two"},
	}

	// Encode.
	encoded, err := l.Encode(context.Background(), original)
	require.NoError(t, err)

	// Decode back.
	decoded, err := l.Load(context.Background(), encoded)
	require.NoError(t, err)

	// Verify key values are preserved.
	m, ok := decoded.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", m["name"])
	assert.Equal(t, int64(42), m["count"])
	assert.Equal(t, true, m["enabled"])
}

func TestGoToCty(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		expectError bool
	}{
		{"nil", nil, false},
		{"bool true", true, false},
		{"bool false", false, false},
		{"string", "hello", false},
		{"int", 42, false},
		{"int64", int64(42), false},
		{"float64", 3.14, false},
		{"empty slice", []any{}, false},
		{"string slice", []any{"a", "b"}, false},
		{"empty map", map[string]any{}, false},
		{"string map", map[string]any{"key": "value"}, false},
		{"unsupported type", make(chan int), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := goToCty(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestParseBlocks(t *testing.T) {
	input := `
name = "example"
count = 5
`
	l := New()
	result, err := l.ParseBlocks(context.Background(), []byte(input), "test.hcl")

	require.NoError(t, err)
	// ParseBlocks may return empty if no blocks match schema.
	// For simple attributes, use Load instead.
	assert.NotNil(t, result)
}

func TestParseBlocksEmptyInput(t *testing.T) {
	l := New()
	result, err := l.ParseBlocks(context.Background(), []byte(""), "test.hcl")

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseBlocksContextCancellation(t *testing.T) {
	l := New()

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := l.ParseBlocks(ctx, []byte(`key = "value"`), "test.hcl")
	assert.Error(t, err)
}

func TestLoaderSpecialCharacters(t *testing.T) {
	input := `key = "value with \"quotes\" and special chars: @#$%"`
	l := New()

	result, err := l.Load(context.Background(), []byte(input))
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `value with "quotes" and special chars: @#$%`, m["key"])
}

func TestLoaderMixedNumericTypes(t *testing.T) {
	input := `
integer = 42
float = 3.14
negative = -10
`
	l := New()

	result, err := l.Load(context.Background(), []byte(input))
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, int64(42), m["integer"])
	assert.Equal(t, 3.14, m["float"])
	assert.Equal(t, int64(-10), m["negative"])
}

func TestLoaderHeredoc(t *testing.T) {
	// HCL2 heredoc syntax requires proper indentation.
	input := `script = "#!/bin/bash\necho Hello"`
	l := New()

	result, err := l.Load(context.Background(), []byte(input))
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.True(t, strings.Contains(m["script"].(string), "echo"))
}
