package flags

import (
	"sort"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/profile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// init registers selectProfilesInteractively as the process-wide profile selector.
// The pkg/config package cannot import pkg/flags (or pkg/profile) directly -- pkg/flags already
// imports pkg/config, and pkg/profile already imports pkg/config -- so this file is the
// dependency-injection seam: pkg/config exposes a function-typed hook (cfg.ProfileSelector /
// cfg.SetProfileSelector) and pkg/flags, which every cmd/* package imports transitively, fills
// it in here. By the time any command's RunE calls cfg.LoadConfig, this init() has already run.
func init() {
	cfg.SetProfileSelector(selectProfilesInteractively)
}

// selectProfilesInteractively is the concrete cfg.ProfileSelector implementation for the bare
// `--profile` interactive-selection sentinel. It discovers available profiles for atmosConfig,
// shows a multi-select picker (reusing the already-tested PromptForMultipleValuesWithPreselection,
// which handles TTY/CI gating, ESC/ctrl+c cancellation, and the empty-options case), and returns
// the final list of profile names to activate.
//
// Preselected holds any profile names the user already typed explicitly alongside the bare flag
// (e.g. `--profile foo --profile`) -- these start pre-checked in the picker since the user asked
// for them. Every other discovered profile starts unchecked: defaulting to "activate everything"
// would be surprising for a flag whose whole purpose is picking a subset, so a bare `--profile`
// with no explicit names opens with nothing selected.
func selectProfilesInteractively(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error) {
	defer perf.Track(atmosConfig, "flags.selectProfilesInteractively")()

	discovered, err := profile.NewProfileManager().ListProfiles(atmosConfig)
	if err != nil {
		// ListProfiles already returns a well-formed, hint-bearing errUtils error
		// (see pkg/profile/manager.go) -- propagate it unchanged.
		return nil, err
	}

	// Use plain profile names as both the huh option label and value (no decoration, e.g. with
	// location type) so the values PromptForMultipleValuesWithPreselection returns are exactly the
	// names loadProfiles expects -- no separate label-to-name mapping to keep in sync.
	options := make([]string, len(discovered))
	for i, p := range discovered {
		options[i] = p.Name
	}
	sort.Strings(options)

	selected, err := PromptForMultipleValuesWithPreselection("profile", "Choose configuration profiles to activate", options, preselected)
	if err != nil {
		// Propagate as-is: errUtils.ErrInteractiveModeNotAvailable (no TTY/CI),
		// errUtils.ErrNoOptionsAvailable (no profiles discovered), errUtils.ErrUserAborted
		// (ctrl+c/esc), or a form error -- PromptForMultipleValuesWithPreselection already produces
		// well-formed, user-facing errors for each case.
		return nil, err
	}

	return selected, nil
}
