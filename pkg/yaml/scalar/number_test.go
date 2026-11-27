package scalar

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func TestNativeNumber(t *testing.T) {
	for _, tc := range []struct {
		text  string
		value any
	}{
		{"42", int64(42)},
		{"18446744073709551615", uint64(18446744073709551615)},
		{"1.75", 1.75},
		{"18446744073709551617", Number("18446744073709551617")},
		{"0.1234567890123456789012345", Number("0.1234567890123456789012345")},
		{"1e+500", Number("1e+500")},
	} {
		n := NativeNumber(json.Number(tc.text))
		require.Equal(t, tc.value, n)
		b, err := json.Marshal(n)
		require.NoError(t, err)
		require.Equal(t, tc.text, string(b))
	}
}

func TestNumberKeepsYAMLNumericType(t *testing.T) {
	for _, tc := range []struct{ text, tag string }{
		{"123456789012345678901234567890123456789", "!!int"},
		{"0.12345678901234567890123456789", "!!float"},
		{"1e+500", "!!float"},
	} {
		var node yaml.Node
		require.NoError(t, node.Encode(Number(tc.text)))
		require.Equal(t, tc.tag, node.Tag)
		require.Equal(t, tc.text, node.Value)
		require.Equal(t, yaml.TaggedStyle, node.Style)
	}
}
