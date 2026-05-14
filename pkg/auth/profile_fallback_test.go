package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/huh"
	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/reexec"
)

// stubRunForm replaces runForm for a single test with a function that returns
// the given error. Auto-restores on cleanup.
func stubRunForm(t *testing.T, returnErr error) {
	t.Helper()
	original := runForm
	runForm = func(_ *huh.Form) error { return returnErr }
	t.Cleanup(func() { runForm = original })
}

// stubRunFormWithAction replaces runForm with an arbitrary function, typically
// used to mutate the bound value before returning nil so the caller observes a
// "user made a selection" outcome.
func stubRunFormWithAction(t *testing.T, action func(*huh.Form) error) {
	t.Helper()
	original := runForm
	runForm = action
	t.Cleanup(func() { runForm = original })
}

// hintsContain reports whether any hint in err contains the given substring.
func hintsContain(err error, substr string) bool {
	for _, h := range cockroachErrors.GetAllHints(err) {
		if strings.Contains(h, substr) {
			return true
		}
	}
	return false
}

// profileFallbackFixture creates two profiles on disk. The "alpha" profile
// defines identity "root-admin"; the "beta" profile defines "dev-user".
// Returns the CliConfigPath (temp dir root).
func profileFallbackFixture(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	alphaDir := filepath.Join(tmpDir, "profiles", "alpha")
	betaDir := filepath.Join(tmpDir, "profiles", "beta")
	require.NoError(t, os.MkdirAll(alphaDir, 0o755))
	require.NoError(t, os.MkdirAll(betaDir, 0o755))

	alphaYAML := `auth:
  identities:
    root-admin:
      kind: aws/user
`
	betaYAML := `auth:
  identities:
    dev-user:
      kind: aws/user
`
	require.NoError(t, os.WriteFile(filepath.Join(alphaDir, "atmos.yaml"), []byte(alphaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(betaDir, "atmos.yaml"), []byte(betaYAML), 0o644))

	return tmpDir
}

// newFallbackManager constructs a minimal manager suitable for exercising
// maybeOfferProfileFallback. We bypass the normal factory-driven bootstrap
// because this test doesn't authenticate — it only inspects the fallback's
// gating logic.
func newFallbackManager(cliConfigPath string) *manager {
	return &manager{
		cliConfigPath: cliConfigPath,
	}
}

// resetGlobalProfileState clears state that influences HasExplicitProfile.
// Both --profile (via os.Args) and ATMOS_PROFILE (env) are checked.
func resetGlobalProfileState(t *testing.T) {
	t.Helper()

	origArgs := os.Args
	os.Args = []string{"atmos", "terraform", "plan"}
	t.Cleanup(func() { os.Args = origArgs })

	t.Setenv("ATMOS_PROFILE", "")
	t.Setenv(reexec.DepthEnvVar, "")
	viper.Reset()
	t.Cleanup(viper.Reset)

	// profiles.base_path is read from the global viper by the fallback so it
	// can locate profiles relative to CliConfigPath.
	viper.Set("profiles.base_path", "profiles")
}

// Scenario 10: loop guard — when ATMOS_REEXEC_DEPTH > 0 the fallback must
// skip everything and return nil so the caller surfaces ErrIdentityNotFound.
func TestMaybeOfferProfileFallback_LoopGuardSkips(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := profileFallbackFixture(t)

	t.Setenv(reexec.DepthEnvVar, "1")

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "root-admin")
	assert.NoError(t, err, "loop guard must short-circuit without error")
}

// Scenario 6/7: explicit --profile via os.Args suppresses the fallback even
// when another profile defines the identity.
func TestMaybeOfferProfileFallback_ExplicitFlagSkips(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := profileFallbackFixture(t)

	os.Args = []string{"atmos", "--profile", "alpha", "terraform", "plan"}

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "dev-user")
	assert.NoError(t, err, "explicit --profile must suppress the fallback")
}

// Scenario 6/7 (env variant): ATMOS_PROFILE suppresses the fallback.
func TestMaybeOfferProfileFallback_ExplicitEnvSkips(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := profileFallbackFixture(t)

	t.Setenv("ATMOS_PROFILE", "alpha")

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "dev-user")
	assert.NoError(t, err, "ATMOS_PROFILE must suppress the fallback")
}

// Scenario 1/5: identity is not defined in any profile — the fallback returns
// nil so the caller surfaces the original ErrIdentityNotFound unchanged.
func TestMaybeOfferProfileFallback_NoCandidatesReturnsNil(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := profileFallbackFixture(t)

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "nonexistent")
	assert.NoError(t, err, "no candidate profile → no fallback error")
}

// Scenario 4/9: non-interactive terminal + candidate profile exists → the
// fallback returns an enriched ErrIdentityNotFound with hints naming the
// profile.
func TestMaybeOfferProfileFallback_NonInteractiveEnrichesError(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := profileFallbackFixture(t)

	// isInteractive() returns false without --interactive — this is the
	// non-interactive path.
	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "root-admin")
	require.Error(t, err, "non-interactive must return an enriched error")
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)

	assert.True(t, hintsContain(err, "alpha"),
		"enriched error hints must mention the profile that defines the identity")
	assert.True(t, hintsContain(err, "root-admin"),
		"enriched error hints must mention the requested identity")
}

// buildProfileSuggestionError with one candidate produces a single-profile hint.
func TestBuildProfileSuggestionError_SingleCandidate(t *testing.T) {
	err := buildProfileSuggestionError("root-admin", []string{"alpha"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)

	assert.True(t, hintsContain(err, "alpha"), "hint should name the profile")
	assert.True(t, hintsContain(err, "--profile alpha"),
		"hint should show the exact re-run command")
	assert.True(t, hintsContain(err, "root-admin"), "hint should name the identity")
}

// buildProfileSuggestionError with multiple candidates lists them all.
func TestBuildProfileSuggestionError_MultipleCandidates(t *testing.T) {
	err := buildProfileSuggestionError("shared-id", []string{"charlie", "alpha", "bravo"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotFound)

	// All three profile names must appear somewhere in the hints.
	assert.True(t, hintsContain(err, "alpha"))
	assert.True(t, hintsContain(err, "bravo"))
	assert.True(t, hintsContain(err, "charlie"))
	assert.True(t, hintsContain(err, "shared-id"))
}

// joinQuoted wraps each name in backticks and joins with ", ".
func TestJoinQuoted(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{"empty", nil, ""},
		{"single", []string{"alpha"}, "`alpha`"},
		{"two", []string{"alpha", "beta"}, "`alpha`, `beta`"},
		{"three", []string{"a", "b", "c"}, "`a`, `b`, `c`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinQuoted(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

// reExecWithProfile builds the correct argv and passes it to reexec.Exec.
// We swap reexec.Exec with a mock so the test doesn't actually replace the
// process.
func TestReExecWithProfile_BuildsArgvAndEnv(t *testing.T) {
	t.Cleanup(func() { reexec.Exec = originalExecFunc })

	// Capture what reexec.Exec was called with.
	var gotArgv0 string
	var gotArgs []string
	var gotEnv []string

	reexec.Exec = func(argv0 string, argv []string, envv []string) error {
		gotArgv0 = argv0
		gotArgs = argv
		gotEnv = envv
		// Return a sentinel so reExecWithProfile returns instead of replacing the process.
		return errExecMockCalled
	}

	origArgs := os.Args
	os.Args = []string{"atmos", "terraform", "plan", "--stack", "dev"}
	t.Cleanup(func() { os.Args = origArgs })

	err := reExecWithProfile("developer")
	require.ErrorIs(t, err, errExecMockCalled)

	assert.NotEmpty(t, gotArgv0, "argv0 (binary path) must be populated")
	// New argv: [atmos, --profile, developer, terraform, plan, --stack, dev].
	require.Len(t, gotArgs, 7)
	assert.Equal(t, "atmos", gotArgs[0])
	assert.Equal(t, "--profile", gotArgs[1])
	assert.Equal(t, "developer", gotArgs[2])
	assert.Equal(t, "terraform", gotArgs[3])
	assert.Equal(t, "plan", gotArgs[4])
	assert.Equal(t, "--stack", gotArgs[5])
	assert.Equal(t, "dev", gotArgs[6])

	// Re-exec depth must be incremented in the env for the child process.
	foundDepth := false
	for _, e := range gotEnv {
		if e == reexec.DepthEnvVar+"=1" {
			foundDepth = true
			break
		}
	}
	assert.True(t, foundDepth, "re-exec must propagate ATMOS_REEXEC_DEPTH=1")
}

// reExecWithProfile handles the edge case where os.Args has only the binary name.
func TestReExecWithProfile_NoExtraArgs(t *testing.T) {
	t.Cleanup(func() { reexec.Exec = originalExecFunc })

	var gotArgs []string
	reexec.Exec = func(_ string, argv []string, _ []string) error {
		gotArgs = argv
		return errExecMockCalled
	}

	origArgs := os.Args
	os.Args = []string{"atmos"}
	t.Cleanup(func() { os.Args = origArgs })

	err := reExecWithProfile("prod")
	require.ErrorIs(t, err, errExecMockCalled)

	require.Len(t, gotArgs, 3)
	assert.Equal(t, []string{"atmos", "--profile", "prod"}, gotArgs)
}

// anyProfileFallbackFixture creates profiles that exercise the identity-
// agnostic fallback path: "auth-alpha" and "auth-beta" both define auth config
// (via auth.identities or auth.providers); "plain" has none. Returns the
// CliConfigPath (temp dir root).
func anyProfileFallbackFixture(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	alphaDir := filepath.Join(tmpDir, "profiles", "auth-alpha")
	betaDir := filepath.Join(tmpDir, "profiles", "auth-beta")
	plainDir := filepath.Join(tmpDir, "profiles", "plain")
	require.NoError(t, os.MkdirAll(alphaDir, 0o755))
	require.NoError(t, os.MkdirAll(betaDir, 0o755))
	require.NoError(t, os.MkdirAll(plainDir, 0o755))

	alphaYAML := `auth:
  identities:
    root-admin:
      kind: aws/user
`
	// Providers-only profile — qualifies as a candidate even without identities.
	betaYAML := `auth:
  providers:
    my-sso:
      kind: aws/sso
`
	plainYAML := `stacks:
  base_path: stacks
`
	require.NoError(t, os.WriteFile(filepath.Join(alphaDir, "atmos.yaml"), []byte(alphaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(betaDir, "atmos.yaml"), []byte(betaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plainDir, "atmos.yaml"), []byte(plainYAML), 0o644))

	return tmpDir
}

// Loop guard — ATMOS_REEXEC_DEPTH > 0 short-circuits the generic fallback.
func TestMaybeOfferAnyProfileFallback_LoopGuardSkips(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := anyProfileFallbackFixture(t)

	t.Setenv(reexec.DepthEnvVar, "1")

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	assert.NoError(t, err, "loop guard must short-circuit without error")
}

// Explicit --profile (via os.Args) suppresses the generic fallback.
func TestMaybeOfferAnyProfileFallback_ExplicitFlagSkips(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := anyProfileFallbackFixture(t)

	os.Args = []string{"atmos", "--profile", "auth-alpha", "auth", "login"}

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	assert.NoError(t, err, "explicit --profile must suppress the generic fallback")
}

// ATMOS_PROFILE suppresses the generic fallback.
func TestMaybeOfferAnyProfileFallback_ExplicitEnvSkips(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := anyProfileFallbackFixture(t)

	t.Setenv("ATMOS_PROFILE", "auth-alpha")

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	assert.NoError(t, err, "ATMOS_PROFILE must suppress the generic fallback")
}

// No profiles with auth config → fallback returns nil so caller surfaces
// the original "no identities/providers" error unchanged.
func TestMaybeOfferAnyProfileFallback_NoCandidatesReturnsNil(t *testing.T) {
	resetGlobalProfileState(t)

	// Fixture with no profiles at all.
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "profiles"), 0o755))

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	assert.NoError(t, err, "no candidates → nil so original error surfaces")
}

// Non-interactive + candidate profiles → enriched ErrNoIdentitiesAvailable
// with hints naming each candidate.
func TestMaybeOfferAnyProfileFallback_NonInteractiveEnrichesError(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := anyProfileFallbackFixture(t)

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	require.Error(t, err, "non-interactive must return an enriched error")
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesAvailable)

	// Both auth-bearing profiles must appear in the hints.
	assert.True(t, hintsContain(err, "auth-alpha"),
		"hint should mention the identities-defining profile")
	assert.True(t, hintsContain(err, "auth-beta"),
		"hint should mention the providers-defining profile")
	// "plain" profile has no auth config — must NOT appear.
	assert.False(t, hintsContain(err, "plain"),
		"profiles without auth config must not appear in hints")
}

// buildAnyProfileSuggestionError with one candidate produces a single-profile hint.
func TestBuildAnyProfileSuggestionError_SingleCandidate(t *testing.T) {
	err := buildAnyProfileSuggestionError([]string{"solo"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesAvailable)

	assert.True(t, hintsContain(err, "solo"), "hint should name the profile")
	assert.True(t, hintsContain(err, "--profile solo"),
		"hint should show the exact re-run command")
}

// buildAnyProfileSuggestionError with multiple candidates lists them all.
func TestBuildAnyProfileSuggestionError_MultipleCandidates(t *testing.T) {
	err := buildAnyProfileSuggestionError([]string{"charlie", "alpha", "bravo"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesAvailable)

	assert.True(t, hintsContain(err, "alpha"))
	assert.True(t, hintsContain(err, "bravo"))
	assert.True(t, hintsContain(err, "charlie"))
}

// Shared sentinel used by the reexec.Exec mocks to short-circuit without
// actually replacing the process.
var errExecMockCalled = errors.New("mock exec called")

// originalExecFunc preserves the production value of reexec.Exec so tests
// can swap and restore it without leaking state between test cases.
var originalExecFunc = reexec.Exec

// newProfileFallbackKeyMap must return a keymap that binds Quit to both
// ctrl+c and esc — these are the only keys the user can press to abort the
// fallback prompt and get ErrUserAborted from huh.
func TestNewProfileFallbackKeyMap(t *testing.T) {
	keyMap := newProfileFallbackKeyMap()
	require.NotNil(t, keyMap)

	quitKeys := keyMap.Quit.Keys()
	assert.Contains(t, quitKeys, "ctrl+c",
		"Quit binding must accept ctrl+c so users can abort the prompt")
	assert.Contains(t, quitKeys, "esc",
		"Quit binding must accept esc so users can abort the prompt")
}

// MaybeOfferAnyProfileFallback is the exported wrapper around the unexported
// maybeOfferAnyProfileFallback. It must satisfy the same gating rules — this
// exercises the thin delegation that would otherwise sit at 0% coverage.
func TestMaybeOfferAnyProfileFallback_Exported_NoCandidates(t *testing.T) {
	resetGlobalProfileState(t)

	// Fixture with no profiles at all.
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "profiles"), 0o755))

	m := newFallbackManager(tmpDir)
	err := m.MaybeOfferAnyProfileFallback(context.Background())
	assert.NoError(t, err,
		"exported wrapper must match internal method: no candidates → nil")
}

// Exported wrapper short-circuits on loop guard, same as the unexported impl.
func TestMaybeOfferAnyProfileFallback_Exported_LoopGuard(t *testing.T) {
	resetGlobalProfileState(t)
	tmpDir := anyProfileFallbackFixture(t)
	t.Setenv(reexec.DepthEnvVar, "1")

	m := newFallbackManager(tmpDir)
	err := m.MaybeOfferAnyProfileFallback(context.Background())
	assert.NoError(t, err,
		"exported wrapper must respect the loop guard")
}

// reExecWithProfile must strip --chdir/-C from the child argv so a relative
// chdir applied to the parent is not re-applied against the already-changed
// cwd. This is the regression guard for the chdir family of fixes documented
// in docs/fixes/.
func TestReExecWithProfile_StripsChdirFromArgv(t *testing.T) {
	t.Cleanup(func() { reexec.Exec = originalExecFunc })

	var gotArgs []string
	reexec.Exec = func(_ string, argv []string, _ []string) error {
		gotArgs = argv
		return errExecMockCalled
	}

	origArgs := os.Args
	// Parent had --chdir /tmp; it should NOT appear in the child argv.
	os.Args = []string{"atmos", "--chdir", "/tmp", "auth", "login"}
	t.Cleanup(func() { os.Args = origArgs })

	err := reExecWithProfile("dev")
	require.ErrorIs(t, err, errExecMockCalled)

	for i, a := range gotArgs {
		assert.NotEqual(t, "--chdir", a,
			"child argv must not contain --chdir (index %d, argv=%v)", i, gotArgs)
		assert.NotEqual(t, "-C", a,
			"child argv must not contain -C (index %d, argv=%v)", i, gotArgs)
	}
	// --profile <name> must still be inserted.
	require.GreaterOrEqual(t, len(gotArgs), 3)
	assert.Equal(t, "--profile", gotArgs[1])
	assert.Equal(t, "dev", gotArgs[2])
	// Downstream non-chdir args must survive.
	assert.Contains(t, gotArgs, "auth")
	assert.Contains(t, gotArgs, "login")
}

// reExecWithProfile must also filter ATMOS_CHDIR from the child env for the
// same reason: a relative chdir already applied to the parent must not be
// re-applied to the already-changed cwd in the child.
func TestReExecWithProfile_FiltersChdirFromEnv(t *testing.T) {
	t.Cleanup(func() { reexec.Exec = originalExecFunc })

	var gotEnv []string
	reexec.Exec = func(_ string, _ []string, envv []string) error {
		gotEnv = envv
		return errExecMockCalled
	}

	// Ensure ATMOS_CHDIR is present in the parent env.
	t.Setenv("ATMOS_CHDIR", "/tmp")

	origArgs := os.Args
	os.Args = []string{"atmos", "auth", "login"}
	t.Cleanup(func() { os.Args = origArgs })

	err := reExecWithProfile("dev")
	require.ErrorIs(t, err, errExecMockCalled)

	// FilterChdirEnv emits "ATMOS_CHDIR=" (empty) as an explicit override so the
	// child cannot inherit the parent's value. The non-empty "ATMOS_CHDIR=/tmp"
	// must not survive.
	sawEmptyOverride := false
	for _, e := range gotEnv {
		assert.NotEqual(t, "ATMOS_CHDIR=/tmp", e,
			"child env must not carry the parent's ATMOS_CHDIR value")
		if e == "ATMOS_CHDIR=" {
			sawEmptyOverride = true
		}
	}
	assert.True(t, sawEmptyOverride,
		"child env must include the explicit ATMOS_CHDIR= override")
}

// buildFallbackAtmosConfig must thread the manager's cliConfigPath and viper's
// profiles.base_path into the returned schema so ProfilesWith* helpers can
// resolve the profile directory layout.
func TestBuildFallbackAtmosConfig_PopulatesPaths(t *testing.T) {
	resetGlobalProfileState(t)
	viper.Set("profiles.base_path", "custom-profiles-dir")

	// Use a platform-neutral path (backslash-safe on Windows) for the
	// round-trip assertion. The value is opaque to buildFallbackAtmosConfig
	// — it only echoes the manager's cliConfigPath back — so we just need
	// any string. filepath.Join avoids hardcoding a Unix separator.
	cliPath := filepath.Join("some", "cli", "config", "path")
	m := newFallbackManager(cliPath)
	cfg := m.buildFallbackAtmosConfig()

	require.NotNil(t, cfg)
	assert.Equal(t, cliPath, cfg.CliConfigPath)
	assert.Equal(t, "custom-profiles-dir", cfg.Profiles.BasePath)
}

// Empty candidate list must short-circuit with ErrNoIdentitiesAvailable
// before any form is constructed — the prompt function has nothing to offer.
func TestPromptForProfileSelection_EmptyList(t *testing.T) {
	m := newFallbackManager("")
	_, err := m.promptForProfileSelection("root-admin", nil)
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesAvailable,
		"empty candidate list must return ErrNoIdentitiesAvailable without running any form")
}

// Exactly-one candidate takes the confirm-single fast path. We stub the form
// to simulate user pressing Yes (no error) — confirm returns the profile.
// Note: the Confirm's bound bool defaults to false, so stubRunForm with nil
// leaves `confirmed=false` and results in ErrUserAborted; to test the Yes
// path we need to mutate the bound value, which is non-trivial without
// reflection. Instead, cover the No-path (nil error → confirmed==false →
// ErrUserAborted) which also exercises the function's return path.
func TestPromptForProfileSelection_SingleCandidateConfirmedNo(t *testing.T) {
	stubRunForm(t, nil) // form.Run returns nil, bound `confirmed` stays false.

	m := newFallbackManager("")
	_, err := m.promptForProfileSelection("root-admin", []string{"alpha"})
	assert.ErrorIs(t, err, errUtils.ErrUserAborted,
		"single-candidate confirm: false bound value must be treated as abort")
}

// User hits Ctrl+C / Esc during the confirm → huh returns ErrUserAborted →
// we translate to errUtils.ErrUserAborted.
func TestPromptForProfileSelection_SingleCandidateUserAborted(t *testing.T) {
	stubRunForm(t, huh.ErrUserAborted)

	m := newFallbackManager("")
	_, err := m.promptForProfileSelection("root-admin", []string{"alpha"})
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// Any other huh error is wrapped with ErrUnsupportedInputType.
func TestPromptForProfileSelection_SingleCandidateOtherError(t *testing.T) {
	sentinel := errors.New("tty write failed")
	stubRunForm(t, sentinel)

	m := newFallbackManager("")
	_, err := m.promptForProfileSelection("root-admin", []string{"alpha"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType,
		"non-abort huh errors must be wrapped with ErrUnsupportedInputType")
}

// Multi-candidate path: user aborts → ErrUserAborted.
func TestPromptForProfileSelection_MultiCandidateUserAborted(t *testing.T) {
	stubRunForm(t, huh.ErrUserAborted)

	m := newFallbackManager("")
	_, err := m.promptForProfileSelection("root-admin", []string{"alpha", "beta"})
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// Multi-candidate path: form fails with a non-abort error → wrapped.
func TestPromptForProfileSelection_MultiCandidateOtherError(t *testing.T) {
	sentinel := errors.New("form broke")
	stubRunForm(t, sentinel)

	m := newFallbackManager("")
	_, err := m.promptForProfileSelection("root-admin", []string{"alpha", "beta"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
}

// Multi-candidate path: user made a valid selection. We stub the form to
// return nil; huh.Select defaults the bound value to the first option at
// form construction, so the returned profile will be the first sorted
// candidate. Exercises the happy-path return of the multi-candidate branch.
func TestPromptForProfileSelection_MultiCandidateSelected(t *testing.T) {
	stubRunForm(t, nil)

	m := newFallbackManager("")
	picked, err := m.promptForProfileSelection("root-admin", []string{"beta", "alpha"})
	require.NoError(t, err, "nil form error is a successful submission")
	// Candidates are sorted before being passed to huh.NewSelect, and huh
	// defaults the bound value to the first option → "alpha".
	assert.Equal(t, "alpha", picked,
		"bound value defaults to first (sorted) option; this pins that contract")
}

// confirmSingleProfileSelection: Yes returns (profile, nil). We can't easily
// mutate the bound bool from the stub, so this asserts the No-path (default
// bound value → ErrUserAborted) to cover the function's final branch.
func TestConfirmSingleProfileSelection_DefaultNoIsAbort(t *testing.T) {
	stubRunForm(t, nil)

	m := newFallbackManager("")
	_, err := m.confirmSingleProfileSelection("root-admin", "alpha")
	assert.ErrorIs(t, err, errUtils.ErrUserAborted,
		"confirm default (false) must be treated as abort")
}

// confirmSingleProfileSelection: user aborts.
func TestConfirmSingleProfileSelection_UserAborted(t *testing.T) {
	stubRunForm(t, huh.ErrUserAborted)

	m := newFallbackManager("")
	_, err := m.confirmSingleProfileSelection("root-admin", "alpha")
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// confirmSingleProfileSelection: form errors with non-abort.
func TestConfirmSingleProfileSelection_OtherError(t *testing.T) {
	stubRunForm(t, errors.New("oops"))

	m := newFallbackManager("")
	_, err := m.confirmSingleProfileSelection("root-admin", "alpha")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
}

// promptForAnyProfileSelection — same matrix as promptForProfileSelection
// but for the identity-agnostic sibling.
func TestPromptForAnyProfileSelection_EmptyList(t *testing.T) {
	m := newFallbackManager("")
	_, err := m.promptForAnyProfileSelection(nil)
	assert.ErrorIs(t, err, errUtils.ErrNoIdentitiesAvailable)
}

func TestPromptForAnyProfileSelection_SingleCandidateDelegates(t *testing.T) {
	stubRunForm(t, huh.ErrUserAborted)

	m := newFallbackManager("")
	_, err := m.promptForAnyProfileSelection([]string{"solo"})
	assert.ErrorIs(t, err, errUtils.ErrUserAborted,
		"single candidate must delegate to confirmSingleAnyProfileSelection")
}

func TestPromptForAnyProfileSelection_MultiCandidateUserAborted(t *testing.T) {
	stubRunForm(t, huh.ErrUserAborted)

	m := newFallbackManager("")
	_, err := m.promptForAnyProfileSelection([]string{"alpha", "beta"})
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

func TestPromptForAnyProfileSelection_MultiCandidateOtherError(t *testing.T) {
	stubRunForm(t, errors.New("form broke"))

	m := newFallbackManager("")
	_, err := m.promptForAnyProfileSelection([]string{"alpha", "beta"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
}

func TestPromptForAnyProfileSelection_MultiCandidateSelected(t *testing.T) {
	stubRunForm(t, nil)

	m := newFallbackManager("")
	_, err := m.promptForAnyProfileSelection([]string{"alpha", "beta"})
	require.NoError(t, err)
}

// confirmSingleAnyProfileSelection — mirrors confirmSingleProfileSelection.
func TestConfirmSingleAnyProfileSelection_DefaultNoIsAbort(t *testing.T) {
	stubRunForm(t, nil)

	m := newFallbackManager("")
	_, err := m.confirmSingleAnyProfileSelection("alpha")
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

func TestConfirmSingleAnyProfileSelection_UserAborted(t *testing.T) {
	stubRunForm(t, huh.ErrUserAborted)

	m := newFallbackManager("")
	_, err := m.confirmSingleAnyProfileSelection("alpha")
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

func TestConfirmSingleAnyProfileSelection_OtherError(t *testing.T) {
	stubRunForm(t, errors.New("oops"))

	m := newFallbackManager("")
	_, err := m.confirmSingleAnyProfileSelection("alpha")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
}

// stubInteractiveTrue forces interactiveCheck to report interactive mode
// for the duration of a single test. Auto-restores on cleanup.
//
// Tests only ever need the "force true" direction — the default
// isInteractive() already returns false in a headless test env, so
// there's no reason to take a bool parameter and trip unparam.
func stubInteractiveTrue(t *testing.T) {
	t.Helper()
	original := interactiveCheck
	interactiveCheck = func() bool { return true }
	t.Cleanup(func() { interactiveCheck = original })
}

// Interactive branch + user aborts the single-candidate confirm →
// maybeOfferProfileFallback returns ErrUserAborted (not the enriched
// identity-not-found error). Exercises the full interactive path:
// ProfilesWithIdentity → prompt → confirm → huh abort → translation.
func TestMaybeOfferProfileFallback_InteractiveUserAborted(t *testing.T) {
	resetGlobalProfileState(t)
	stubInteractiveTrue(t)
	stubRunForm(t, huh.ErrUserAborted)

	tmpDir := profileFallbackFixture(t)

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "root-admin")
	assert.ErrorIs(t, err, errUtils.ErrUserAborted,
		"interactive + user aborts → whole fallback must return ErrUserAborted")
}

// Interactive branch + prompt errors (non-abort) → error propagates
// wrapped as ErrUnsupportedInputType.
func TestMaybeOfferProfileFallback_InteractivePromptError(t *testing.T) {
	resetGlobalProfileState(t)
	stubInteractiveTrue(t)
	stubRunForm(t, errors.New("form broke"))

	tmpDir := profileFallbackFixture(t)

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "root-admin")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
}

// Interactive branch + user confirms + reexec.Exec errors →
// "failed to re-exec" returned. We swap reexec.Exec to a mock returning
// sentinel so reExecWithProfile does not replace the test process.
func TestMaybeOfferProfileFallback_InteractiveReExecFails(t *testing.T) {
	resetGlobalProfileState(t)
	stubInteractiveTrue(t)

	// Pre-mutate the bound bool via a custom stub so confirm returns "Yes".
	stubRunFormWithAction(t, func(f *huh.Form) error {
		// Drive the confirm into the affirmative path: the form has a
		// single *bool field; huh doesn't expose setters, but the field
		// will already be false. We rely on Single-candidate → confirm
		// → bound=false → ErrUserAborted path staying false, so this
		// path only reaches re-exec on MULTI-candidate fixture.
		return nil
	})

	// Add a second profile so the multi-candidate branch fires (bypasses
	// confirmSingleProfileSelection).
	tmpDir := t.TempDir()
	for _, name := range []string{"alpha", "gamma"} {
		dir := filepath.Join(tmpDir, "profiles", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		yaml := `auth:
  identities:
    shared-id:
      kind: aws/user
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(yaml), 0o644))
	}

	// Stub reexec.Exec to return a sentinel so the "failed to re-exec"
	// branch fires.
	t.Cleanup(func() { reexec.Exec = originalExecFunc })
	reexec.Exec = func(_ string, _ []string, _ []string) error {
		return errors.New("exec blew up")
	}

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferProfileFallback(context.Background(), "shared-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to re-exec",
		"re-exec failure must propagate through the interactive branch")
}

// Same three cases for the identity-agnostic fallback — interactive user
// aborted, prompt error, re-exec error.
func TestMaybeOfferAnyProfileFallback_InteractiveUserAborted(t *testing.T) {
	resetGlobalProfileState(t)
	stubInteractiveTrue(t)
	stubRunForm(t, huh.ErrUserAborted)

	tmpDir := anyProfileFallbackFixture(t)

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

func TestMaybeOfferAnyProfileFallback_InteractivePromptError(t *testing.T) {
	resetGlobalProfileState(t)
	stubInteractiveTrue(t)
	stubRunForm(t, errors.New("form broke"))

	tmpDir := anyProfileFallbackFixture(t)

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
}

func TestMaybeOfferAnyProfileFallback_InteractiveReExecFails(t *testing.T) {
	resetGlobalProfileState(t)
	stubInteractiveTrue(t)
	stubRunForm(t, nil) // Select form's bound value defaults to first option.

	tmpDir := anyProfileFallbackFixture(t)

	t.Cleanup(func() { reexec.Exec = originalExecFunc })
	reexec.Exec = func(_ string, _ []string, _ []string) error {
		return errors.New("exec blew up")
	}

	m := newFallbackManager(tmpDir)
	err := m.maybeOfferAnyProfileFallback(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to re-exec")
}

// stubRunFormWithAction is referenced only by tests that need to mutate
// the form-bound value before returning. Keep a compile sentinel so the
// helper isn't dead-code-eliminated if all its callers move away.
var _ = stubRunFormWithAction
