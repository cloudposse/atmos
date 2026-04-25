package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestLoadConfig_ProfilesDefault_LoadsWhenNoFlagOrEnv verifies that
// `profiles.default` in the base atmos.yaml is loaded automatically when
// neither --profile nor ATMOS_PROFILE is set.
func TestLoadConfig_ProfilesDefault_LoadsWhenNoFlagOrEnv(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "config-profiles-default")
	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	require.DirExists(t, absFixture, "required scenario fixture is missing")

	t.Chdir(absFixture)

	// Reset Viper between tests to avoid state from prior runs.
	viper.Reset()
	t.Cleanup(viper.Reset)

	// Make sure nothing is "explicitly" active.
	t.Setenv("ATMOS_PROFILE", "")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := LoadConfig(&configAndStacksInfo)
	require.NoError(t, err)

	// developer profile sets logs.level: Debug; base sets Info.
	// If the default profile was loaded, logs.level should be Debug.
	assert.Equal(t, "Debug", atmosConfig.Logs.Level,
		"profiles.default should have loaded the 'developer' profile (logs.level=Debug)")

	// Record what was populated into ProfilesFromArg for diagnostic clarity.
	assert.Equal(t, []string{"developer"}, configAndStacksInfo.ProfilesFromArg,
		"profiles.default should populate ProfilesFromArg with 'developer'")
}

// TestLoadConfig_ProfilesDefault_OverriddenByExplicitFlag verifies that an
// explicit --profile value takes precedence over profiles.default.
func TestLoadConfig_ProfilesDefault_OverriddenByExplicitFlag(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "config-profiles-default")
	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	require.DirExists(t, absFixture, "required scenario fixture is missing")

	t.Chdir(absFixture)

	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_PROFILE", "")

	// Explicitly request the production profile via ConfigAndStacksInfo
	// (simulates --profile production being parsed upstream).
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ProfilesFromArg: []string{"production"},
	}
	atmosConfig, err := LoadConfig(&configAndStacksInfo)
	require.NoError(t, err)

	// production profile sets logs.level: Error.
	assert.Equal(t, "Error", atmosConfig.Logs.Level,
		"explicit --profile production should win over profiles.default")
	assert.Equal(t, []string{"production"}, configAndStacksInfo.ProfilesFromArg,
		"ProfilesFromArg should remain as explicitly provided")
}

// TestLoadConfig_ProfilesDefault_OverriddenByEnvVar verifies that
// ATMOS_PROFILE takes precedence over profiles.default.
func TestLoadConfig_ProfilesDefault_OverriddenByEnvVar(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "config-profiles-default")
	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	require.DirExists(t, absFixture, "required scenario fixture is missing")

	t.Chdir(absFixture)

	viper.Reset()
	t.Cleanup(viper.Reset)

	t.Setenv("ATMOS_PROFILE", "production")

	// Bind env var like production does.
	require.NoError(t, viper.GetViper().BindEnv(profileKey, "ATMOS_PROFILE"))

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := LoadConfig(&configAndStacksInfo)
	require.NoError(t, err)

	assert.Equal(t, "Error", atmosConfig.Logs.Level,
		"ATMOS_PROFILE=production should win over profiles.default")
	assert.Equal(t, []string{"production"}, configAndStacksInfo.ProfilesFromArg)
}

// TestLoadConfig_SyncsProfilesBasePathToGlobalViper verifies that after
// LoadConfig loads an atmos.yaml containing a custom `profiles.base_path`,
// the value is visible on the global viper via viper.GetString.
//
// Regression guard for a bug where LoadConfig uses a local viper instance
// and never writes profiles.base_path to the global viper. The auth profile
// fallback (pkg/auth/profile_fallback.go) reads from the global viper, so
// without this sync it cannot discover profiles at custom locations and
// silently returns no candidates — meaning commands like `atmos auth shell`
// fail with a bare "no default identity" error instead of prompting.
func TestLoadConfig_SyncsProfilesBasePathToGlobalViper(t *testing.T) {
	tempDir := t.TempDir()
	customProfiles := filepath.Join(tempDir, "custom-profiles")
	require.NoError(t, os.MkdirAll(customProfiles, 0o755))

	atmosYAML := "" +
		"components:\n" +
		"  terraform:\n" +
		"    base_path: \"components/terraform\"\n" +
		"stacks:\n" +
		"  base_path: \"stacks\"\n" +
		"logs:\n" +
		"  level: Info\n" +
		"profiles:\n" +
		"  base_path: \"custom-profiles\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tempDir)

	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_PROFILE", "")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := LoadConfig(&configAndStacksInfo)
	require.NoError(t, err)

	require.Equal(t, "custom-profiles", atmosConfig.Profiles.BasePath,
		"precondition: atmos.yaml profiles.base_path should be loaded into atmosConfig")

	assert.Equal(t, "custom-profiles", viper.GetString("profiles.base_path"),
		"LoadConfig must sync profiles.base_path to global viper so the auth "+
			"profile fallback can find profiles at custom locations")
}

// TestLoadConfig_DoesNotOverwriteGlobalProfilesBasePathWhenUnset verifies
// that when atmos.yaml does not set profiles.base_path, LoadConfig does not
// overwrite an existing value on the global viper (e.g., one set by another
// mechanism such as a flag binding).
func TestLoadConfig_DoesNotOverwriteGlobalProfilesBasePathWhenUnset(t *testing.T) {
	tempDir := t.TempDir()

	atmosYAML := "" +
		"components:\n" +
		"  terraform:\n" +
		"    base_path: \"components/terraform\"\n" +
		"stacks:\n" +
		"  base_path: \"stacks\"\n" +
		"logs:\n" +
		"  level: Info\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosYAML), 0o644))

	t.Chdir(tempDir)

	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_PROFILE", "")

	// Pre-populate the global viper with a value from a different source.
	viper.GetViper().Set("profiles.base_path", "preexisting-value")

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	_, err := LoadConfig(&configAndStacksInfo)
	require.NoError(t, err)

	assert.Equal(t, "preexisting-value", viper.GetString("profiles.base_path"),
		"LoadConfig must not overwrite a pre-existing global viper value "+
			"when atmos.yaml does not set profiles.base_path")
}
