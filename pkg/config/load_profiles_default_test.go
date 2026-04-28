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

// TestLoadConfig_ParsesProfileFromArgsWhenViperIsSetButEmpty is a regression
// guard for the profile-fallback re-exec path on DisableFlagParsing commands
// (terraform, helmfile, packer, auth exec).
//
// Production symptom: when the profile-fallback re-exec runs
// `atmos --profile managers auth exec ...`, the child's leaf command has
// DisableFlagParsing=true, so Cobra does not parse --profile. Something
// upstream (an earlier binding, a default, or a prior Set call) causes
// `viper.IsSet("profile")` to report true while `GetStringSlice("profile")`
// returns an empty slice. The previous short-circuit in
// `getProfilesFromFlagsOrEnv` trusted `IsSet` and never reached the
// os.Args fallback, so the picked profile from the fallback re-exec was
// silently ignored and the child produced "no default identity configured:
// no identities available".
//
// This test reproduces that state directly by calling
// viper.Set(profileKey, []string{}) — which is the simplest way to force
// the "IsSet=true, GetStringSlice=[]" divergence that broke the fallback.
// It then sets os.Args to contain `--profile production` and asserts that
// LoadConfig falls back to os.Args parsing and loads the production profile.
func TestLoadConfig_ParsesProfileFromArgsWhenViperIsSetButEmpty(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "config-profiles-default")
	absFixture, err := filepath.Abs(fixturePath)
	require.NoError(t, err)

	require.DirExists(t, absFixture, "required scenario fixture is missing")

	t.Chdir(absFixture)

	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_PROFILE", "")

	// Reproduce the production viper state: IsSet=true (key was touched by
	// an upstream Set call) but GetStringSlice=[] (no real value was stored).
	// This is the exact divergence that caused the old `if !IsSet { fallback }`
	// gate to skip the os.Args fallback and drop the picked profile.
	viper.GetViper().Set(profileKey, []string{})

	require.True(t, viper.GetViper().IsSet(profileKey),
		"precondition: viper.Set with empty slice must make IsSet=true")
	require.Empty(t, viper.GetViper().GetStringSlice(profileKey),
		"precondition: IsSet=true can coexist with GetStringSlice=[]")

	// Swap os.Args to mimic the re-exec'd child: argv contains --profile
	// production even though cobra never parsed it.
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"atmos", "--profile", "production", "auth", "exec", "--", "echo", "hi"}

	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := LoadConfig(&configAndStacksInfo)
	require.NoError(t, err)

	assert.Equal(t, "Error", atmosConfig.Logs.Level,
		"LoadConfig must fall back to os.Args parsing when viper has "+
			"IsSet=true but GetStringSlice=[] — otherwise the profile-fallback "+
			"re-exec path silently drops the picked profile")
	assert.Equal(t, []string{"production"}, configAndStacksInfo.ProfilesFromArg,
		"picked profile must be populated from os.Args fallback")
}
