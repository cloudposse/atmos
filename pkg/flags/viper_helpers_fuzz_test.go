package flags

import "testing"

// FuzzParseKeyValuePair exercises the "key=value" splitter used to parse CLI
// flag values with arbitrary input to catch panics on malformed pairs.
func FuzzParseKeyValuePair(f *testing.F) {
	f.Add("foo=bar")
	f.Add("  foo = bar  ")
	f.Add("foo=")
	f.Add("foo=bar=baz")
	f.Add("foobar")
	f.Add("=value")
	f.Add("   =value")
	f.Add("")
	f.Add("url=https://example.com/path?query=value")
	f.Add(`config={"key":"value"}`)

	f.Fuzz(func(t *testing.T, pair string) {
		_, _ = parseKeyValuePair(pair)
	})
}
