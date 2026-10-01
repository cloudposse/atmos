package config

import (
	"fmt"
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

// ParseUseMocksFlag interprets the value of the string-valued --use-mocks flag.
//
// An empty value or "false" turns mocks off. "true" (a bare --use-mocks) turns them on using the
// effective components.terraform.mocks.mode, so the returned mode is empty. "fallback" or
// "always" turn mocks on and override components.terraform.mocks.mode for this run. Any other
// value is rejected with errUtils.ErrInvalidFlagValue.
func ParseUseMocksFlag(raw string) (enabled bool, mode string, err error) {
	switch normalized := strings.ToLower(strings.TrimSpace(raw)); normalized {
	case "", UseMocksFalse:
		return false, "", nil
	case UseMocksTrue:
		return true, "", nil
	case string(schema.TerraformMocksModeFallback), string(schema.TerraformMocksModeAlways):
		return true, normalized, nil
	default:
		return false, "", fmt.Errorf("%w: --use-mocks accepts true, false, %s, or %s, got %q",
			errUtils.ErrInvalidFlagValue, schema.TerraformMocksModeFallback, schema.TerraformMocksModeAlways, raw)
	}
}
