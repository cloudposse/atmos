package json

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/stack/loader"
)

func TestNew(t *testing.T) {
	l := New()

	assert.NotNil(t, l)
	assert.Equal(t, "JSON", l.Name())
	assert.Equal(t, []string{".json"}, l.Extensions())
}

func TestLoaderLoad(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    any
		expectError bool
	}{
		{
			name:  "simple object",
			input: `{"key": "value"}`,
			expected: map[string]any{
				"key": "value",
			},
		},
		{
			name:  "nested object",
			input: `{"outer": {"inner": "value"}}`,
			expected: map[string]any{
				"outer": map[string]any{
					"inner": "value",
				},
			},
		},
		{
			name:  "array",
			input: `{"items": ["one", "two", "three"]}`,
			expected: map[string]any{
				"items": []any{"one", "two", "three"},
			},
		},
		{
			name:  "mixed types",
			input: `{"string": "hello", "number": 42, "bool": true, "float": 3.14}`,
			expected: map[string]any{
				"string": "hello",
				"number": json.Number("42"),
				"bool":   true,
				"float":  json.Number("3.14"),
			},
		},
		{
			name:        "invalid json",
			input:       `{"key": invalid}`,
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
			input: `{"key": null}`,
			expected: map[string]any{
				"key": nil,
			},
		},
		{
			name:     "top level array",
			input:    `["one", "two", "three"]`,
			expected: []any{"one", "two", "three"},
		},
		{
			name:  "complex nested structure",
			input: `{"components": {"terraform": {"vpc": {"vars": {"region": "us-east-1"}}}}}`,
			expected: map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"vars": map[string]any{
								"region": "us-east-1",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New()

			result, err := l.Load(context.Background(), []byte(tt.input))

			if tt.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, errUtils.ErrLoaderParseFailed))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoaderLoadWithMetadata(t *testing.T) {
	input := `{
  "vars": {
    "region": "us-east-1",
    "env": "prod"
  },
  "settings": {
    "enabled": true
  }
}`
	l := New()

	result, metadata, err := l.LoadWithMetadata(context.Background(), []byte(input),
		loader.WithSourceFile("test.json"))

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, metadata)
	assert.Equal(t, "test.json", metadata.SourceFile)

	// Check that positions were extracted.
	assert.NotEmpty(t, metadata.Positions)

	// Verify specific positions exist.
	pos, ok := metadata.Positions["vars"]
	assert.True(t, ok, "position for 'vars' should exist")
	assert.Greater(t, pos.Line, 0, "position line should be positive")

	pos, ok = metadata.Positions["vars.region"]
	assert.True(t, ok, "position for 'vars.region' should exist")
	assert.Greater(t, pos.Line, 0, "position line should be positive")

	pos, ok = metadata.Positions["settings"]
	assert.True(t, ok, "position for 'settings' should exist")
	assert.Greater(t, pos.Line, 0, "position line should be positive")
}

func TestLoaderEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		contains []string
	}{
		{
			name: "simple object",
			input: map[string]any{
				"key": "value",
			},
			contains: []string{`"key"`, `"value"`},
		},
		{
			name: "nested object",
			input: map[string]any{
				"outer": map[string]any{
					"inner": "value",
				},
			},
			contains: []string{`"outer"`, `"inner"`, `"value"`},
		},
		{
			name: "array",
			input: map[string]any{
				"items": []string{"one", "two"},
			},
			contains: []string{`"items"`, `"one"`, `"two"`},
		},
		{
			name: "boolean and number",
			input: map[string]any{
				"enabled": true,
				"count":   42,
			},
			contains: []string{`"enabled"`, "true", `"count"`, "42"},
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

func TestLoaderEncodeWithOptions(t *testing.T) {
	l := New()
	input := map[string]any{
		"key": "value",
	}

	// Test with custom indent.
	result, err := l.Encode(context.Background(), input, loader.WithIndent("    "))
	require.NoError(t, err)
	assert.Contains(t, string(result), "    ")

	// Test with compact output.
	result, err = l.Encode(context.Background(), input, loader.WithCompactOutput(true))
	require.NoError(t, err)
	// Compact output should not have indentation (still has trailing newline from encoder).
	resultStr := string(result)
	assert.NotContains(t, resultStr, "  ") // No 2-space indent.
}

func TestLoaderCaching(t *testing.T) {
	l := New()
	input := []byte(`{"key": "value"}`)

	// First load should parse.
	result1, err := l.Load(context.Background(), input, loader.WithSourceFile("test.json"))
	require.NoError(t, err)

	// Second load should hit cache.
	result2, err := l.Load(context.Background(), input, loader.WithSourceFile("test.json"))
	require.NoError(t, err)

	assert.Equal(t, result1, result2)

	// Check cache stats.
	entries, _ := l.CacheStats()
	assert.Equal(t, 1, entries)
}

func TestLoaderCacheDifferentContent(t *testing.T) {
	l := New()

	// Load first content.
	_, err := l.Load(context.Background(), []byte(`{"key": "value1"}`), loader.WithSourceFile("test.json"))
	require.NoError(t, err)

	// Load second content - should be separate cache entry.
	_, err = l.Load(context.Background(), []byte(`{"key": "value2"}`), loader.WithSourceFile("test.json"))
	require.NoError(t, err)

	// Should have 2 cache entries.
	entries, _ := l.CacheStats()
	assert.Equal(t, 2, entries)
}

func TestLoaderClearCache(t *testing.T) {
	l := New()

	// Load some content.
	_, err := l.Load(context.Background(), []byte(`{"key": "value"}`), loader.WithSourceFile("test.json"))
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

	_, err := l.Load(ctx, []byte(`{"key": "value"}`))
	assert.Error(t, err)
}

func TestLoaderComplexStructure(t *testing.T) {
	input := `{
  "components": {
    "terraform": {
      "vpc": {
        "vars": {
          "region": "us-east-1",
          "availability_zones": ["us-east-1a", "us-east-1b"]
        },
        "settings": {
          "spacelift": {
            "workspace_enabled": true
          }
        }
      }
    }
  }
}`
	l := New()
	result, meta, err := l.LoadWithMetadata(context.Background(), []byte(input))

	require.NoError(t, err)
	assert.NotNil(t, result)

	// Check deep positions.
	pos, ok := meta.Positions["components.terraform.vpc.vars.region"]
	assert.True(t, ok, "deep path position should exist")
	assert.Greater(t, pos.Line, 0)

	// Verify array positions.
	pos, ok = meta.Positions["components.terraform.vpc.vars.availability_zones[0]"]
	assert.True(t, ok, "array element position should exist")
}

func TestLoaderConcurrentAccess(t *testing.T) {
	l := New()
	input := []byte(`{"key": "value"}`)
	done := make(chan bool)

	// Concurrent loads.
	for i := 0; i < 100; i++ {
		go func() {
			_, _ = l.Load(context.Background(), input, loader.WithSourceFile("test.json"))
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
	input := `{
  "root": {
    "child1": {
      "grandchild": "value1"
    },
    "child2": ["item1", "item2"]
  }
}`
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
	// Test that large numbers are preserved.
	input := `{"large_number": 9007199254740993}`
	l := New()

	result, err := l.Load(context.Background(), []byte(input))
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)

	// The number should be preserved as json.Number.
	num := m["large_number"]
	assert.NotNil(t, num)
}

func TestLoaderSpecialCharacters(t *testing.T) {
	input := `{"key": "value with \"quotes\" and\nnewline"}`
	l := New()

	result, err := l.Load(context.Background(), []byte(input))
	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value with \"quotes\" and\nnewline", m["key"])
}

func TestLoaderEncodeContextCancellation(t *testing.T) {
	l := New()

	// Create cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := l.Encode(ctx, map[string]any{"key": "value"})
	assert.Error(t, err)
}

func TestLoaderEncodeNoHTMLEscape(t *testing.T) {
	l := New()
	input := map[string]any{
		"url": "https://example.com?foo=bar&baz=qux",
	}

	result, err := l.Encode(context.Background(), input)
	require.NoError(t, err)

	// Should not escape & as \u0026.
	assert.Contains(t, string(result), "&")
	assert.NotContains(t, string(result), "\\u0026")
}
