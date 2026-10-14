// Package standalone holds the input handling shared by standalone scripts that declare their own
// command-line interface: value parsing, positional validation, help annotations, and usage errors.
package standalone

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

const decimalBase = 10

// DecimalInt is a pflag.Value for integer flags. Unlike pflag's own integer flags it accepts only
// base-10 digits, so `010` is ten and `0x10` is rejected instead of being read as octal or hex.
type DecimalInt struct{ value int }

// NewDecimalInt creates a DecimalInt holding the flag's default value.
func NewDecimalInt(initial int) *DecimalInt {
	defer perf.Track(nil, "standalone.NewDecimalInt")()

	return &DecimalInt{value: initial}
}

// Set parses a base-10 integer.
func (v *DecimalInt) Set(raw string) error {
	defer perf.Track(nil, "standalone.DecimalInt.Set")()

	parsed, err := ParseDecimalInt(raw)
	if err != nil {
		return err
	}
	v.value = parsed
	return nil
}

// String returns the current value in decimal, which is the text pflag reads typed values from.
func (v *DecimalInt) String() string {
	defer perf.Track(nil, "standalone.DecimalInt.String")()

	return strconv.Itoa(v.value)
}

// Type names the flag type shown in help output.
func (v *DecimalInt) Type() string {
	defer perf.Track(nil, "standalone.DecimalInt.Type")()

	return "int"
}

// ParseDecimalInt parses raw as a base-10 integer, for both command-line and environment values.
func ParseDecimalInt(raw string) (int, error) {
	defer perf.Track(nil, "standalone.ParseDecimalInt")()

	parsed, err := strconv.ParseInt(raw, decimalBase, strconv.IntSize)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a base-10 integer", errUtils.ErrInvalidFlagValue, raw)
	}
	return int(parsed), nil
}

// ParseStringList splits a list value the way pflag's StringSlice does for command-line values:
// comma-separated, with CSV quoting for items that contain commas. An empty value is an empty list.
func ParseStringList(raw string) ([]string, error) {
	defer perf.Track(nil, "standalone.ParseStringList")()

	if raw == "" {
		return []string{}, nil
	}
	items, err := csv.NewReader(strings.NewReader(raw)).Read()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidFlagValue, err)
	}
	return items, nil
}

// ValidatePositionals checks the number of positional values against the declared arguments and
// names the first missing required argument.
func ValidatePositionals(specs []*flags.PositionalArgSpec, positional []string) error {
	defer perf.Track(nil, "standalone.ValidatePositionals")()

	for index, spec := range specs {
		if spec.Required && index >= len(positional) {
			return fmt.Errorf("%w: missing required argument `<%s>`", errUtils.ErrScriptUsage, spec.Name)
		}
	}
	if len(positional) <= len(specs) {
		return nil
	}
	extra := positional[len(specs)]
	if len(specs) == 0 {
		return fmt.Errorf("%w: unexpected argument %q, this script takes no positional arguments", errUtils.ErrScriptUsage, extra)
	}
	return fmt.Errorf("%w: unexpected argument %q, this script takes at most %d positional argument(s)", errUtils.ErrScriptUsage, extra, len(specs))
}
