package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// MatchPattern accepts a plain string, the `!not` tag, and the `{not: pattern}` map form that
// tests/test-cases/schema.json documents for stdout, stderr, tty, and file_contains entries.
func TestMatchPatternUnmarshalForms(t *testing.T) {
	var patterns []MatchPattern
	require.NoError(t, yaml.Unmarshal([]byte("- '(?m)^vars:'\n- !not '(?m)^region:'\n- not: '(?m)^timeout:'\n"), &patterns))
	assert.Equal(t, []MatchPattern{
		{Pattern: "(?m)^vars:"},
		{Pattern: "(?m)^region:", Negate: true},
		{Pattern: "(?m)^timeout:", Negate: true},
	}, patterns)
}

func TestMatchPatternRejectsOtherMaps(t *testing.T) {
	var patterns []MatchPattern
	err := yaml.Unmarshal([]byte("- {pattern: x}\n"), &patterns)
	require.Error(t, err)
	err = yaml.Unmarshal([]byte("- {not: x, extra: y}\n"), &patterns)
	require.Error(t, err)
	err = yaml.Unmarshal([]byte("- 42\n"), &patterns)
	require.Error(t, err)
	err = yaml.Unmarshal([]byte("- not: 42\n"), &patterns)
	require.Error(t, err, "a non-string not value must not be coerced into a pattern")
	err = yaml.Unmarshal([]byte("- not: [a]\n"), &patterns)
	require.Error(t, err)
}
