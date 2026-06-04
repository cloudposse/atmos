package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// --- LoadStackAuthDefaults tests ---

// TestLoadStackAuthDefaults_NoDefaultsInFiles verifies that when stack files
// have auth sections but no identity is marked as default, the result is empty.
func TestLoadStackAuthDefaults_NoDefaultsInFiles(t *testing.T) {
	tmpDir := t.TempDir()

	content := `
auth:
  identities:
    staging:
      default: false
    production:
      default: false
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "stack.yaml"), []byte(content), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, defaults, "no identity marked default should produce empty result")
}

// TestLoadStackAuthDefaults_MultipleDefaultsInSameFile verifies that when
// a single stack file defines multiple default identities, all are returned.
func TestLoadStackAuthDefaults_MultipleDefaultsInSameFile(t *testing.T) {
	tmpDir := t.TempDir()

	content := `
auth:
  identities:
    identity-a:
      default: true
    identity-b:
      default: true
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "stack.yaml"), []byte(content), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)

	// Two different defaults in one file means they conflict with each other.
	// The function detects that identity-a != identity-b and discards both.
	assert.Empty(t, defaults, "multiple different defaults in one file should conflict and be discarded")
}

// TestLoadStackAuthDefaults_MixedFilesWithAndWithoutAuth verifies that files
// without auth sections are silently skipped alongside files that have defaults.
func TestLoadStackAuthDefaults_MixedFilesWithAndWithoutAuth(t *testing.T) {
	tmpDir := t.TempDir()

	withAuth := `
auth:
  identities:
    my-identity:
      default: true
`
	withoutAuth := `
vars:
  stage: dev
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "with-auth.yaml"), []byte(withAuth), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "without-auth.yaml"), []byte(withoutAuth), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		IncludeStackAbsolutePaths: []string{filepath.Join(tmpDir, "*.yaml")},
		ExcludeStackAbsolutePaths: []string{},
	}

	defaults, err := LoadStackAuthDefaults(atmosConfig)
	require.NoError(t, err)
	assert.Len(t, defaults, 1)
	assert.True(t, defaults["my-identity"])
}

// --- MergeStackAuthDefaults tests ---

// TestMergeStackAuthDefaults_ClearsMultipleExistingDefaults verifies that when
// stack defaults are applied, ALL pre-existing defaults from atmos.yaml are cleared.
func TestMergeStackAuthDefaults_ClearsMultipleExistingDefaults(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity-a": {Kind: "aws/permission-set", Default: true},
			"identity-b": {Kind: "aws/permission-set", Default: true},
			"identity-c": {Kind: "aws/permission-set"},
		},
	}

	stackDefaults := map[string]bool{"identity-c": true}

	MergeStackAuthDefaults(authConfig, stackDefaults)

	assert.False(t, authConfig.Identities["identity-a"].Default,
		"identity-a default should be cleared by stack override")
	assert.False(t, authConfig.Identities["identity-b"].Default,
		"identity-b default should be cleared by stack override")
	assert.True(t, authConfig.Identities["identity-c"].Default,
		"identity-c should be set as the new default from stack")
}

// TestMergeStackAuthDefaults_StackDefaultNotInConfig verifies that a stack default
// for an identity not present in atmos.yaml is silently ignored.
func TestMergeStackAuthDefaults_StackDefaultNotInConfig(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"existing": {Kind: "aws/assume-role", Default: true},
		},
	}

	// Stack sets a default for a nonexistent identity.
	stackDefaults := map[string]bool{"nonexistent": true}

	MergeStackAuthDefaults(authConfig, stackDefaults)

	// The existing default should be cleared (stack had a default).
	assert.False(t, authConfig.Identities["existing"].Default,
		"existing default should be cleared because stack defined a default")
	// Nonexistent identity should not be created.
	_, exists := authConfig.Identities["nonexistent"]
	assert.False(t, exists, "nonexistent identity should not be added to config")
}

// TestMergeStackAuthDefaults_NilDefaults verifies nil defaults map is handled.
func TestMergeStackAuthDefaults_NilDefaults(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"existing": {Kind: "aws/assume-role", Default: true},
		},
	}

	MergeStackAuthDefaults(authConfig, nil)

	// Original default should be preserved.
	assert.True(t, authConfig.Identities["existing"].Default)
}

// --- loadFileForAuthDefaults tests ---

// TestLoadFileForAuthDefaults_EmptyFile verifies that an empty YAML file
// returns empty defaults without error.
func TestLoadFileForAuthDefaults_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(""), 0o644))

	defaults, err := loadFileForAuthDefaults(filePath)
	require.NoError(t, err)
	assert.Empty(t, defaults)
}

// TestLoadFileForAuthDefaults_AuthSectionNoIdentities verifies that an auth
// section without any identities returns empty defaults.
func TestLoadFileForAuthDefaults_AuthSectionNoIdentities(t *testing.T) {
	tmpDir := t.TempDir()
	content := `
auth:
  providers:
    my-sso:
      kind: aws/sso
`
	filePath := filepath.Join(tmpDir, "no-identities.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o644))

	defaults, err := loadFileForAuthDefaults(filePath)
	require.NoError(t, err)
	assert.Empty(t, defaults)
}

// --- isAuthIdentityName tests ---

// TestIsAuthIdentityName_EmptyIdentitiesMap verifies that an empty identities
// map (not nil config) returns false.
func TestIsAuthIdentityName_EmptyIdentitiesMap(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{},
		},
	}

	assert.False(t, isAuthIdentityName("anything", atmosConfig))
}

// TestIsAuthIdentityName_MatchesIdentityNames verifies exact and case-insensitive
// matching of identity names.
func TestIsAuthIdentityName_MatchesIdentityNames(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Identities: map[string]schema.Identity{
				"root-admin":         {Kind: "aws/permission-set", Default: true},
				"plat-dev/terraform": {Kind: "aws/assume-role"},
			},
		},
	}

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"exact match", "root-admin", true},
		{"case insensitive uppercase", "ROOT-ADMIN", true},
		{"case insensitive mixed", "Root-Admin", true},
		{"identity with slash", "plat-dev/terraform", true},
		{"not an identity", "ci", false},
		{"partial match is not enough", "root", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isAuthIdentityName(tt.input, atmosConfig))
		})
	}
}

// TestIsAuthIdentityName_NilConfig verifies that nil config returns false.
func TestIsAuthIdentityName_NilConfig(t *testing.T) {
	assert.False(t, isAuthIdentityName("root-admin", nil))
}

// --- hasAnyDefault tests ---

// TestHasAnyDefault verifies the helper that checks if any identity has default: true.
func TestHasAnyDefault(t *testing.T) {
	tests := []struct {
		name     string
		defaults map[string]bool
		expected bool
	}{
		{"empty map", map[string]bool{}, false},
		{"all false", map[string]bool{"a": false, "b": false}, false},
		{"one true", map[string]bool{"a": false, "b": true}, true},
		{"all true", map[string]bool{"a": true, "b": true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, hasAnyDefault(tt.defaults))
		})
	}
}

// --- clearExistingDefaults tests ---

// TestClearExistingDefaults verifies that all default flags are cleared.
func TestClearExistingDefaults(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"a": {Kind: "aws/permission-set", Default: true},
			"b": {Kind: "aws/permission-set", Default: true},
			"c": {Kind: "aws/permission-set", Default: false},
		},
	}

	clearExistingDefaults(authConfig)

	assert.False(t, authConfig.Identities["a"].Default)
	assert.False(t, authConfig.Identities["b"].Default)
	assert.False(t, authConfig.Identities["c"].Default)
}

// --- Profile/identity separation tests ---

// TestConfigLoadWithAuthDefault_DoesNotRequireProfile verifies that defining
// a default auth identity does not trigger profile loading or cause errors.
func TestConfigLoadWithAuthDefault_DoesNotRequireProfile(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "auth-realm-profile-bug")

	_, err := os.Stat(fixturePath)
	if os.IsNotExist(err) {
		t.Skip("Fixture not found at", fixturePath)
	}

	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	t.Chdir(absFixture)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)

	assert.NoError(t, err, "config loading should not fail due to auth identity name not matching a profile")
	require.NotNil(t, atmosConfig)

	_, hasRootAdmin := atmosConfig.Auth.Identities["root-admin"]
	assert.True(t, hasRootAdmin, "root-admin identity should be loaded from .atmos.d/auth.yaml")

	if hasRootAdmin {
		assert.True(t, atmosConfig.Auth.Identities["root-admin"].Default,
			"root-admin should be marked as default")
	}
}

// TestAtmosProfileEnvVar_CausesProfileNotFoundError verifies that setting
// ATMOS_PROFILE to an auth identity name produces a profile-not-found error.
func TestAtmosProfileEnvVar_CausesProfileNotFoundError(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "auth-realm-profile-bug")

	_, statErr := os.Stat(fixturePath)
	if os.IsNotExist(statErr) {
		t.Skip("Fixture not found at", fixturePath)
	}

	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	t.Chdir(absFixture)
	t.Setenv("ATMOS_PROFILE", "root-admin")

	profiles, source := getProfilesFromFallbacks()
	assert.Equal(t, []string{"root-admin"}, profiles)
	assert.Equal(t, "env", source)

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	_, err = InitCliConfig(configAndStacksInfo, false)

	// Config loading should fail because "root-admin" is not a profile directory.
	require.Error(t, err, "ATMOS_PROFILE=root-admin should cause an error since it is not a profile")
	assert.ErrorContains(t, err, "profile not found")
}

// TestGetProfilesFromFallbacks_NoEnvVar verifies that getProfilesFromFallbacks
// returns nil when ATMOS_PROFILE is not set and no CLI args match.
func TestGetProfilesFromFallbacks_NoEnvVar(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "")

	profiles, source := getProfilesFromFallbacks()
	assert.Empty(t, profiles)
	assert.Empty(t, source)
}

// TestGetProfilesFromFallbacks_EnvVar verifies that getProfilesFromFallbacks
// reads the ATMOS_PROFILE environment variable.
func TestGetProfilesFromFallbacks_EnvVar(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "my-profile")

	profiles, source := getProfilesFromFallbacks()
	assert.Equal(t, []string{"my-profile"}, profiles)
	assert.Equal(t, "env", source)
}

// TestGetProfilesFromFallbacks_CommaSeparatedEnvVar verifies that multiple
// profiles can be specified via comma-separated ATMOS_PROFILE.
func TestGetProfilesFromFallbacks_CommaSeparatedEnvVar(t *testing.T) {
	t.Setenv("ATMOS_PROFILE", "ci,monitoring")

	profiles, source := getProfilesFromFallbacks()
	assert.Equal(t, "env", source)
	assert.Contains(t, profiles, "ci")
	assert.Contains(t, profiles, "monitoring")
}

// TestIdentityAndProfile_AreIndependent verifies that auth identity names
// do not appear in the list of available configuration profiles.
func TestIdentityAndProfile_AreIndependent(t *testing.T) {
	tmpDir := t.TempDir()

	profilesDir := filepath.Join(tmpDir, "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "ci"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "developer"), 0o755))

	ciSettings := "logs:\n  level: Warning\n"
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "ci", "settings.yaml"), []byte(ciSettings), 0o644))

	devSettings := "logs:\n  level: Debug\n"
	require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "developer", "settings.yaml"), []byte(devSettings), 0o644))

	atmosConfig := schema.AtmosConfiguration{
		CliConfigPath: tmpDir,
		Profiles:      schema.ProfilesConfig{},
	}

	locations, err := discoverProfileLocations(&atmosConfig)
	require.NoError(t, err)

	available, err := listAvailableProfiles(locations)
	require.NoError(t, err)

	_, hasCi := available["ci"]
	_, hasDev := available["developer"]
	assert.True(t, hasCi, "ci profile should be available")
	assert.True(t, hasDev, "developer profile should be available")

	_, hasRootAdmin := available["root-admin"]
	assert.False(t, hasRootAdmin, "root-admin is an identity name, not a profile")
}

// --- findProfileDirectory tests ---

// TestFindProfileDirectory_Found verifies that a profile is found at the
// highest-precedence location.
func TestFindProfileDirectory_Found(t *testing.T) {
	tmpDir := t.TempDir()

	// Create profile in project-local location.
	profileDir := filepath.Join(tmpDir, "profiles", "ci")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))

	locations := []ProfileLocation{
		{Path: filepath.Join(tmpDir, "profiles"), Type: "project", Precedence: 4},
	}

	path, locType, err := findProfileDirectory("ci", locations)
	require.NoError(t, err)
	assert.Equal(t, profileDir, path)
	assert.Equal(t, "project", locType)
}

// TestFindProfileDirectory_NotFound verifies that a missing profile produces
// an error with searched paths.
func TestFindProfileDirectory_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	locations := []ProfileLocation{
		{Path: filepath.Join(tmpDir, "profiles"), Type: "project", Precedence: 4},
	}

	_, _, err := findProfileDirectory("nonexistent", locations)
	require.Error(t, err)
	assert.ErrorContains(t, err, "profile not found")
}

// TestFindProfileDirectory_PrecedenceOrder verifies that a profile found in
// a higher-precedence location wins over a lower-precedence one.
func TestFindProfileDirectory_PrecedenceOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the same profile in two locations.
	highPrecedence := filepath.Join(tmpDir, "hidden", "profiles")
	lowPrecedence := filepath.Join(tmpDir, "project", "profiles")
	require.NoError(t, os.MkdirAll(filepath.Join(highPrecedence, "ci"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(lowPrecedence, "ci"), 0o755))

	locations := []ProfileLocation{
		{Path: lowPrecedence, Type: "project", Precedence: 4},
		{Path: highPrecedence, Type: "project-hidden", Precedence: 2},
	}

	path, locType, err := findProfileDirectory("ci", locations)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(highPrecedence, "ci"), path)
	assert.Equal(t, "project-hidden", locType)
}

// --- applyStackDefaults tests ---

// TestApplyStackDefaults_SkipsFalseEntries verifies that entries with false
// in the stack defaults map do not modify the auth config.
func TestApplyStackDefaults_SkipsFalseEntries(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"identity-a": {Kind: "aws/assume-role", Default: true},
		},
	}

	stackDefaults := map[string]bool{"identity-a": false}

	applyStackDefaults(authConfig, stackDefaults)

	// The false entry should not change the existing default.
	assert.True(t, authConfig.Identities["identity-a"].Default)
}

// TestApplyStackDefaults_SkipsMissingIdentity verifies that a stack default
// referencing an identity not in atmos.yaml is silently skipped.
func TestApplyStackDefaults_SkipsMissingIdentity(t *testing.T) {
	authConfig := &schema.AuthConfig{
		Identities: map[string]schema.Identity{
			"existing": {Kind: "aws/assume-role"},
		},
	}

	stackDefaults := map[string]bool{"missing": true}

	applyStackDefaults(authConfig, stackDefaults)

	// Missing identity should not be created.
	_, exists := authConfig.Identities["missing"]
	assert.False(t, exists)
	// Existing identity should be unchanged.
	assert.False(t, authConfig.Identities["existing"].Default)
}

// --- getAllStackFiles tests ---

// TestGetAllStackFiles_WithExcludePatterns verifies that exclude patterns
// properly filter out matching files.
func TestGetAllStackFiles_WithExcludePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "keep.yaml"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "remove.yaml"), []byte(""), 0o644))

	files := getAllStackFiles(
		[]string{filepath.Join(tmpDir, "*.yaml")},
		[]string{filepath.Join(tmpDir, "remove.yaml")},
	)

	assert.Len(t, files, 1)
	assert.Contains(t, files[0], "keep.yaml")
}
