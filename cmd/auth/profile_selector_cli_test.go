package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestBareProfileFlag_NonTTY_CLI is a CLI-level regression test proving the full, real
// interactive-profile-selection wiring end to end: this package (cmd/auth) imports pkg/flags
// transitively (see auth.go, login.go, etc.), so pkg/flags's package init() has already
// registered the real selectProfilesInteractively as cfg's ProfileSelector by the time this test
// runs -- no fake selector is injected here.
//
// This proves three things about `atmos auth login --profile` (or any command) run bare in a
// non-TTY/CI context, now that Phase A (pkg/flags NoOptDefVal support for StringSliceFlag) and
// Phase B (this interactive-selection wiring) are both in place:
//
//  1. It no longer produces the old raw pflag parse error ("flag needs an argument for command
//     ..."), which Phase A already fixed at the flag-parsing layer.
//  2. It no longer produces the confusing error that would occur without Phase B: Atmos silently
//     trying (and failing) to load a profile literally named "__SELECT__" ("profile '__SELECT__'
//     not found").
//  3. It instead surfaces a clear, well-formed errUtils error from the real interactive-picker
//     code path. In a genuinely non-interactive context (as here, and as any CI run would be),
//     that error is errUtils.ErrInteractiveModeNotAvailable -- surfaced by
//     flags.PromptForMultipleValues, which pkg/flags's real selectProfilesInteractively delegates
//     to. (A nil selector -- e.g. a caller that imports pkg/config without ever importing
//     pkg/flags -- would instead see errUtils.ErrProfileSelectionUnavailable; see
//     pkg/config/profile_selector_test.go and pkg/flags/profile_selector_test.go for that case.)
func TestBareProfileFlag_NonTTY_CLI(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("interactive", false) // Force the non-interactive gate deterministically, matching CI.

	// Isolate: own CWD/atmos.yaml/XDG/git-root so this test never touches the real repository's
	// atmos.yaml, profiles/ directory, or the developer machine's $HOME/.config/atmos/profiles.
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("base_path: .\n"), 0o644))

	// Simulates what Cobra flag parsing hands LoadConfig for a bare `--profile` invocation
	// (e.g. `atmos auth login --profile`) after Phase A's NoOptDefVal preprocessing resolves the
	// bare flag to the sentinel -- this is the exact value flowing into
	// configAndStacksInfo.ProfilesFromArg in production, whichever of the several call sites
	// built it (internal/exec.ProcessCommandLineArgs, describe_component.go, or the
	// getProfilesFromFlagsOrEnv fallback).
	_, err := cfg.LoadConfig(&schema.ConfigAndStacksInfo{
		ProfilesFromArg: []string{cfg.ProfileFlagSelectValue},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable,
		"real (non-fake) selectProfilesInteractively wiring should surface the non-interactive gate error")

	msg := err.Error()
	assert.NotContains(t, msg, "flag needs an argument",
		"must not resurface the old raw pflag parse error Phase A already fixed")
	assert.NotContains(t, msg, "'__SELECT__' not found",
		"must not surface a confusing 'profile not found' error naming the internal sentinel")
	assert.NotContains(t, msg, `"__SELECT__" not found`,
		"must not surface a confusing 'profile not found' error naming the internal sentinel")
}
