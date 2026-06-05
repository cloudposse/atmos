package secret

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEnvSecrets(t *testing.T) {
	input := strings.NewReader("# comment\n\nA=1\nB=\"two\"\nC=three=with=eq\n")
	got, err := parseEnvSecrets(input)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"A": "1",
		"B": "two",
		"C": "three=with=eq",
	}, got)
}

func TestParseJSONSecrets(t *testing.T) {
	input := strings.NewReader(`{"A":"1","B":2}`)
	got, err := parseJSONSecrets(input)
	require.NoError(t, err)
	assert.Equal(t, "1", got["A"])
	assert.Equal(t, "2", got["B"])
}

func TestSortedKeys(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, sortedKeys(map[string]string{"c": "", "a": "", "b": ""}))
}

func TestFormatTable(t *testing.T) {
	out := formatTable([][]string{
		{"SECRET", "STATUS"},
		{"API_KEY", "initialized"},
	})
	lines := strings.Split(out, "\n")
	require.Len(t, lines, 2)
	assert.True(t, strings.HasPrefix(lines[0], "SECRET"))
	assert.Contains(t, lines[1], "API_KEY")
	assert.Contains(t, lines[1], "initialized")
}
