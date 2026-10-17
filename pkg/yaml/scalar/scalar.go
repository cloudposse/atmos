// Package scalar provides yaml.Node normalization for scalars that the YAML decoder
// would otherwise convert lossily.
package scalar

import (
	goyaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

const floatTag = "!!float"

// PreserveDigitStrings walks a yaml.Node tree and retags plain (unquoted) scalars that consist
// solely of decimal digits, but that the YAML decoder would resolve to a float64, as strings.
//
// The yaml.v3 decoder resolves an unquoted integer-looking scalar that is not a valid int (for example
// 068007702576, which has a leading zero and a non-octal digit, or an integer too large for uint64)
// to a float64. That silently loses information: the leading zero is dropped and the value is
// rendered in exponent notation (6.8007702576e+10). Typical victims are AWS account IDs. Keeping
// the original text as a string round-trips these values exactly.
//
// Scalars that yaml.v3 resolves to integers (including valid YAML 1.1 octal such as 0755) and real
// floats (such as 1.5 or 1e3) are left untouched, as are quoted scalars and explicitly tagged ones.
func PreserveDigitStrings(node *goyaml.Node) {
	defer perf.Track(nil, "yaml.scalar.PreserveDigitStrings")()

	preserveDigitStrings(node)
}

// preserveDigitStrings is the recursive implementation.
// Separated from the public entry point so perf.Track fires only once.
func preserveDigitStrings(node *goyaml.Node) {
	if node == nil {
		return
	}

	if node.Kind == goyaml.ScalarNode && node.Style == 0 && node.Tag == floatTag && isSignedDigits(node.Value) {
		node.Tag = "!!str"
	}

	for _, child := range node.Content {
		preserveDigitStrings(child)
	}
}

// isSignedDigits reports whether s is an optional sign followed by one or more ASCII digits.
func isSignedDigits(s string) bool {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
