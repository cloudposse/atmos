package expand

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "gopkg.in/yaml.v3"
)

// parseAndExpand is a test helper that parses YAML, expands key delimiters,
// and returns the resulting map.
func parseAndExpand(t *testing.T, yamlStr, delimiter string) map[string]any {
	t.Helper()

	var node goyaml.Node
	err := goyaml.Unmarshal([]byte(yamlStr), &node)
	require.NoError(t, err)

	KeyDelimiters(&node, delimiter)

	var result map[string]any
	err = node.Decode(&result)
	require.NoError(t, err)

	return result
}

func TestKeyDelimiters(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		delimiter string
		check     func(t *testing.T, result map[string]any)
	}{
		{
			name:      "no_delimiter_in_keys",
			yaml:      "a: v",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "v", result["a"])
			},
		},
		{
			name:      "single_dot_unquoted",
			yaml:      "a.b: v",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok, "expected nested map under 'a'")
				assert.Equal(t, "v", a["b"])
			},
		},
		{
			name:      "multi_level_dots",
			yaml:      "a.b.c: v",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				b, ok := a["b"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "v", b["c"])
			},
		},
		{
			name:      "quoted_double_preserved",
			yaml:      `"a.b": v`,
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				// Quoted key stays literal.
				assert.Equal(t, "v", result["a.b"])
				assert.Nil(t, result["a"])
			},
		},
		{
			name:      "quoted_single_preserved",
			yaml:      `'a.b': v`,
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "v", result["a.b"])
				assert.Nil(t, result["a"])
			},
		},
		{
			name:      "mixed_quoted_and_unquoted",
			yaml:      "a.b: v1\n\"c.d\": v2",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				// Unquoted expanded.
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "v1", a["b"])
				// Quoted preserved.
				assert.Equal(t, "v2", result["c.d"])
			},
		},
		{
			name:      "same_prefix_multiple_keys",
			yaml:      "a.b: 1\na.c: 2",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, 1, a["b"])
				assert.Equal(t, 2, a["c"])
			},
		},
		{
			name:      "merge_with_existing_nested",
			yaml:      "a:\n  c: old\na.b: new",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "new", a["b"])
				assert.Equal(t, "old", a["c"])
			},
		},
		{
			name:      "dotted_wins_conflict",
			yaml:      "a:\n  b: old\na.b: new",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "new", a["b"])
			},
		},
		{
			name:      "recursive_expansion",
			yaml:      "p:\n  c.k: v",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				p, ok := result["p"].(map[string]any)
				require.True(t, ok)
				c, ok := p["c"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "v", c["k"])
			},
		},
		{
			name:      "list_value",
			yaml:      "a.b:\n  - x\n  - y",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				b, ok := a["b"].([]any)
				require.True(t, ok)
				assert.Equal(t, []any{"x", "y"}, b)
			},
		},
		{
			name:      "leading_dot_literal",
			yaml:      ".a: v",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "v", result[".a"])
			},
		},
		{
			name:      "trailing_dot_literal",
			yaml:      "a.: v",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "v", result["a."])
			},
		},
		{
			name:      "consecutive_dots_literal",
			yaml:      "a..b: v",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "v", result["a..b"])
			},
		},
		{
			name:      "custom_delimiter_double_colon",
			yaml:      "a::b::c: v",
			delimiter: "::",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				b, ok := a["b"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "v", b["c"])
			},
		},
		{
			name:      "custom_delimiter_dots_not_expanded",
			yaml:      "a.b: v",
			delimiter: "::",
			check: func(t *testing.T, result map[string]any) {
				// Dots are literal when delimiter is ::.
				assert.Equal(t, "v", result["a.b"])
			},
		},
		{
			name:      "empty_map",
			yaml:      "{}",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				assert.Empty(t, result)
			},
		},
		{
			name:      "map_value_under_dotted_key",
			yaml:      "a.b:\n  c: 1\n  d: 2",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				a, ok := result["a"].(map[string]any)
				require.True(t, ok)
				b, ok := a["b"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, 1, b["c"])
				assert.Equal(t, 2, b["d"])
			},
		},
		{
			name:      "deeply_nested_parent_with_dotted_child",
			yaml:      "components:\n  terraform:\n    vpc:\n      metadata.component: vpc-base",
			delimiter: ".",
			check: func(t *testing.T, result map[string]any) {
				components := result["components"].(map[string]any)
				terraform := components["terraform"].(map[string]any)
				vpc := terraform["vpc"].(map[string]any)
				metadata, ok := vpc["metadata"].(map[string]any)
				require.True(t, ok, "metadata should be a nested map, not a literal key")
				assert.Equal(t, "vpc-base", metadata["component"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndExpand(t, tt.yaml, tt.delimiter)
			tt.check(t, result)
		})
	}
}

func TestKeyDelimiters_NilNode(t *testing.T) {
	// Should not panic.
	KeyDelimiters(nil, ".")
}

func TestKeyDelimiters_EmptyDelimiter(t *testing.T) {
	var node goyaml.Node
	err := goyaml.Unmarshal([]byte("a.b: v"), &node)
	require.NoError(t, err)

	// Empty delimiter = no expansion.
	KeyDelimiters(&node, "")

	var result map[string]any
	err = node.Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "v", result["a.b"])
}
