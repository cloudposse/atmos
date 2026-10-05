package utils

import "testing"

// FuzzUnmarshalYAML exercises the YAML-to-map parsing path with arbitrary,
// potentially malformed input to catch panics in the underlying yaml.v3 decoder
// and Atmos's custom-tag handling.
func FuzzUnmarshalYAML(f *testing.F) {
	f.Add("---\nhello: world")
	f.Add("Not YAML")
	f.Add("")
	f.Add("key: [1, 2, 3]")
	f.Add("a: &anchor\n  b: 1\nc: *anchor")
	f.Add("k: !custom-tag value")
	f.Add("k: !!str 123")

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = UnmarshalYAML[map[any]any](input)
	})
}
