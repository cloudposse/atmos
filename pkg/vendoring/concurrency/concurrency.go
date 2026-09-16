// Package concurrency resolves vendoring's edition-aware worker limit.
package concurrency

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	Flag    = "max-concurrency"
	Env     = "ATMOS_VENDOR_MAX_CONCURRENCY"
	Default = 4
)

// Resolve applies explicit flag, environment, and edition-adjusted configuration precedence.
func Resolve(flags *pflag.FlagSet, config *schema.AtmosConfiguration) (int, error) {
	defer perf.Track(config, "concurrency.Resolve")()

	n := Default
	if config != nil {
		n = config.Vendor.MaxConcurrency
	}
	if flags != nil && flags.Changed(Flag) {
		value, err := flags.GetInt(Flag)
		if err != nil {
			return 0, err
		}
		n = value
	} else if value, ok := os.LookupEnv(Env); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("%w: %s must be a positive integer", errUtils.ErrInvalidFlagValue, Env)
		}
		n = parsed
	}
	if n < 1 {
		return 0, fmt.Errorf("%w: vendor.max_concurrency, %s, and --%s must be at least 1", errUtils.ErrInvalidFlagValue, Env, Flag)
	}
	return n, nil
}

// Effective resolves library options. Zero means the loaded configuration's default.
func Effective(config *schema.AtmosConfiguration, explicit int) (int, error) {
	defer perf.Track(config, "concurrency.Effective")()

	if explicit != 0 {
		if explicit < 1 {
			return 0, errUtils.ErrInvalidFlagValue
		}
		return explicit, nil
	}
	if config == nil || config.Vendor.MaxConcurrency == 0 {
		return Default, nil
	}
	if config.Vendor.MaxConcurrency < 1 {
		return 0, errUtils.ErrInvalidFlagValue
	}
	return config.Vendor.MaxConcurrency, nil
}
