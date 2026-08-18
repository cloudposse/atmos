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
// shows a multi-select picker (reusing the already-tested PromptForMultipleValues, which handles
// TTY/CI gating, ESC/ctrl+c cancellation, and the empty-options case), and returns the final list
// of profile names to activate.
//
// Preselected holds any profile names the user already typed explicitly alongside the bare flag
// (e.g. `--profile foo --profile`). PromptForMultipleValues pre-checks every option it's given by
// default, which is the right default for "pick which of the discovered profiles to activate" --
// but it has no notion of a name that must always survive regardless of what the user does in the
// picker. Rather than forking PromptForMultipleValues to add partial pre-selection (which would
// duplicate huh form boilerplate that's already tested), preselected is unioned into the result
// after the picker returns: any name the user explicitly typed on the command line is guaranteed
// to be in the final list, in addition to whatever they interactively picked.
func selectProfilesInteractively(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error) {
	defer perf.Track(atmosConfig, "flags.selectProfilesInteractively")()

	discovered, err := profile.NewProfileManager().ListProfiles(atmosConfig)
	if err != nil {
		// ListProfiles already returns a well-formed, hint-bearing errUtils error
		// (see pkg/profile/manager.go) -- propagate it unchanged.
		return nil, err
	}

	// Use plain profile names as both the huh option label and value (no decoration, e.g. with
	// location type) so the values PromptForMultipleValues returns are exactly the names
	// loadProfiles expects -- no separate label-to-name mapping to keep in sync.
	options := make([]string, len(discovered))
	for i, p := range discovered {
		options[i] = p.Name
	}
	sort.Strings(options)

	selected, err := PromptForMultipleValues("profile", "Choose configuration profiles to activate", options)
	if err != nil {
		// Propagate as-is: errUtils.ErrInteractiveModeNotAvailable (no TTY/CI),
		// errUtils.ErrNoOptionsAvailable (no profiles discovered), errUtils.ErrUserAborted
		// (ctrl+c/esc), or a form error -- PromptForMultipleValues already produces
		// well-formed, user-facing errors for each case.
		return nil, err
	}

	return mergeProfileSelections(preselected, selected), nil
}

// mergeProfileSelections returns the union of preselected and selected, preserving order:
// preselected names first (the user typed these explicitly on the command line, so they always
// survive), followed by any additional interactively selected names not already present.
// Duplicates are removed.
func mergeProfileSelections(preselected, selected []string) []string {
	defer perf.Track(nil, "flags.mergeProfileSelections")()

	seen := make(map[string]bool, len(preselected)+len(selected))
	result := make([]string, 0, len(preselected)+len(selected))

	for _, p := range preselected {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	for _, s := range selected {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}
