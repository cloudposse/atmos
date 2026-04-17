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

	if _, statErr := os.Stat(absFixture); os.IsNotExist(statErr) {
		t.Skipf("fixture not found at %s", absFixture)
	}

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

	if _, statErr := os.Stat(absFixture); os.IsNotExist(statErr) {
		t.Skipf("fixture not found at %s", absFixture)
	}

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

	if _, statErr := os.Stat(absFixture); os.IsNotExist(statErr) {
		t.Skipf("fixture not found at %s", absFixture)
	}

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
