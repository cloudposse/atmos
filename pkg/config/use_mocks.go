package config

import (
	"fmt"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// UseMocksTrue is the value a bare --use-mocks (NoOptDefVal), --use-mocks=true, or
// ATMOS_USE_MOCKS=true carries: mocks are on and the mode comes from
// components.terraform.mocks.mode.
const UseMocksTrue = "true"

// UseMocksFalse is the explicit "off" value of --use-mocks.
const UseMocksFalse = "false"

// UseMocksFlagAndEnvSource names both places a `atmos terraform` --use-mocks value can come from,
// for error messages that cannot tell which one supplied it.
const UseMocksFlagAndEnvSource = UseMocksFlag + " (or ATMOS_USE_MOCKS)"

// ParseUseMocksFlag interprets the value of the string-valued --use-mocks flag.
//
// An empty value or a false boolean ("false", "0", "f") turns mocks off. A true boolean ("true",
// "1", "t"; a bare --use-mocks is "true") turns them on using the effective
// components.terraform.mocks.mode, so the returned mode is empty. "fallback" or "always" turn
// mocks on and override components.terraform.mocks.mode for this run. Matching is
// case-insensitive, and the boolean forms are the ones the flag accepted when it was a plain
// boolean. Any other value is rejected with errUtils.ErrInvalidFlagValue.
func ParseUseMocksFlag(raw string) (enabled bool, mode string, err error) {
	return ParseUseMocksValue(raw, UseMocksFlag)
}

// ParseUseMocksValue is ParseUseMocksFlag with the value's source (for example
// UseMocksFlagAndEnvSource) named in the error message.
func ParseUseMocksValue(raw, source string) (enabled bool, mode string, err error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "":
		return false, "", nil
	case string(schema.TerraformMocksModeFallback), string(schema.TerraformMocksModeAlways):
		return true, normalized, nil
	}
	if on, parseErr := strconv.ParseBool(normalized); parseErr == nil {
		return on, "", nil
	}
	return false, "", fmt.Errorf("%w: %s accepts true, false, %s, or %s, got %q",
		errUtils.ErrInvalidFlagValue, source, schema.TerraformMocksModeFallback, schema.TerraformMocksModeAlways, raw)
}

// CheckUseMocksSeparatedMode rejects `--use-mocks always` (a space instead of `=`). Because a bare
// --use-mocks takes no value, the mode word is parsed as a positional argument: a component name
// for `atmos describe component`, or a stray argument passed to `terraform plan`. The args are
// the positional arguments that follow the component; enabled and mode are ParseUseMocksFlag's result.
func CheckUseMocksSeparatedMode(enabled bool, mode string, args []string) error {
	if !enabled || mode != "" {
		return nil
	}
	for _, arg := range args {
		candidate := strings.ToLower(strings.TrimSpace(arg))
		if candidate != string(schema.TerraformMocksModeFallback) && candidate != string(schema.TerraformMocksModeAlways) {
			continue
		}
		return errUtils.Build(fmt.Errorf("%w: %s received %q as a separate argument", errUtils.ErrInvalidFlagValue, UseMocksFlag, arg)).
			WithHintf("Attach the mode with an equals sign: `%s=%s`. A bare `%s` takes no value.", UseMocksFlag, candidate, UseMocksFlag).
			Err()
	}
	return nil
}
