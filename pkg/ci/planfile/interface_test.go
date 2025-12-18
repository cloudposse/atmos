package planfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultKeyPattern(t *testing.T) {
	pattern := DefaultKeyPattern()
	assert.Equal(t, "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan", pattern.Pattern)
}

func TestKeyPatternGenerateKey(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		ctx      *KeyContext
		expected string
	}{
		{
			name:    "default pattern",
			pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan",
			ctx: &KeyContext{
				Stack:     "plat-ue2-dev",
				Component: "vpc",
				SHA:       "abc123",
			},
			expected: "plat-ue2-dev/vpc/abc123.tfplan",
		},
		{
			name:    "pattern with branch",
			pattern: "{{ .Branch }}/{{ .Stack }}/{{ .Component }}.tfplan",
			ctx: &KeyContext{
				Stack:     "plat-ue2-dev",
				Component: "vpc",
				Branch:    "feature-branch",
			},
			expected: "feature-branch/plat-ue2-dev/vpc.tfplan",
		},
		{
			name:    "empty values stay as placeholders",
			pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan",
			ctx: &KeyContext{
				Stack:     "plat-ue2-dev",
				Component: "vpc",
				// SHA is empty
			},
			expected: "plat-ue2-dev/vpc/{{ .SHA }}.tfplan",
		},
		{
			name:    "component path",
			pattern: "{{ .ComponentPath }}/{{ .SHA }}.tfplan",
			ctx: &KeyContext{
				ComponentPath: "components/terraform/vpc",
				SHA:           "abc123",
			},
			expected: "components/terraform/vpc/abc123.tfplan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := KeyPattern{Pattern: tt.pattern}
			key, err := pattern.GenerateKey(tt.ctx)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, key)
		})
	}
}

func TestReplaceAll(t *testing.T) {
	tests := []struct {
		s        string
		old      string
		new      string
		expected string
	}{
		{"hello world", "world", "go", "hello go"},
		{"foo bar foo", "foo", "baz", "baz bar baz"},
		{"no match", "xyz", "abc", "no match"},
		{"", "foo", "bar", ""},
	}

	for _, tt := range tests {
		result := replaceAll(tt.s, tt.old, tt.new)
		assert.Equal(t, tt.expected, result)
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected int
	}{
		{"hello world", "world", 6},
		{"hello world", "hello", 0},
		{"hello world", "xyz", -1},
		{"", "foo", -1},
		{"foo", "", 0},
	}

	for _, tt := range tests {
		result := indexOf(tt.s, tt.substr)
		assert.Equal(t, tt.expected, result)
	}
}
