package flags

import "testing"

// FuzzParseClosureDepth exercises the depth-carrying closure flag parser with
// arbitrary input to ensure malformed values are always rejected with an error,
// never a panic.
func FuzzParseClosureDepth(f *testing.F) {
	f.Add("")
	f.Add(ClosureDepthUnlimited)
	f.Add("true")
	f.Add("TRUE")
	f.Add("false")
	f.Add("0")
	f.Add("1")
	f.Add("3")
	f.Add(" 2 ")
	f.Add("-2")
	f.Add("banana")
	f.Add("1.5")

	f.Fuzz(func(t *testing.T, value string) {
		_, _ = ParseClosureDepth("include-dependencies", value)
	})
}
