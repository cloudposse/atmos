package template

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFieldRefs(t *testing.T) {
	tests := []struct {
		name     string
		template string
		expected [][]string // Path slices
	}{
		{
			name:     "simple field",
			template: "{{ .foo }}",
			expected: [][]string{{"foo"}},
		},
		{
			name:     "nested field",
			template: "{{ .foo.bar }}",
			expected: [][]string{{"foo", "bar"}},
		},
		{
			name:     "multiple fields",
			template: "{{ .foo }}-{{ .bar }}",
			expected: [][]string{{"foo"}, {"bar"}},
		},
		{
			name:     "deeply nested",
			template: "{{ .a.b.c.d }}",
			expected: [][]string{{"a", "b", "c", "d"}},
		},
		{
			name:     "duplicate refs deduplicated",
			template: "{{ .foo }}-{{ .foo }}",
			expected: [][]string{{"foo"}},
		},
		{
			name:     "no template syntax",
			template: "just a plain string",
			expected: nil,
		},
		{
			name:     "empty string",
			template: "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, err := ExtractFieldRefs(tt.template)
			require.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, refs)
				return
			}

			assert.Len(t, refs, len(tt.expected))
			for i, expectedPath := range tt.expected {
				assert.Equal(t, expectedPath, refs[i].Path)
			}
		})
	}
}

func TestExtractFieldRefsByPrefix(t *testing.T) {
	tests := []struct {
		name     string
		template string
		prefix   string
		expected []string
	}{
		{
			name:     "simple local ref",
			template: "{{ .locals.foo }}",
			prefix:   "locals",
			expected: []string{"foo"},
		},
		{
			name:     "multiple local refs",
			template: "{{ .locals.foo }}-{{ .locals.bar }}",
			prefix:   "locals",
			expected: []string{"foo", "bar"},
		},
		{
			name:     "conditional with multiple refs",
			template: "{{ if .locals.flag }}{{ .locals.x }}{{ else }}{{ .locals.y }}{{ end }}",
			prefix:   "locals",
			expected: []string{"flag", "x", "y"},
		},
		{
			name:     "pipe expression",
			template: `{{ .locals.foo | printf "%s-%s" .locals.bar }}`,
			prefix:   "locals",
			expected: []string{"foo", "bar"},
		},
		{
			name:     "range block",
			template: "{{ range .locals.items }}{{ .locals.prefix }}-{{ . }}{{ end }}",
			prefix:   "locals",
			expected: []string{"items", "prefix"},
		},
		{
			name:     "with block - context change",
			template: "{{ with .locals.config }}{{ .name }}{{ end }}",
			prefix:   "locals",
			expected: []string{"config"}, // .name is NOT .locals.name inside with block
		},
		{
			name:     "mixed prefixes - only locals",
			template: "{{ .locals.a }}-{{ .vars.b }}-{{ .settings.c }}",
			prefix:   "locals",
			expected: []string{"a"},
		},
		{
			name:     "mixed prefixes - only vars",
			template: "{{ .locals.a }}-{{ .vars.b }}-{{ .settings.c }}",
			prefix:   "vars",
			expected: []string{"b"},
		},
		{
			name:     "nested conditionals",
			template: "{{ if .locals.a }}{{ if .locals.b }}{{ .locals.c }}{{ end }}{{ end }}",
			prefix:   "locals",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "no template syntax",
			template: "just a plain string",
			prefix:   "locals",
			expected: nil,
		},
		{
			name:     "deep path - only first level after prefix",
			template: "{{ .locals.config.nested.value }}",
			prefix:   "locals",
			expected: []string{"config"},
		},
		{
			name:     "comparison in if",
			template: `{{ if eq .locals.env "prod" }}{{ .locals.value }}{{ end }}`,
			prefix:   "locals",
			expected: []string{"env", "value"},
		},
		{
			name:     "single pipe with builtin",
			template: "{{ .locals.name | len }}",
			prefix:   "locals",
			expected: []string{"name"},
		},
		{
			name:     "printf with multiple refs",
			template: `{{ printf "%s-%s-%s" .locals.a .locals.b .locals.c }}`,
			prefix:   "locals",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "else if chain",
			template: "{{ if .locals.a }}x{{ else if .locals.b }}y{{ else }}{{ .locals.c }}{{ end }}",
			prefix:   "locals",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "range with else",
			template: "{{ range .locals.items }}{{ . }}{{ else }}{{ .locals.empty }}{{ end }}",
			prefix:   "locals",
			expected: []string{"items", "empty"},
		},
		{
			name:     "duplicate refs deduplicated",
			template: "{{ .locals.foo }}-{{ .locals.foo }}-{{ .locals.foo }}",
			prefix:   "locals",
			expected: []string{"foo"},
		},
		{
			name:     "no prefix match",
			template: "{{ .vars.foo }}",
			prefix:   "locals",
			expected: nil,
		},
		{
			name:     "single path element - no match",
			template: "{{ .foo }}",
			prefix:   "locals",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractFieldRefsByPrefix(tt.template, tt.prefix)
			require.NoError(t, err)

			// Sort for deterministic comparison.
			sort.Strings(result)
			if tt.expected != nil {
				sort.Strings(tt.expected)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractAllFieldRefsByPrefix(t *testing.T) {
	tests := []struct {
		name     string
		template string
		prefix   string
		expected []string
	}{
		{
			name:     "simple ref",
			template: "{{ .locals.foo }}",
			prefix:   "locals",
			expected: []string{"foo"},
		},
		{
			name:     "nested ref - full path",
			template: "{{ .locals.config.nested.value }}",
			prefix:   "locals",
			expected: []string{"config.nested.value"},
		},
		{
			name:     "multiple nested refs",
			template: "{{ .locals.a.b }}-{{ .locals.x.y.z }}",
			prefix:   "locals",
			expected: []string{"a.b", "x.y.z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractAllFieldRefsByPrefix(tt.template, tt.prefix)
			require.NoError(t, err)

			sort.Strings(result)
			if tt.expected != nil {
				sort.Strings(tt.expected)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasTemplateActions(t *testing.T) {
	tests := []struct {
		template string
		expected bool
	}{
		{"{{ .foo }}", true},
		{"{{ if .x }}y{{ end }}", true},
		{"{{ range .items }}{{ . }}{{ end }}", true},
		{"{{ with .config }}{{ .value }}{{ end }}", true},
		{"plain text", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.template, func(t *testing.T) {
			result, err := HasTemplateActions(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFieldRefString(t *testing.T) {
	tests := []struct {
		path     []string
		expected string
	}{
		{[]string{"foo"}, "foo"},
		{[]string{"locals", "bar"}, "locals.bar"},
		{[]string{"a", "b", "c"}, "a.b.c"},
		{nil, ""},
		{[]string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			ref := FieldRef{Path: tt.path}
			assert.Equal(t, tt.expected, ref.String())
		})
	}
}

func TestExtractFieldRefs_InvalidTemplate(t *testing.T) {
	// Invalid template syntax should return an error.
	_, err := ExtractFieldRefs("{{ .foo }")
	assert.Error(t, err)
}

func TestExtractFieldRefsByPrefix_InvalidTemplate(t *testing.T) {
	// Invalid template syntax should return an error.
	_, err := ExtractFieldRefsByPrefix("{{ .locals.foo }", "locals")
	assert.Error(t, err)
}

func TestHasTemplateActions_InvalidTemplate(t *testing.T) {
	// Invalid template syntax should return an error.
	_, err := HasTemplateActions("{{ .foo }")
	assert.Error(t, err)
}

func TestExtractFieldRefs_NilTreeRoot(t *testing.T) {
	// Test with a template that parses but has no meaningful content.
	refs, err := ExtractFieldRefs("plain text without any template")
	require.NoError(t, err)
	assert.Nil(t, refs)
}

func TestExtractFieldRefs_TemplateNode(t *testing.T) {
	// Test template that invokes another template with a pipe argument.
	// The .config field reference should be captured from the template invocation.
	refs, err := ExtractFieldRefs(`{{ template "inner" .config }}`)
	require.NoError(t, err)
	// The field reference in the template invocation is captured.
	assert.NotNil(t, refs)
	assert.Len(t, refs, 1)
	assert.Equal(t, []string{"config"}, refs[0].Path)
}

func TestExtractAllFieldRefsByPrefix_NoMatch(t *testing.T) {
	// Test when no refs match the prefix.
	result, err := ExtractAllFieldRefsByPrefix("{{ .vars.foo }}", "locals")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExtractAllFieldRefsByPrefix_InvalidTemplate(t *testing.T) {
	// Invalid template should return an error.
	_, err := ExtractAllFieldRefsByPrefix("{{ .locals.foo }", "locals")
	assert.Error(t, err)
}

func TestExtractAllFieldRefsByPrefix_NoTemplateDelimiters(t *testing.T) {
	// No template delimiters means no refs.
	result, err := ExtractAllFieldRefsByPrefix("plain text", "locals")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExtractFieldRefs_PipeWithDeclarations(t *testing.T) {
	// Test pipe node with variable declarations.
	refs, err := ExtractFieldRefs(`{{ $x := .foo }}{{ $x }}`)
	require.NoError(t, err)
	assert.Len(t, refs, 1)
	assert.Equal(t, []string{"foo"}, refs[0].Path)
}

func TestExtractFieldRefs_RangeWithElse(t *testing.T) {
	// Test range block with else clause.
	refs, err := ExtractFieldRefs(`{{ range .items }}{{ .name }}{{ else }}{{ .empty }}{{ end }}`)
	require.NoError(t, err)
	// Should capture .items, .name (within range context), .empty
	assert.NotEmpty(t, refs)
	// Verify specific expected references.
	refPaths := make(map[string]bool)
	for _, ref := range refs {
		refPaths[ref.String()] = true
	}
	assert.True(t, refPaths["items"], "should contain reference to .items")
	assert.True(t, refPaths["name"], "should contain reference to .name")
	assert.True(t, refPaths["empty"], "should contain reference to .empty")
}

func TestExtractFieldRefs_WithBlock(t *testing.T) {
	// Test with block that changes context.
	refs, err := ExtractFieldRefs(`{{ with .config }}{{ .value }}{{ end }}`)
	require.NoError(t, err)
	// Should capture both .config and .value
	foundConfig := false
	foundValue := false
	for _, ref := range refs {
		if len(ref.Path) == 1 && ref.Path[0] == "config" {
			foundConfig = true
		}
		if len(ref.Path) == 1 && ref.Path[0] == "value" {
			foundValue = true
		}
	}
	assert.True(t, foundConfig, "should capture .config")
	assert.True(t, foundValue, "should capture .value")
}

func TestExtractFieldRefs_NestedRangeAndIf(t *testing.T) {
	// Test deeply nested control structures.
	refs, err := ExtractFieldRefs(`{{ range .items }}{{ if .active }}{{ .name }}{{ end }}{{ end }}`)
	require.NoError(t, err)
	assert.NotEmpty(t, refs)
}

func TestExtractFieldRefs_CommandWithMultipleArgs(t *testing.T) {
	// Test command node with multiple arguments.
	refs, err := ExtractFieldRefs(`{{ printf "%s %s" .first .second }}`)
	require.NoError(t, err)
	assert.Len(t, refs, 2)
}

func TestHasTemplateActions_RangeAction(t *testing.T) {
	// Test that range is detected as an action.
	result, err := HasTemplateActions(`{{ range .items }}{{ . }}{{ end }}`)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestHasTemplateActions_WithAction(t *testing.T) {
	// Test that with is detected as an action.
	result, err := HasTemplateActions(`{{ with .config }}{{ .value }}{{ end }}`)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestHasTemplateActions_NoActionsJustText(t *testing.T) {
	// Test that text nodes without actions return false.
	result, err := HasTemplateActions(`{{ "literal text" }}`)
	require.NoError(t, err)
	// This is actually an action (ActionNode), so it returns true.
	assert.True(t, result)
}

func TestExtractFieldRefsByPrefix_SinglePathElement(t *testing.T) {
	// When path has only one element, it doesn't match prefix.second pattern.
	result, err := ExtractFieldRefsByPrefix("{{ .foo }}", "foo")
	require.NoError(t, err)
	// .foo doesn't match the pattern .foo.X
	assert.Nil(t, result)
}

func TestExtractFieldRefs_ElseIfChain(t *testing.T) {
	// Test else-if chain parsing.
	refs, err := ExtractFieldRefs(`{{ if .a }}1{{ else if .b }}2{{ else if .c }}3{{ else }}{{ .d }}{{ end }}`)
	require.NoError(t, err)
	// Should find exactly a, b, c, d.
	assert.Len(t, refs, 4, "else-if chain should have exactly 4 references")
}
