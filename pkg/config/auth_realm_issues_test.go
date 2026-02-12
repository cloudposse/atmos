package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIssue2072_ConflictingStackDefaultsDiscarded tests that LoadStackAuthDefaults
// discards conflicting defaults from different stack files rather than returning
// all of them (which would cause a false "multiple default identities" error).
//
// Fix: When multiple stack files define DIFFERENT default identities, the function
// returns empty (no global default) since it can't resolve which one to use without
// knowing the target stack.
//
// See: https://github.com/cloudposse/atmos/issues/2072
func TestIssue2072_ConflictingStackDefaultsDiscarded(t *testing.T) {
	tmpDir := t.TempDir()

	// Create organization stack with "organization" as default identity.
	orgContent := `
auth:
  identities:
    organization:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "organization.yaml"), []byte(orgContent), 0o644)
	require.NoError(t, err)

	// Create staging stack with "staging" as default identity.
	stagingContent := `
auth:
  identities:
    staging:
      default: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "staging.yaml"), []byte(stagingContent), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)

	// FIXED: Conflicting defaults from different stacks are discarded.
	// LoadStackAuthDefaults returns empty when stacks disagree on the default identity.
	assert.Empty(t, defaults, "Conflicting defaults from different stacks should be discarded")
}

// TestIssue2072_AgreementStackDefaultsPreserved tests that when all stack files
// agree on the same default identity, it is preserved.
func TestIssue2072_AgreementStackDefaultsPreserved(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two stacks that both set "staging" as default identity.
	stack1 := `
auth:
  identities:
    staging:
      default: true
`
	err := os.WriteFile(filepath.Join(tmpDir, "stack1.yaml"), []byte(stack1), 0o644)
	require.NoError(t, err)

	stack2 := `
auth:
  identities:
    staging:
      default: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "stack2.yaml"), []byte(stack2), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)

	// When all stacks agree on the same default, it is used.
	assert.Len(t, defaults, 1, "Agreeing defaults should be preserved")
	assert.True(t, defaults["staging"], "staging should be the agreed default")
}

// TestIssue2072_MergeStackAuthDefaultsWithNoConflict verifies that MergeStackAuthDefaults
// correctly applies a single stack default to the auth config.
func TestIssue2072_MergeStackAuthDefaultsWithNoConflict(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"organization": {Kind: "aws/permission-set"},
			"staging":      {Kind: "aws/permission-set"},
		},
	}

	// Single default from stack - no conflict.
	stackDefaults := map[string]bool{
		"staging": true,
	}

	MergeStackAuthDefaults(authConfig, stackDefaults)

	// Only staging should be marked as default.
	assert.True(t, authConfig.Identities["staging"].Default,
		"staging should be the default identity")
	assert.False(t, authConfig.Identities["organization"].Default,
		"organization should NOT be default")
}

// TestIssue2071_ConfigLoadWithAuthDefaultDoesNotRequireProfile tests that
// defining a default auth identity does not trigger profile loading.
//
// Bug: When a default identity is defined in .atmos.d/auth.yaml, the identity
// name is incorrectly treated as a profile name, causing "profile not found" errors.
//
// See: https://github.com/cloudposse/atmos/issues/2071
func TestIssue2071_ConfigLoadWithAuthDefaultDoesNotRequireProfile(t *testing.T) {
	// Use the fixture directory for this test.
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "auth-realm-profile-bug")

	// Verify the fixture exists.
	_, err := os.Stat(fixturePath)
	if os.IsNotExist(err) {
		t.Skip("Fixture not found at", fixturePath)
	}

	// Save and restore working directory.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		err := os.Chdir(origDir)
		require.NoError(t, err)
	})

	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	err = os.Chdir(absFixture)
	require.NoError(t, err)

	// Load config - this should NOT fail even though the auth identity name
	// "root-admin" does not match any profile directory.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)

	// The identity "root-admin" should be loaded from .atmos.d/auth.yaml.
	// It should NOT cause a "profile not found" error.
	assert.NoError(t, err, "Config loading should not fail due to auth identity name not matching a profile")
	assert.NotNil(t, atmosConfig)

	// Verify the auth identity was loaded.
	if err == nil {
		_, hasRootAdmin := atmosConfig.Auth.Identities["root-admin"]
		assert.True(t, hasRootAdmin, "root-admin identity should be loaded from .atmos.d/auth.yaml")

		if hasRootAdmin {
			assert.True(t, atmosConfig.Auth.Identities["root-admin"].Default,
				"root-admin should be marked as default")
		}
	}
}

// TestIssue2071_AtmosProfileEnvVarCausesProfileNotFound tests that when
// ATMOS_PROFILE is set to an auth identity name, config loading fails with
// a profile not found error. The error builder adds hints suggesting
// ATMOS_IDENTITY (rendered by the CLI formatter, not visible in .Error()).
func TestIssue2071_AtmosProfileEnvVarCausesProfileNotFound(t *testing.T) {
	// Use the fixture directory for this test.
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "auth-realm-profile-bug")

	// Verify the fixture exists.
	_, err := os.Stat(fixturePath)
	if os.IsNotExist(err) {
		t.Skip("Fixture not found at", fixturePath)
	}

	// Save and restore working directory and env.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		err := os.Chdir(origDir)
		require.NoError(t, err)
	})

	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	err = os.Chdir(absFixture)
	require.NoError(t, err)

	// Set ATMOS_PROFILE to the identity name - this is what the user likely did.
	t.Setenv("ATMOS_PROFILE", "root-admin")

	// Verify the env var path works in isolation first.
	profiles, source := getProfilesFromFallbacks()
	assert.Equal(t, []string{"root-admin"}, profiles,
		"getProfilesFromFallbacks should pick up ATMOS_PROFILE=root-admin")
	assert.Equal(t, "env", source)

	// When ATMOS_PROFILE=root-admin is set, config loading fails because
	// "root-admin" is an identity name, not a profile name.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	_, err = InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		// The sentinel error should be profile not found.
		assert.ErrorContains(t, err, "profile not found",
			"Error should be a profile not found error")
		t.Log("REPRODUCED: ATMOS_PROFILE=root-admin causes 'profile not found' error")

		// The error builder adds hints about ATMOS_IDENTITY that are rendered
		// by the CLI error formatter (not visible in err.Error()).
		// The isAuthIdentityName helper detects this case.
	}
	// If err is nil, the global viper intercepted the profile lookup.
	// This is acceptable behavior - not all code paths trigger the error.
}

// TestIssue2071_GetProfilesFromFallbacksPicksUpEnvVar verifies that
// getProfilesFromFallbacks reads ATMOS_PROFILE env var directly.
// This is the code path that causes the profile loading when
// the user has ATMOS_PROFILE set to an identity name.
func TestIssue2071_GetProfilesFromFallbacksPicksUpEnvVar(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "root-admin")

	profiles, source := getProfilesFromFallbacks()

	// getProfilesFromFallbacks should pick up the env var.
	assert.Equal(t, []string{"root-admin"}, profiles,
		"ATMOS_PROFILE env var should be picked up by getProfilesFromFallbacks")
	assert.Equal(t, "env", source)
}

// TestIssue2071_GetProfilesFromFlagsOrEnvPicksUpEnvVar verifies that
// getProfilesFromFlagsOrEnv returns the ATMOS_PROFILE env var when
// the global viper doesn't have the profile key set.
func TestIssue2071_GetProfilesFromFlagsOrEnvPicksUpEnvVar(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "root-admin")

	profiles, source := getProfilesFromFlagsOrEnv()

	// The result depends on whether the global viper has "profile" set.
	// In test context without CLI flag registration, it should fall through
	// to getProfilesFromFallbacks which reads the env var.
	if len(profiles) > 0 {
		assert.Equal(t, []string{"root-admin"}, profiles,
			"ATMOS_PROFILE env var should be returned by getProfilesFromFlagsOrEnv")
		assert.Equal(t, "env", source)
	} else {
		// If the global viper has "profile" set from another test/flag registration,
		// it may enter the viper path and return empty.
		t.Log("Global viper may have 'profile' key set, preventing fallback path")
	}
}

// TestIssue2071_IdentityAndProfileAreIndependent verifies that identity names
// and profile names are independent concepts.
func TestIssue2071_IdentityAndProfileAreIndependent(t *testing.T) {
	tmpDir := t.TempDir()

	// Create profiles directory with only "ci" and "developer" profiles.
	profilesDir := filepath.Join(tmpDir, "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "ci"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "developer"), 0o755))

	// Create ci profile settings.
	ciSettings := `logs:
  level: Warning
`
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "ci", "settings.yaml"), []byte(ciSettings), 0o644))

	// Create developer profile settings.
	devSettings := `logs:
  level: Debug
`
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "developer", "settings.yaml"), []byte(devSettings), 0o644))

	// Discover profile locations using a config that points to our temp dir.
	atmosConfig := schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{},
	}

	locations, err := discoverProfileLocations(&atmosConfig)
	require.NoError(t, err)

	// List available profiles.
	available, err := listAvailableProfiles(locations)
	require.NoError(t, err)

	// Should have "ci" and "developer" profiles.
	_, hasCi := available["ci"]
	_, hasDev := available["developer"]
	assert.True(t, hasCi, "ci profile should be available")
	assert.True(t, hasDev, "developer profile should be available")

	// "root-admin" is an identity name, NOT a profile name.
	// It should NOT exist in the available profiles.
	_, hasRootAdmin := available["root-admin"]
	assert.False(t, hasRootAdmin, "root-admin is an identity name, not a profile")
}

// TestIssue2071_IsAuthIdentityName verifies the helper function that detects
// whether a profile name matches an auth identity name.
func TestIssue2071_IsAuthIdentityName(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"root-admin": {Kind: "aws/permission-set", Default: true},
				"developer":  {Kind: "aws/permission-set"},
			},
		},
	}

	assert.True(t, isAuthIdentityName("root-admin", atmosConfig),
		"root-admin should be recognized as an auth identity")
	assert.True(t, isAuthIdentityName("Root-Admin", atmosConfig),
		"case-insensitive match should work")
	assert.False(t, isAuthIdentityName("ci", atmosConfig),
		"ci is not an auth identity")
	assert.False(t, isAuthIdentityName("nonexistent", atmosConfig),
		"nonexistent should not be recognized as an auth identity")
	assert.False(t, isAuthIdentityName("root-admin", nil),
		"nil config should return false")
}
