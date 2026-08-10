package yaml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Value type names accepted by SetWithType / SetFileWithType. These let CLI
// callers coerce a string argument into a typed YAML scalar (or a raw YAML
// literal) instead of always writing a quoted string.
const (
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
			return "", fmt.Errorf("%w: %q is not an integer", ErrInvalidYAMLExpression, value)
		}
		return value, nil
	case TypeFloat:
		if _, err := strconv.ParseFloat(value, bitSize64); err != nil {
			return "", fmt.Errorf("%w: %q is not a float", ErrInvalidYAMLExpression, value)
		}
		return value, nil
	case TypeBool:
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return "", fmt.Errorf("%w: %q is not a boolean", ErrInvalidYAMLExpression, value)
		}
		return strconv.FormatBool(b), nil
	default:
		return "", fmt.Errorf("%w: unknown value type %q", ErrInvalidYAMLExpression, valueType)
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
