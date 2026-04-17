package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/reexec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// Env var that prevents infinite re-exec loops when an auto-selected
	// profile also fails to resolve the identity.
	profileFallbackGuardEnv = "ATMOS_PROFILE_FALLBACK"

	// Value set on the guard env var during re-exec.
	profileFallbackGuardValue = "1"

	// CLI flag Atmos uses to select a profile.
	profileFlagName = "--profile"

	// Help text shared by every fallback prompt.
	profileFallbackCancelHint = "Press ctrl+c or esc to cancel"
)

// newProfileFallbackKeyMap builds the shared huh keymap used by every profile
// fallback prompt so Ctrl+C / Esc cleanly abort the form.
func newProfileFallbackKeyMap() *huh.KeyMap {
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "quit"),
	)
	return keyMap
}

// Process replacement goes through pkg/reexec.Exec, which has
// platform-specific defaults (syscall.Exec on Unix, a spawn-wait-exit shim
// on Windows). Tests swap reexec.Exec to avoid actually replacing the test
// process.

// buildFallbackAtmosConfig returns a minimal AtmosConfiguration scoped to the
// manager's loaded atmos.yaml so the config-layer profile helpers can discover
// profiles consistently. The profiles.base_path value is read from the global
// Viper (where the loaded base config lives).
func (m *manager) buildFallbackAtmosConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		CliConfigPath: m.cliConfigPath,
		Profiles: schema.ProfilesConfig{
			BasePath: viper.GetString("profiles.base_path"),
		},
	}
}

// maybeOfferProfileFallback checks whether the missing identity is defined in
// an available Atmos profile, and — when conditions align — prompts the user
// to select one and re-executes Atmos with `--profile <picked>` prepended.
//
// The function returns:
//   - nil when no fallback was triggered (caller should continue with the
//     normal ErrIdentityNotFound path).
//   - an error that already contains hints for the user when the situation is
//     recoverable only in an interactive session (caller should surface it).
//   - never returns on a successful re-exec (the current process is replaced).
//
// The four gating conditions (see PRD: interactive-profile-suggestion):
//  1. The caller already established that the identity is unresolved.
//  2. No profile is explicitly active (no --profile flag, no ATMOS_PROFILE).
//  3. At least one profile defines the requested identity.
//  4. Either (a) the terminal is interactive → prompt + re-exec, or (b) the
//     terminal is non-interactive → enrich the error with profile hints.
//
// The loop guard (ATMOS_PROFILE_FALLBACK=1) is checked first — if set, we
// skip the fallback entirely and let the original error surface so users
// don't get trapped in an endless prompt cycle.
func (m *manager) maybeOfferProfileFallback(ctx context.Context, identityName string) error {
	defer perf.Track(nil, "auth.Manager.maybeOfferProfileFallback")()

	// Loop guard — if we've already re-exec'd once for this identity and still
	// don't find it, surface the original error instead of prompting again.
	if os.Getenv(profileFallbackGuardEnv) == profileFallbackGuardValue { //nolint:forbidigo // Loop-guard sentinel, not user-facing config.
		log.Debug("profile fallback loop guard active, skipping", logKeyIdentity, identityName)
		return nil
	}

	// An explicit --profile or ATMOS_PROFILE means the user has committed to a
	// profile choice; never suggest a different one (PRD scenario 6).
	if cfg.HasExplicitProfile() {
		log.Debug("profile explicitly active, skipping fallback", logKeyIdentity, identityName)
		return nil
	}

	candidates, err := cfg.ProfilesWithIdentity(m.buildFallbackAtmosConfig(), identityName)
	if err != nil {
		log.Debug("failed to search profiles for identity", logKeyIdentity, identityName, "error", err)
		return nil
	}
	if len(candidates) == 0 {
		// Identity is not defined in any profile — nothing to suggest.
		return nil
	}

	// Non-interactive: we cannot prompt, but we can enrich the error with a
	// concrete command the user can run (PRD scenarios 4 and 9).
	if !isInteractive() {
		return buildProfileSuggestionError(identityName, candidates)
	}

	// Interactive: prompt the user to pick a profile and re-exec with it.
	picked, promptErr := m.promptForProfileSelection(identityName, candidates)
	if promptErr != nil {
		if errors.Is(promptErr, errUtils.ErrUserAborted) {
			// User explicitly cancelled the profile prompt — treat the whole
			// operation as aborted rather than surfacing the confusing
			// "identity not found" error they were just shown a fix for.
			log.Debug("user aborted profile selection", logKeyIdentity, identityName)
			return errUtils.ErrUserAborted
		}
		return promptErr
	}

	// Re-exec never returns on success; if it does, something went wrong.
	if err := reExecWithProfile(picked); err != nil {
		return fmt.Errorf("failed to re-exec with profile %q: %w", picked, err)
	}
	// Unreachable on successful exec.
	_ = ctx
	return nil
}

// buildProfileSuggestionError wraps ErrIdentityNotFound with actionable hints
// naming the profiles that define the missing identity. Used on the
// non-interactive path where we cannot prompt.
func buildProfileSuggestionError(identityName string, candidates []string) error {
	b := errUtils.Build(errUtils.ErrIdentityNotFound).
		WithExplanationf("Identity `%s` is not defined in the currently loaded configuration.", identityName)

	if len(candidates) == 1 {
		b = b.
			WithHintf("Identity `%s` is defined in profile `%s`", identityName, candidates[0]).
			WithHintf("Re-run with `%s %s` to use it", profileFlagName, candidates[0])
	} else {
		sorted := make([]string, len(candidates))
		copy(sorted, candidates)
		sort.Strings(sorted)
		b = b.
			WithHintf("Identity `%s` is defined in these profiles: `%s`", identityName, joinQuoted(sorted)).
			WithHint("Re-run with `--profile <name>` using one of the profiles above")
	}

	return b.
		WithContext(identityNameKey, identityName).
		WithContext("candidate_profiles", fmt.Sprintf("%v", candidates)).
		WithExitCode(1).
		Err()
}

// joinQuoted joins a list of profile names with commas, wrapping each in
// backticks for terminal rendering consistency.
func joinQuoted(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, fmt.Sprintf("`%s`", n))
	}
	return strings.Join(quoted, ", ")
}

// promptForProfileSelection shows the user an interactive selector listing the
// profiles that define the requested identity and returns the picked name.
func (m *manager) promptForProfileSelection(identityName string, profiles []string) (string, error) {
	if len(profiles) == 0 {
		return "", errUtils.ErrNoIdentitiesAvailable
	}

	sorted := make([]string, len(profiles))
	copy(sorted, profiles)
	sort.Strings(sorted)

	// Fast path: exactly one candidate — ask for yes/no confirmation rather
	// than forcing a single-item list.
	if len(sorted) == 1 {
		return m.confirmSingleProfileSelection(identityName, sorted[0])
	}

	var selected string

	keyMap := newProfileFallbackKeyMap()

	title := ui.FormatInline(fmt.Sprintf("Select a profile for identity `%s`:", identityName))
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Description(profileFallbackCancelHint).
				Options(huh.NewOptions(sorted...)...).
				Value(&selected),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrUnsupportedInputType, err)
	}

	return selected, nil
}

// confirmSingleProfileSelection asks the user to confirm switching to the one
// profile that defines the identity. Rejection is treated as abort.
func (m *manager) confirmSingleProfileSelection(identityName, profile string) (string, error) {
	var confirmed bool

	keyMap := newProfileFallbackKeyMap()

	title := ui.FormatInline(fmt.Sprintf("Switch to profile `%s` for identity `%s`?", profile, identityName))
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(profileFallbackCancelHint).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrUnsupportedInputType, err)
	}

	if !confirmed {
		return "", errUtils.ErrUserAborted
	}
	return profile, nil
}

// maybeOfferAnyProfileFallback is the identity-agnostic sibling of
// maybeOfferProfileFallback. It fires when the caller hit a "no identities
// available" / "no providers available" terminal error from an auth command
// (login, exec, shell, env, console, whoami) because the base atmos.yaml has
// no usable auth config, AND no profile is explicitly active, AND at least
// one profile defines auth config.
//
// The function returns:
//   - nil when no fallback was triggered (caller surfaces the original error).
//   - an enriched error with hints naming candidate profiles when the terminal
//     is non-interactive.
//   - never returns on a successful re-exec (the current process is replaced).
func (m *manager) maybeOfferAnyProfileFallback(ctx context.Context) error {
	defer perf.Track(nil, "auth.Manager.maybeOfferAnyProfileFallback")()

	// Loop guard — a previous re-exec already picked a profile; surface the
	// caller's error instead of prompting again.
	if os.Getenv(profileFallbackGuardEnv) == profileFallbackGuardValue { //nolint:forbidigo // Loop-guard sentinel, not user-facing config.
		log.Debug("profile fallback loop guard active, skipping generic fallback")
		return nil
	}

	// An explicit --profile or ATMOS_PROFILE means the user has committed to a
	// profile choice; never suggest a different one.
	if cfg.HasExplicitProfile() {
		log.Debug("profile explicitly active, skipping generic fallback")
		return nil
	}

	candidates, err := cfg.ProfilesWithAuthConfig(m.buildFallbackAtmosConfig())
	if err != nil {
		log.Debug("failed to search profiles for auth config", "error", err)
		return nil
	}
	if len(candidates) == 0 {
		// No profile defines auth config — nothing to suggest.
		return nil
	}

	// Non-interactive: enrich the caller's error with a concrete command.
	if !isInteractive() {
		return buildAnyProfileSuggestionError(candidates)
	}

	// Interactive: prompt to pick a profile and re-exec with it.
	picked, promptErr := m.promptForAnyProfileSelection(candidates)
	if promptErr != nil {
		if errors.Is(promptErr, errUtils.ErrUserAborted) {
			// User explicitly cancelled — treat as a clean abort instead of
			// falling through to the "no identities available" error.
			log.Debug("user aborted generic profile selection")
			return errUtils.ErrUserAborted
		}
		return promptErr
	}

	if err := reExecWithProfile(picked); err != nil {
		return fmt.Errorf("failed to re-exec with profile %q: %w", picked, err)
	}
	// Unreachable on successful exec.
	_ = ctx
	return nil
}

// MaybeOfferAnyProfileFallback is the exported interface-surface wrapper around
// maybeOfferAnyProfileFallback. See the unexported method for semantics.
func (m *manager) MaybeOfferAnyProfileFallback(ctx context.Context) error {
	return m.maybeOfferAnyProfileFallback(ctx)
}

// buildAnyProfileSuggestionError wraps ErrNoIdentitiesAvailable with actionable
// hints naming the profiles that define auth config. Used on the non-interactive
// path where we cannot prompt.
func buildAnyProfileSuggestionError(candidates []string) error {
	b := errUtils.Build(errUtils.ErrNoIdentitiesAvailable).
		WithExplanation("No identities or providers are defined in the currently loaded configuration.")

	sorted := make([]string, len(candidates))
	copy(sorted, candidates)
	sort.Strings(sorted)

	if len(sorted) == 1 {
		b = b.
			WithHintf("Profile `%s` defines auth configuration", sorted[0]).
			WithHintf("Re-run with `%s %s` to use it", profileFlagName, sorted[0])
	} else {
		b = b.
			WithHintf("These profiles define auth configuration: %s", joinQuoted(sorted)).
			WithHint("Re-run with `--profile <name>` using one of the profiles above")
	}

	return b.
		WithContext("candidate_profiles", fmt.Sprintf("%v", sorted)).
		WithExitCode(1).
		Err()
}

// promptForAnyProfileSelection shows a selector listing every profile that
// defines auth config and returns the picked name.
func (m *manager) promptForAnyProfileSelection(profiles []string) (string, error) {
	if len(profiles) == 0 {
		return "", errUtils.ErrNoIdentitiesAvailable
	}

	sorted := make([]string, len(profiles))
	copy(sorted, profiles)
	sort.Strings(sorted)

	if len(sorted) == 1 {
		return m.confirmSingleAnyProfileSelection(sorted[0])
	}

	var selected string

	keyMap := newProfileFallbackKeyMap()

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(ui.FormatInline("Select a profile with auth config:")).
				Description(profileFallbackCancelHint).
				Options(huh.NewOptions(sorted...)...).
				Value(&selected),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrUnsupportedInputType, err)
	}

	return selected, nil
}

// confirmSingleAnyProfileSelection asks the user to confirm switching to the
// one profile that defines auth config. Rejection is treated as abort.
func (m *manager) confirmSingleAnyProfileSelection(profile string) (string, error) {
	var confirmed bool

	keyMap := newProfileFallbackKeyMap()

	title := ui.FormatInline(fmt.Sprintf("Switch to profile `%s`?", profile))
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(profileFallbackCancelHint).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errUtils.ErrUserAborted
		}
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrUnsupportedInputType, err)
	}

	if !confirmed {
		return "", errUtils.ErrUserAborted
	}
	return profile, nil
}

// reExecWithProfile re-runs Atmos with `--profile <name>` inserted at the
// front of the argument list (after argv[0]) and the loop-guard env var set.
// On success, this function does NOT return — the current process is replaced
// (Unix) or exits with the child's status (Windows, via Go's syscall shim).
func reExecWithProfile(profileName string) error {
	defer perf.Track(nil, "auth.reExecWithProfile")()

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate atmos binary: %w", err)
	}

	// Build new argv: [atmos, --profile, <name>, <original args minus --chdir>].
	// os.Args[0] is the program name that the re-exec'd process will see.
	// --chdir / -C was already applied to this process by processEarlyChdirFlag;
	// stripping it from the child's argv prevents a relative chdir from being
	// re-applied against the already-changed cwd.
	origArgs := os.Args
	newArgs := make([]string, 0, len(origArgs)+2)
	newArgs = append(newArgs, origArgs[0])
	newArgs = append(newArgs, profileFlagName, profileName)
	if len(origArgs) > 1 {
		newArgs = append(newArgs, reexec.StripChdirArgs(origArgs[1:])...)
	}

	// Propagate environment + loop guard. ATMOS_CHDIR is filtered for the
	// same reason --chdir is stripped from argv.
	env := reexec.FilterChdirEnv(os.Environ())
	env = append(env, profileFallbackGuardEnv+"="+profileFallbackGuardValue)

	log.Debug("re-executing atmos with selected profile",
		"profile", profileName,
		"exe", exe,
		"argv", newArgs)

	return reexec.Exec(exe, newArgs, env)
}
