package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// profileIdentityFixture builds a temp directory with two profiles. The
// "alpha" profile defines auth.identities.root-admin; the "beta" profile
// defines auth.identities.dev-user. Used by the helpers tests below.
func profileIdentityFixture(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()

	tmpDir := t.TempDir()

	// Two profile directories, each with its own atmos.yaml.
	alphaDir := filepath.Join(tmpDir, "profiles", "alpha")
	betaDir := filepath.Join(tmpDir, "profiles", "beta")
	emptyDir := filepath.Join(tmpDir, "profiles", "empty")
	require.NoError(t, os.MkdirAll(alphaDir, 0o755))
	require.NoError(t, os.MkdirAll(betaDir, 0o755))
	require.NoError(t, os.MkdirAll(emptyDir, 0o755))

	alphaYAML := `auth:
  identities:
    root-admin:
      kind: aws/user
    other-identity:
      kind: aws/user
`
	betaYAML := `auth:
  identities:
    dev-user:
      kind: aws/user
`
	emptyYAML := `stacks:
  base_path: stacks
`
	require.NoError(t, os.WriteFile(filepath.Join(alphaDir, "atmos.yaml"), []byte(alphaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(betaDir, "atmos.yaml"), []byte(betaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(emptyDir, "atmos.yaml"), []byte(emptyYAML), 0o644))

	return &schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
	}
}

func TestProfileDefinesIdentity_Match(t *testing.T) {
	cfg := profileIdentityFixture(t)

	defines, err := ProfileDefinesIdentity(cfg, "alpha", "root-admin")
	require.NoError(t, err)
	assert.True(t, defines, "alpha profile defines root-admin")
}

func TestProfileDefinesIdentity_CaseInsensitive(t *testing.T) {
	cfg := profileIdentityFixture(t)

	defines, err := ProfileDefinesIdentity(cfg, "alpha", "ROOT-ADMIN")
	require.NoError(t, err)
	assert.True(t, defines, "case-insensitive match should succeed")
}

func TestProfileDefinesIdentity_NotDefined(t *testing.T) {
	cfg := profileIdentityFixture(t)

	defines, err := ProfileDefinesIdentity(cfg, "alpha", "dev-user")
	require.NoError(t, err)
	assert.False(t, defines, "alpha does not define dev-user (that's in beta)")
}

func TestProfileDefinesIdentity_ProfileWithoutAuth(t *testing.T) {
	cfg := profileIdentityFixture(t)

	defines, err := ProfileDefinesIdentity(cfg, "empty", "anything")
	require.NoError(t, err)
	assert.False(t, defines, "profile without auth section defines no identities")
}

func TestProfileDefinesIdentity_MissingProfile(t *testing.T) {
	cfg := profileIdentityFixture(t)

	defines, err := ProfileDefinesIdentity(cfg, "nonexistent", "root-admin")
	assert.NoError(t, err, "missing profile should not surface as error — just 'not defined'")
	assert.False(t, defines)
}

func TestProfileDefinesIdentity_EmptyInputs(t *testing.T) {
	cfg := profileIdentityFixture(t)

	// Empty identity name.
	defines, err := ProfileDefinesIdentity(cfg, "alpha", "")
	require.NoError(t, err)
	assert.False(t, defines)

	// Empty profile name.
	defines, err = ProfileDefinesIdentity(cfg, "", "root-admin")
	require.NoError(t, err)
	assert.False(t, defines)
}

func TestProfileDefinesIdentity_NilConfig(t *testing.T) {
	_, err := ProfileDefinesIdentity(nil, "alpha", "root-admin")
	assert.Error(t, err)
}

func TestProfilesWithIdentity_SingleMatch(t *testing.T) {
	cfg := profileIdentityFixture(t)

	matches, err := ProfilesWithIdentity(cfg, "root-admin")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha"}, matches)
}

func TestProfilesWithIdentity_DifferentProfile(t *testing.T) {
	cfg := profileIdentityFixture(t)

	matches, err := ProfilesWithIdentity(cfg, "dev-user")
	require.NoError(t, err)
	assert.Equal(t, []string{"beta"}, matches)
}

func TestProfilesWithIdentity_NoMatch(t *testing.T) {
	cfg := profileIdentityFixture(t)

	matches, err := ProfilesWithIdentity(cfg, "nonexistent-identity")
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestProfilesWithIdentity_EmptyName(t *testing.T) {
	cfg := profileIdentityFixture(t)

	matches, err := ProfilesWithIdentity(cfg, "")
	require.NoError(t, err)
	assert.Nil(t, matches)
}

func TestProfilesWithIdentity_SortedOutput(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()

	// Three profiles with the same identity — verify deterministic order.
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		dir := filepath.Join(tmpDir, "profiles", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "atmos.yaml"),
			[]byte("auth:\n  identities:\n    shared-identity:\n      kind: aws/user\n"),
			0o644,
		))
	}

	cfg := &schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
	}

	matches, err := ProfilesWithIdentity(cfg, "shared-identity")
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, matches,
		"output should be alphabetically sorted")
}

// HasExplicitProfile inspects os.Args directly (it must work for commands with
// DisableFlagParsing=true where pflag never runs). The three tests below
// therefore manipulate os.Args rather than going through cmd.SetArgs.
func TestHasExplicitProfile_NoFlagOrEnv(t *testing.T) {
	// Clear env and ensure os.Args has no --profile.
	t.Setenv("ATMOS_PROFILE", "")
	origArgs := os.Args
	os.Args = []string{"atmos", "terraform", "plan"}
	t.Cleanup(func() { os.Args = origArgs })

	assert.False(t, HasExplicitProfile())
}

func TestHasExplicitProfile_FlagInArgs(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "")
	origArgs := os.Args
	os.Args = []string{"atmos", "--profile", "dev", "terraform", "plan"}
	t.Cleanup(func() { os.Args = origArgs })

	assert.True(t, HasExplicitProfile())
}

func TestHasExplicitProfile_EnvVar(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "prod")
	origArgs := os.Args
	os.Args = []string{"atmos", "terraform", "plan"}
	t.Cleanup(func() { os.Args = origArgs })

	assert.True(t, HasExplicitProfile())
}

// profileAuthConfigFixture builds profiles covering the four auth-config shapes
// used by the ProfileDefinesAuthConfig / ProfilesWithAuthConfig tests:
//
//	identities-only → has auth.identities, no auth.providers (returns true)
//	providers-only  → has auth.providers, no auth.identities (returns true)
//	both            → has both sections                     (returns true)
//	neither         → no auth block at all                   (returns false)
func profileAuthConfigFixture(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()

	tmpDir := t.TempDir()

	fixtures := map[string]string{
		"identities-only": `auth:
  identities:
    root-admin:
      kind: aws/user
`,
		"providers-only": `auth:
  providers:
    my-sso:
      kind: aws/sso
`,
		"both": `auth:
  identities:
    dev-user:
      kind: aws/user
  providers:
    my-sso:
      kind: aws/sso
`,
		"neither": `stacks:
  base_path: stacks
`,
	}

	for name, body := range fixtures {
		dir := filepath.Join(tmpDir, "profiles", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(body), 0o644))
	}

	return &schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
	}
}

func TestProfileDefinesAuthConfig_IdentitiesOnly(t *testing.T) {
	cfg := profileAuthConfigFixture(t)

	defines, err := ProfileDefinesAuthConfig(cfg, "identities-only")
	require.NoError(t, err)
	assert.True(t, defines, "profile with auth.identities should be a candidate")
}

func TestProfileDefinesAuthConfig_ProvidersOnly(t *testing.T) {
	cfg := profileAuthConfigFixture(t)

	defines, err := ProfileDefinesAuthConfig(cfg, "providers-only")
	require.NoError(t, err)
	assert.True(t, defines, "profile with auth.providers should be a candidate")
}

func TestProfileDefinesAuthConfig_Both(t *testing.T) {
	cfg := profileAuthConfigFixture(t)

	defines, err := ProfileDefinesAuthConfig(cfg, "both")
	require.NoError(t, err)
	assert.True(t, defines)
}

func TestProfileDefinesAuthConfig_Neither(t *testing.T) {
	cfg := profileAuthConfigFixture(t)

	defines, err := ProfileDefinesAuthConfig(cfg, "neither")
	require.NoError(t, err)
	assert.False(t, defines, "profile with no auth block should not be a candidate")
}

func TestProfileDefinesAuthConfig_MissingProfile(t *testing.T) {
	cfg := profileAuthConfigFixture(t)

	defines, err := ProfileDefinesAuthConfig(cfg, "nonexistent")
	assert.NoError(t, err, "missing profile should not surface as error")
	assert.False(t, defines)
}

func TestProfileDefinesAuthConfig_EmptyName(t *testing.T) {
	cfg := profileAuthConfigFixture(t)

	defines, err := ProfileDefinesAuthConfig(cfg, "")
	require.NoError(t, err)
	assert.False(t, defines)
}

func TestProfileDefinesAuthConfig_NilConfig(t *testing.T) {
	_, err := ProfileDefinesAuthConfig(nil, "identities-only")
	assert.Error(t, err)
}

func TestProfilesWithAuthConfig_IncludesAllAuthProfiles(t *testing.T) {
	cfg := profileAuthConfigFixture(t)

	matches, err := ProfilesWithAuthConfig(cfg)
	require.NoError(t, err)
	// "neither" is excluded because it has no auth block; the other three qualify.
	assert.Equal(t, []string{"both", "identities-only", "providers-only"}, matches,
		"should list all profiles with auth config, sorted alphabetically")
}

func TestProfilesWithAuthConfig_NilConfig(t *testing.T) {
	matches, err := ProfilesWithAuthConfig(nil)
	require.NoError(t, err)
	assert.Nil(t, matches)
}

func TestProfilesWithAuthConfig_NoProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	// Create the profiles dir but no profile subdirectories.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "profiles"), 0o755))

	cfg := &schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
	}

	matches, err := ProfilesWithAuthConfig(cfg)
	require.NoError(t, err)
	assert.Empty(t, matches)
}
