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
