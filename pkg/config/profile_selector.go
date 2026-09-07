package config

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProfileSelector prompts the user to interactively pick which configuration
// profiles to activate, given the profiles already discovered for atmosConfig
// and any names the user already provided explicitly alongside the bare
// --profile flag (e.g. `--profile foo --profile`). It returns the final list
// of profile names to activate.
//
// This package cannot depend on the concrete interactive-picker implementation
// (pkg/flags, which itself imports pkg/profile) without creating an import
// cycle: pkg/flags already imports pkg/config, and pkg/profile already
// imports pkg/config. This function type is the seam that lets pkg/flags
// inject its concrete implementation via SetProfileSelector without
// pkg/config ever importing pkg/flags or pkg/profile directly.
type ProfileSelector func(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error)

// profileSelector holds the process-wide interactive profile picker
// implementation, if one has been registered. It is nil until a caller
// (normally pkg/flags's package init(), which every cmd/* package imports
// transitively) registers a concrete implementation via SetProfileSelector.
// A nil profileSelector means interactive profile selection cannot be
// performed in this process; see ErrProfileSelectionUnavailable.
var profileSelector ProfileSelector

// SetProfileSelector registers the concrete interactive profile-selection
// implementation. Called from pkg/flags's package init() so it runs before
// any command's RunE calls LoadConfig. Tests may call this directly to
// inject a fake selector; save and restore the previous value (typically nil)
// to avoid leaking state between tests.
func SetProfileSelector(fn ProfileSelector) {
	profileSelector = fn
}
