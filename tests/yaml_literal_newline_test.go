package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// TestYAMLLiteralNewlineParsing tests how YAML parses literal newlines.
func TestYAMLLiteralNewlineParsing(t *testing.T) {
	testCases := []struct {
		name     string
		yaml     string
		expected string
	}{
		{
			name:     "double-quoted with escape sequences",
			yaml:     `foo: "bar\nbaz\nbongo\n"`,
			expected: "bar\nbaz\nbongo\n",
		},
		{
			name: "literal style with pipe",
			yaml: `foo: |
  bar
  baz
  bongo
`,
			expected: "bar\nbaz\nbongo\n",
		},
		{
			name: "folded style",
			yaml: `foo: >
  bar
  baz
  bongo
`,
			expected: "bar baz bongo\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			err := yaml.Unmarshal([]byte(tc.yaml), &data)
			assert.NoError(t, err)

			foo, ok := data["foo"].(string)
			assert.True(t, ok, "foo should be a string")
			assert.Equal(t, tc.expected, foo, "YAML parsing should produce expected value")
		})
	}
}
