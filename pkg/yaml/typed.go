package yaml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Value type names accepted by SetWithType / SetFileWithType. These let CLI
// callers coerce a string argument into a typed YAML scalar (or a raw YAML
// literal) instead of always writing a quoted string.
const (
	// TypeAuto infers the type: from the Atmos config schema when the path is
	// modeled, otherwise from the type of the value already at that path (if
	// any), otherwise string. This is the default for SetFileWithType callers
	// (config set, stack set) -- it is never passed through to buildRHS, which
	// only ever sees a type TypeAuto has already been resolved to.
	TypeAuto   = "auto"
	TypeString = "string"
	TypeInt    = "int"
	TypeBool   = "bool"
	TypeFloat  = "float"
	TypeNull   = "null"
	// TypeYAML treats the value as a raw YAML/yq literal (e.g. `[1,2,3]`,
	// `{a: 1}`, or an unquoted scalar), inserted verbatim.
	TypeYAML = "yaml"
)

const (
	// Decimal base used to validate integer values.
	decimalBase = 10
	// Bit size used to validate int and float values.
	bitSize64 = 64
)

// looksNonStringPattern matches raw CLI values that read as a bool or a
// number -- the shape a user would plausibly intend as something other than
// a literal string. The numeric branch covers signed integers/decimals
// (leading- or trailing-dot forms like ".5" and "5."), underscore-separated
// digit groups (e.g. "1_000"), scientific notation (e.g. "1e3"), and the
// literal "nan" -- all shapes strconv.ParseFloat already accepts today (see
// buildValidatedRHS), so warning for them is accurate.
//
// Deliberately NOT matched: hex ("0x1A"), octal ("0o17"), and ".inf"/"-.inf".
// These are legal YAML 1.1 numeric scalars, but strconv.ParseInt(v, 10, 64)
// and strconv.ParseFloat(v, 64) both reject them outright (ParseInt only
// accepts 0x/0o prefixes with base 0, and Go's float grammar requires
// "inf"/"Inf"/"INF" without a leading dot) -- warning "pass --type=int/float"
// for these would send a user straight into a parser error, a worse outcome
// than today's silent string. Extending the parsers to accept them too is a
// separate, larger behavior change, tracked as a follow-up rather than
// bundled here.
var looksNonStringPattern = regexp.MustCompile(`(?i)^(?:true|false|nan|[+-]?(?:[0-9](?:_?[0-9])*(?:\.[0-9_]*)?|\.[0-9](?:_?[0-9])*)(?:e[+-]?[0-9]+)?)$`)

// LooksNonString reports whether raw looks like it was meant to be a bool or
// number (e.g. "true", "42", "3.14") rather than a literal string. Callers use
// this to warn when a value with this shape is about to be stored as a
// TypeString because type inference wasn't available or --type wasn't passed
// -- silently writing `"true"` instead of `true` is easy to miss.
func LooksNonString(raw string) bool {
	defer perf.Track(nil, "yaml.LooksNonString")()

	return looksNonStringPattern.MatchString(strings.TrimSpace(raw))
}

// buildRHS coerces a CLI string value into a yq right-hand-side expression
// according to valueType. An empty valueType defaults to TypeString. Plain
// (non-validating) types are handled here; numeric/boolean types that need
// validation are delegated to buildValidatedRHS to keep this function flat.
func buildRHS(value, valueType string) (string, error) {
	switch valueType {
	case "", TypeString:
		return encodeStringValue(value), nil
	case TypeYAML:
		return value, nil
	case TypeNull:
		return "null", nil
	default:
		return buildValidatedRHS(value, valueType)
	}
}

// buildValidatedRHS handles value types whose input must be validated before it
// is inserted verbatim as a typed YAML scalar.
func buildValidatedRHS(value, valueType string) (string, error) {
	switch valueType {
	case TypeInt:
		if _, err := strconv.ParseInt(value, decimalBase, bitSize64); err != nil {
			return "", fmt.Errorf("%w: %q is not an integer", ErrInvalidTypedValue, value)
		}
		return value, nil
	case TypeFloat:
		if _, err := strconv.ParseFloat(value, bitSize64); err != nil {
			return "", fmt.Errorf("%w: %q is not a float", ErrInvalidTypedValue, value)
		}
		return value, nil
	case TypeBool:
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return "", fmt.Errorf("%w: %q is not a boolean", ErrInvalidTypedValue, value)
		}
		return strconv.FormatBool(b), nil
	default:
		return "", fmt.Errorf("%w: unknown value type %q", ErrInvalidTypedValue, valueType)
	}
}

// SetWithType assigns value at path, coercing it according to valueType, and
// returns the modified document. See the Type* constants for valid types.
func SetWithType(content []byte, path, value, valueType string) ([]byte, error) {
	defer perf.Track(nil, "yaml.SetWithType")()

	rhs, err := buildRHS(value, valueType)
	if err != nil {
		return nil, err
	}
	return SetRaw(content, path, rhs)
}

// SetFileWithType assigns a typed value at path in a YAML file (atomic
// write). It returns whether the path was newly created (true) as opposed to
// already existing and being updated (false), letting callers distinguish
// the two for user-facing messaging.
func SetFileWithType(filePath, path, value, valueType string) (bool, error) {
	defer perf.Track(nil, "yaml.SetFileWithType")()

	rhs, err := buildRHS(value, valueType)
	if err != nil {
		return false, err
	}

	existed, err := fileHasPath(filePath, path)
	if err != nil {
		return false, err
	}

	if err := SetFileRaw(filePath, path, rhs); err != nil {
		return false, err
	}
	return !existed, nil
}
