package global

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// IdentitySelector represents the state of the identity flag.
// The identity flag has three states:
// 1. Not provided (use default from config/env)
// 2. Provided without value (--identity) → interactive selection
// 3. Provided with value (--identity=name) → use specific identity
//
// This type encapsulates the complex NoOptDefVal semantics of the identity flag.
type IdentitySelector struct {
	value    string
	provided bool
}

// NewIdentitySelector creates an IdentitySelector from flag state.
func NewIdentitySelector(value string, provided bool) IdentitySelector {
	defer perf.Track(nil, "flags.NewIdentitySelector")()

	return IdentitySelector{
		value:    value,
		provided: provided,
	}
}

// IsInteractiveSelector returns true if --identity was used without a value.
// This triggers interactive identity selection.
func (i IdentitySelector) IsInteractiveSelector() bool {
	defer perf.Track(nil, "flags.IdentitySelector.IsInteractiveSelector")()

	return i.provided && i.value == cfg.IdentityFlagSelectValue // "__SELECT__"
}

// Value returns the identity name.
// Returns empty string if not provided or if interactive selection.
func (i IdentitySelector) Value() string {
	defer perf.Track(nil, "flags.IdentitySelector.Value")()

	return i.value
}

// IsEmpty returns true if no identity was provided.
func (i IdentitySelector) IsEmpty() bool {
	defer perf.Track(nil, "flags.IdentitySelector.IsEmpty")()

	return !i.provided || i.value == ""
}

// IsProvided returns true if the flag was explicitly set.
func (i IdentitySelector) IsProvided() bool {
	defer perf.Track(nil, "flags.IdentitySelector.IsProvided")()

	return i.provided
}

// IsDisabled returns true if authentication was explicitly disabled.
// This happens when --identity=false, --identity=0, --identity=no, or --identity=off
// is used, or when ATMOS_IDENTITY is set to one of these values.
// The value is normalized to the disabled sentinel value by parseIdentityFlag.
func (i IdentitySelector) IsDisabled() bool {
	defer perf.Track(nil, "flags.IdentitySelector.IsDisabled")()

	return i.provided && i.value == cfg.IdentityFlagDisabledValue
}
