package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/reexec"
)

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
