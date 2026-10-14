package customcommand

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Supported values of a custom command flag's `type:`.
const (
	// FlagTypeString is the default flag type. Its value is a string.
	FlagTypeString = "string"
	// FlagTypeBool is a boolean flag.
	FlagTypeBool = "bool"
	// FlagTypeInt is an integer flag. Its value is a Go int in templates and scripts.
	FlagTypeInt = "int"
)

// SupportedFlagTypes lists the accepted values of a flag's `type:`, in documentation order.
func SupportedFlagTypes() []string {
	defer perf.Track(nil, "customcommand.SupportedFlagTypes")()

	return []string{FlagTypeString, FlagTypeBool, FlagTypeInt}
}

// EffectiveFlagType returns the flag's type, treating an empty type as the default string type.
func EffectiveFlagType(flag *schema.CommandFlag) string {
	defer perf.Track(nil, "customcommand.EffectiveFlagType")()

	if flag.Type == "" {
		return FlagTypeString
	}
	return flag.Type
}

// ValidateFlag fails when the flag declares a type Atmos does not support, or a combination of
// options that cannot work for that type. Custom commands call it at registration, so a typo in
// atmos.yaml fails loudly instead of quietly registering a string flag.
func ValidateFlag(commandName string, flag *schema.CommandFlag) error {
	defer perf.Track(nil, "customcommand.ValidateFlag")()

	flagType := EffectiveFlagType(flag)
	supported := false
	for _, candidate := range SupportedFlagTypes() {
		if flagType == candidate {
			supported = true
			break
		}
	}
	if !supported {
		return errUtils.Build(errUtils.ErrCustomCommandFlagType).
			WithExplanationf("Custom command '%s' declares flag '--%s' with type '%s'.", commandName, flag.Name, flag.Type).
			WithHintf("Supported flag types: %s.", strings.Join(SupportedFlagTypes(), ", ")).
			WithContext("command", commandName).
			WithContext("flag", flag.Name).
			WithContext("type", flag.Type).
			Err()
	}
	if flagType == FlagTypeInt {
		if _, err := IntFlagDefault(flag); err != nil {
			return errUtils.Build(err).
				WithContext("command", commandName).
				WithContext("flag", flag.Name).
				Err()
		}
		if len(flag.Values) > 0 {
			return errUtils.Build(errUtils.ErrCustomCommandFlagType).
				WithExplanationf("Custom command '%s' declares `values` for the int flag '--%s'.", commandName, flag.Name).
				WithHint("Only string flags can restrict their values to a fixed list.").
				WithContext("command", commandName).
				WithContext("flag", flag.Name).
				Err()
		}
	}
	return nil
}

// IntFlagDefault returns the default of an int flag. A flag with no default is 0. The YAML
// default may be an integer, a whole-number float, or a numeric string.
func IntFlagDefault(flag *schema.CommandFlag) (int, error) {
	defer perf.Track(nil, "customcommand.IntFlagDefault")()

	value, ok := intDefaultValue(flag.Default)
	if !ok {
		return 0, errUtils.Build(errUtils.ErrCustomCommandFlagDefault).
			WithExplanationf("The default %v of int flag '--%s' is not an integer.", flag.Default, flag.Name).
			WithHint("Use a whole number such as `default: 3`.").
			Err()
	}
	return value, nil
}

// intDefaultValue converts a YAML-decoded default to an int, reporting false when it is not a whole number.
func intDefaultValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case nil:
		return 0, true
	case int:
		return value, true
	case int64:
		return signedIntDefault(value)
	case uint64:
		if value > math.MaxInt {
			return 0, false
		}
		return int(value), true
	case float64:
		return floatToInt(value)
	case string:
		return parseIntDefault(value)
	default:
		return 0, false
	}
}

func signedIntDefault(value int64) (int, bool) {
	if value < math.MinInt || value > math.MaxInt {
		return 0, false
	}
	return int(value), true
}

// floatToInt converts a whole-number float to an int, reporting false for NaN, infinities,
// fractions, and values outside the int range. The upper bound is exclusive: float64(math.MaxInt)
// rounds up to 2^63 (the first value that does not fit), while -float64(math.MinInt) is exactly
// that same power of two, so `< -float64(math.MinInt)` is the precise limit on any int width.
func floatToInt(value float64) (int, bool) {
	if value != math.Trunc(value) || value < float64(math.MinInt) || value >= -float64(math.MinInt) {
		return 0, false
	}
	return int(value), true
}

// parseIntDefault parses a numeric string default. A blank string means no default.
func parseIntDefault(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

// ParseIntFlagValue converts the string form of an int flag value into an int.
func ParseIntFlagValue(name, value string) (int, error) {
	defer perf.Track(nil, "customcommand.ParseIntFlagValue")()

	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("%w: flag --%s: %w", errUtils.ErrInvalidFlag, name, err)
	}
	return parsed, nil
}
