package scalar

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goyaml "gopkg.in/yaml.v3"
)

func decode(t *testing.T, input string) map[string]any {
	t.Helper()

	var node goyaml.Node
	require.NoError(t, goyaml.Unmarshal([]byte(input), &node))
	PreserveDigitStrings(&node)

	var out map[string]any
	require.NoError(t, node.Decode(&out))
	return out
}

func TestPreserveDigitStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{"leading zero non-octal digits", "v: 068007702576", "068007702576"},
		{"leading zero 12 digits", "v: 000123456789", "000123456789"},
		{"integer beyond uint64", "v: 123456789012345678901234567890", "123456789012345678901234567890"},
		{"signed leading zero", "v: -08", "-08"},
		{"quoted string untouched", `v: "068007702576"`, "068007702576"},
		{"plain 12 digit int untouched", "v: 123456789012", 123456789012},
		{"max int64 untouched", "v: 9223372036854775807", 9223372036854775807},
		{"valid octal untouched", "v: 0755", 493},
		{"float untouched", "v: 1.5", 1.5},
		{"float with leading zero untouched", "v: 08.5", 8.5},
		{"exponent float untouched", "v: 1e3", 1000.0},
		{"bool untouched", "v: true", true},
		{"plain string untouched", "v: hello", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decode(t, tt.input)
			assert.Equal(t, tt.expected, got["v"])
			assert.IsType(t, tt.expected, got["v"])
		})
	}
}

func TestPreserveDigitStrings_Nested(t *testing.T) {
	got := decode(t, `
vars:
  list:
    - 068007702576
    - 7
  map:
    id: 000123456789
    n: 5
`)
	vars := got["vars"].(map[string]any)
	assert.Equal(t, []any{"068007702576", 7}, vars["list"])
	assert.Equal(t, map[string]any{"id": "000123456789", "n": 5}, vars["map"])
}

func TestPreserveDigitStrings_NilNode(t *testing.T) {
	assert.NotPanics(t, func() { PreserveDigitStrings(nil) })
}

func TestIsSignedDigits(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"0123", true},
		{"+12", true},
		{"-12", true},
		{"", false},
		{"-", false},
		{"1.5", false},
		{"1e3", false},
		{"12a", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, isSignedDigits(tt.in))
		})
	}
}
