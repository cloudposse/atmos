package yaml

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/stack/loader"
)

func TestNew(t *testing.T) {
	l := New()

	assert.NotNil(t, l)
	assert.Equal(t, "YAML", l.Name())
	assert.Equal(t, []string{".yaml", ".yml"}, l.Extensions())
}

func TestLoaderLoad(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    any
		expectError bool
	}{
		{
			name:  "simple map",
			input: "key: value",
			expected: map[string]any{
				"key": "value",
			},
		},
		{
			name:  "nested map",
			input: "outer:\n  inner: value",
			expected: map[string]any{
				"outer": map[string]any{
					"inner": "value",
				},
			},
		},
		{
			name:  "array",
			input: "items:\n  - one\n  - two\n  - three",
			expected: map[string]any{
				"items": []any{"one", "two", "three"},
			},
		},
		{
			name:  "mixed types",
			input: "string: hello\nnumber: 42\nbool: true\nfloat: 3.14",
			expected: map[string]any{
				"string": "hello",
				"number": 42,
				"bool":   true,
				"float":  3.14,
			},
		},
		{
			name:        "invalid yaml",
			input:       "key: [invalid",
			expectError: true,
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
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
	// Use exact input to control line numbers precisely.
	input := "vars:\n  region: us-east-1\n  env: prod\nsettings:\n  enabled: true\n"
	l := New()

	result, metadata, err := l.LoadWithMetadata(context.Background(), []byte(input),
		loader.WithSourceFile("test.yaml"))

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, metadata)
	assert.Equal(t, "test.yaml", metadata.SourceFile)

	// Check that positions were extracted.
	assert.NotEmpty(t, metadata.Positions)

	// Verify specific positions exist (note: yaml.v3 counts from document start).
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
			name: "simple map",
			input: map[string]any{
				"key": "value",
			},
			contains: []string{"key: value"},
		},
		{
			name: "nested map",
			input: map[string]any{
				"outer": map[string]any{
					"inner": "value",
				},
			},
			contains: []string{"outer:", "inner: value"},
		},
		{
			name: "array",
			input: map[string]any{
				"items": []string{"one", "two"},
			},
			contains: []string{"items:", "- one", "- two"},
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
	assert.Contains(t, string(result), "key: value")
}

func TestLoaderCaching(t *testing.T) {
	l := New()
	input := []byte("key: value")

	// First load should parse.
	result1, err := l.Load(context.Background(), input, loader.WithSourceFile("test.yaml"))
	require.NoError(t, err)

	// Second load should hit cache.
	result2, err := l.Load(context.Background(), input, loader.WithSourceFile("test.yaml"))
	require.NoError(t, err)

	assert.Equal(t, result1, result2)

	// Check cache stats.
	entries, _ := l.CacheStats()
	assert.Equal(t, 1, entries)
}

func TestLoaderCacheDifferentContent(t *testing.T) {
	l := New()

	// Load first content.
	_, err := l.Load(context.Background(), []byte("key: value1"), loader.WithSourceFile("test.yaml"))
	require.NoError(t, err)

	// Load second content - should be separate cache entry.
	_, err = l.Load(context.Background(), []byte("key: value2"), loader.WithSourceFile("test.yaml"))
	require.NoError(t, err)

	// Should have 2 cache entries.
	entries, _ := l.CacheStats()
	assert.Equal(t, 2, entries)
}

func TestLoaderClearCache(t *testing.T) {
	l := New()

	// Load some content.
	_, err := l.Load(context.Background(), []byte("key: value"), loader.WithSourceFile("test.yaml"))
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

	_, err := l.Load(ctx, []byte("key: value"))
	assert.Error(t, err)
}

func TestLoaderMultiDocument(t *testing.T) {
	// YAML with multiple documents - only first is loaded.
	input := `doc1: value1
---
doc2: value2`

	l := New()
	result, err := l.Load(context.Background(), []byte(input))

	require.NoError(t, err)

	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value1", m["doc1"])
	assert.NotContains(t, m, "doc2")
}

func TestLoaderComplexStructure(t *testing.T) {
	input := `
components:
  terraform:
    vpc:
      vars:
        region: us-east-1
        availability_zones:
          - us-east-1a
          - us-east-1b
      settings:
        spacelift:
          workspace_enabled: true
`
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
	input := []byte("key: value")
	done := make(chan bool)

	// Concurrent loads.
	for i := 0; i < 100; i++ {
		go func() {
			_, _ = l.Load(context.Background(), input, loader.WithSourceFile("test.yaml"))
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

func TestLoaderEncodeInvalidData(t *testing.T) {
	l := New()

	// Create data with a channel which can't be marshaled.
	// The yaml library panics on unsupported types, but our Encode method recovers.
	ch := make(chan int)
	input := map[string]any{
		"channel": ch,
	}

	_, err := l.Encode(context.Background(), input)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrEncodeFailed))
}

func TestLoaderPositionExtraction(t *testing.T) {
	input := `
root:
  child1:
    grandchild: value1
  child2:
    - item1
    - item2
`
	l := New()
	_, meta, err := l.LoadWithMetadata(context.Background(), []byte(strings.TrimSpace(input)))

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
