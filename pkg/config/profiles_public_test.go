package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// profilesPublicTestFixture creates a CliConfigPath with three profiles:
//   - "alpha" defines identity "root-admin" (auth.identities).
//   - "beta" defines provider "my-sso" only (auth.providers, no identities).
//   - "plain" has no auth config at all.
//
// Returns the CliConfigPath (temp dir root).
func profilesPublicTestFixture(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	alpha := filepath.Join(tmpDir, "profiles", "alpha")
	beta := filepath.Join(tmpDir, "profiles", "beta")
	plain := filepath.Join(tmpDir, "profiles", "plain")
	require.NoError(t, os.MkdirAll(alpha, 0o755))
	require.NoError(t, os.MkdirAll(beta, 0o755))
	require.NoError(t, os.MkdirAll(plain, 0o755))

	alphaYAML := `auth:
  identities:
    root-admin:
      kind: aws/user
`
	betaYAML := `auth:
  providers:
    my-sso:
      kind: aws/sso
`
	plainYAML := `stacks:
  base_path: stacks
`
	require.NoError(t, os.WriteFile(filepath.Join(alpha, "atmos.yaml"), []byte(alphaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(beta, "atmos.yaml"), []byte(betaYAML), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(plain, "atmos.yaml"), []byte(plainYAML), 0o644))

	return tmpDir
}

func TestProfilesWithIdentity(t *testing.T) {
	t.Run("nil config returns nil without error", func(t *testing.T) {
		got, err := ProfilesWithIdentity(nil, "root-admin")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("empty identity name returns nil without error", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{CliConfigPath: "/anywhere"}
		got, err := ProfilesWithIdentity(cfg, "")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("whitespace-only identity name returns nil without error", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{CliConfigPath: "/anywhere"}
		got, err := ProfilesWithIdentity(cfg, "   ")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("identity is found in exactly one profile", func(t *testing.T) {
		tmpDir := profilesPublicTestFixture(t)
		cfg := &schema.AtmosConfiguration{
			CliConfigPath: tmpDir,
			Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
		}

		got, err := ProfilesWithIdentity(cfg, "root-admin")
		require.NoError(t, err)
		assert.Equal(t, []string{"alpha"}, got)
	})

	t.Run("identity is not defined in any profile → empty result", func(t *testing.T) {
		tmpDir := profilesPublicTestFixture(t)
		cfg := &schema.AtmosConfiguration{
			CliConfigPath: tmpDir,
			Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
		}

		got, err := ProfilesWithIdentity(cfg, "does-not-exist")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("malformed profile is skipped; valid profiles still match", func(t *testing.T) {
		tmpDir := profilesPublicTestFixture(t)

		// Add a profile whose atmos.yaml is malformed YAML — the identity
		// search must skip it and still find the valid "alpha" profile.
		brokenDir := filepath.Join(tmpDir, "profiles", "broken")
		require.NoError(t, os.MkdirAll(brokenDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(brokenDir, "atmos.yaml"),
			[]byte(":::not valid yaml:::\n  - :\n"),
			0o644,
		))

		cfg := &schema.AtmosConfiguration{
			CliConfigPath: tmpDir,
			Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
		}

		got, err := ProfilesWithIdentity(cfg, "root-admin")
		require.NoError(t, err, "a single broken profile must not fail the search")
		assert.Equal(t, []string{"alpha"}, got,
			"broken profile is skipped; valid match still returned")
	})
}

func TestProfilesWithAuthConfig(t *testing.T) {
	t.Run("nil config returns nil without error", func(t *testing.T) {
		got, err := ProfilesWithAuthConfig(nil)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("returns both identity- and provider-defining profiles, excluding plain", func(t *testing.T) {
		tmpDir := profilesPublicTestFixture(t)
		cfg := &schema.AtmosConfiguration{
			CliConfigPath: tmpDir,
			Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
		}

		got, err := ProfilesWithAuthConfig(cfg)
		require.NoError(t, err)
		// Result is alphabetically sorted.
		assert.Equal(t, []string{"alpha", "beta"}, got)
		assert.NotContains(t, got, "plain",
			"profiles without auth config must be excluded")
	})

	t.Run("no profiles at all returns empty", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "profiles"), 0o755))

		cfg := &schema.AtmosConfiguration{
			CliConfigPath: tmpDir,
			Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
		}

		got, err := ProfilesWithAuthConfig(cfg)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("malformed profile is skipped; valid auth-bearing profiles still returned", func(t *testing.T) {
		tmpDir := profilesPublicTestFixture(t)

		// Add a profile with malformed YAML — the auth-config search must
		// skip it and still return alpha (identities) + beta (providers).
		brokenDir := filepath.Join(tmpDir, "profiles", "broken")
		require.NoError(t, os.MkdirAll(brokenDir, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(brokenDir, "atmos.yaml"),
			[]byte(":::not valid yaml:::\n  - :\n"),
			0o644,
		))

		cfg := &schema.AtmosConfiguration{
			CliConfigPath: tmpDir,
			Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
		}

		got, err := ProfilesWithAuthConfig(cfg)
		require.NoError(t, err, "a single broken profile must not fail the search")
		assert.Equal(t, []string{"alpha", "beta"}, got,
			"broken profile is skipped; valid auth-bearing profiles still returned")
	})
}

// ProfileDefinesAuthConfig rejects a nil config with ErrInvalidAuthConfig so
// callers don't silently treat an absent config as "no auth".
func TestProfileDefinesAuthConfig_NilConfigErrors(t *testing.T) {
	_, err := ProfileDefinesAuthConfig(nil, "any-profile")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "atmosConfig is nil")
}

// ProfileDefinesAuthConfig returns false (no error) for a whitespace-only
// profile name — callers shouldn't need to pre-trim.
func TestProfileDefinesAuthConfig_EmptyProfileName(t *testing.T) {
	cfg := &schema.AtmosConfiguration{CliConfigPath: "/anywhere"}
	got, err := ProfileDefinesAuthConfig(cfg, "   ")
	require.NoError(t, err)
	assert.False(t, got)
}

// ProfileDefinesAuthConfig returns false (no error) when the named profile
// doesn't exist — same "absence is not an error" contract as the other
// profile-discovery helpers.
func TestProfileDefinesAuthConfig_ProfileNotFoundIsNotAnError(t *testing.T) {
	tmpDir := profilesPublicTestFixture(t)
	cfg := &schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
	}

	got, err := ProfileDefinesAuthConfig(cfg, "nonexistent-profile")
	require.NoError(t, err)
	assert.False(t, got)
}

// ProfileDefinesAuthConfig correctly identifies the three cases against the
// shared fixture: identities-only, providers-only, and no-auth.
func TestProfileDefinesAuthConfig_ClassifiesProfiles(t *testing.T) {
	tmpDir := profilesPublicTestFixture(t)
	cfg := &schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
	}

	tests := []struct {
		profile string
		want    bool
	}{
		{"alpha", true},  // auth.identities populated.
		{"beta", true},   // auth.providers populated.
		{"plain", false}, // neither.
	}
	for _, tc := range tests {
		t.Run(tc.profile, func(t *testing.T) {
			got, err := ProfileDefinesAuthConfig(cfg, tc.profile)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
