package global

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// PagerSelector handles the pager flag which has three states:
// 1. Not set (use config/env default)
// 2. Set without value (--pager) → enable with default pager
// 3. Set with value (--pager=less or --pager=false) → use specific pager or disable
//
// This type encapsulates the NoOptDefVal semantics of the pager flag.
type PagerSelector struct {
	value    string
	provided bool
}

// NewPagerSelector creates a PagerSelector from flag state.
func NewPagerSelector(value string, provided bool) PagerSelector {
	defer perf.Track(nil, "flags.NewPagerSelector")()

	return PagerSelector{
		value:    value,
		provided: provided,
	}
}

// IsEnabled returns true if pager should be enabled.
// Returns false if explicitly set to "false" or not provided.
func (p PagerSelector) IsEnabled() bool {
	defer perf.Track(nil, "flags.PagerSelector.IsEnabled")()

	if !p.provided {
		return false // Not set, use default from config
	}
	return p.value != "false"
}

// Pager returns the pager command to use.
// Returns empty string if using default pager or if disabled.
func (p PagerSelector) Pager() string {
	defer perf.Track(nil, "flags.PagerSelector.Pager")()

	if p.value == "true" || p.value == "" || p.value == "false" {
		return "" // Use default pager or disabled
	}
	return p.value // Specific pager (e.g., "less", "more")
}

// IsProvided returns true if the flag was explicitly set.
func (p PagerSelector) IsProvided() bool {
	defer perf.Track(nil, "flags.PagerSelector.IsProvided")()

	return p.provided
}

// Value returns the raw flag value.
func (p PagerSelector) Value() string {
	defer perf.Track(nil, "flags.PagerSelector.Value")()

	return p.value
}
