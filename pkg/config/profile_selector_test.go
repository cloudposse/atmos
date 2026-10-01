package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// withTestProfileSelector registers fn as the process-wide ProfileSelector for the duration of
// the test and restores the previous value afterward via t.Cleanup. This mirrors the
// save-then-restore-via-t.Cleanup pattern already used in this package for other global state
// (e.g. viper.Reset()/t.Cleanup(viper.Reset) in load_profiles_default_test.go, or
// origEdition/t.Cleanup in load_edition_test.go) so tests never leak state into each other.
func withTestProfileSelector(t *testing.T, fn ProfileSelector) {
	t.Helper()

	original := profileSelector
	SetProfileSelector(fn)
	t.Cleanup(func() {
		profileSelector = original
	})
}

// TestResolveProfileSelectionSentinel_NoSentinel verifies that profiles without the sentinel
// pass through unchanged and the selector is never invoked.
func TestResolveProfileSelectionSentinel_NoSentinel(t *testing.T) {
	called := false
	withTestProfileSelector(t, func(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error) {
		called = true
		return nil, nil
	})

	profiles := []string{"foo", "bar"}
	resolved, err := resolveProfileSelectionSentinel(&schema.AtmosConfiguration{}, profiles)
	require.NoError(t, err)
	assert.Equal(t, profiles, resolved)
	assert.False(t, called, "selector must not be called when the sentinel is absent")
}

// TestResolveProfileSelectionSentinel_NilSelector verifies that a nil selector (e.g. a caller
// that imports pkg/config directly without pkg/flags ever registering one) produces a clear,
// static error rather than silently trying to load a profile literally named "__SELECT__".
func TestResolveProfileSelectionSentinel_NilSelector(t *testing.T) {
	withTestProfileSelector(t, nil)

	_, err := resolveProfileSelectionSentinel(&schema.AtmosConfiguration{}, []string{ProfileFlagSelectValue})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProfileSelectionUnavailable)
}

// TestResolveProfileSelectionSentinel_CallsSelectorWithPreselected verifies the sentinel is
// detected regardless of position, the sentinel itself is excluded from preselected, the
// selector receives the same tempConfig pointer, and the resolved names are written back to
// the global Viper singleton for other same-process readers.
func TestResolveProfileSelectionSentinel_CallsSelectorWithPreselected(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	var gotPreselected []string
	var gotConfig *schema.AtmosConfiguration
	withTestProfileSelector(t, func(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error) {
		gotPreselected = preselected
		gotConfig = atmosConfig
		return []string{"picked1", "picked2"}, nil
	})

	tempConfig := &schema.AtmosConfiguration{CliConfigPath: "/tmp/example"}
	resolved, err := resolveProfileSelectionSentinel(tempConfig, []string{"foo", ProfileFlagSelectValue})
	require.NoError(t, err)

	assert.Equal(t, []string{"picked1", "picked2"}, resolved)
	assert.Equal(t, []string{"foo"}, gotPreselected, "the sentinel itself must not appear in preselected")
	assert.Same(t, tempConfig, gotConfig, "selector should receive the same tempConfig pointer LoadConfig built")

	assert.Equal(t, []string{"picked1", "picked2"}, viper.GetViper().GetStringSlice(profileKey),
		"resolved profiles must be written back to viper so other same-process readers of "+
			"--profile/ATMOS_PROFILE see the resolved names instead of the sentinel")
}

// TestResolveProfileSelectionSentinel_UserAborted verifies that ErrUserAborted (user hit
// ctrl+c/esc in the picker) propagates as a real error rather than being swallowed.
func TestResolveProfileSelectionSentinel_UserAborted(t *testing.T) {
	withTestProfileSelector(t, func(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error) {
		return nil, errUtils.ErrUserAborted
	})

	_, err := resolveProfileSelectionSentinel(&schema.AtmosConfiguration{}, []string{ProfileFlagSelectValue})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrUserAborted)
}

// TestResolveProfileSelectionSentinel_EmptySelectionNoError verifies that a user deliberately
// selecting nothing (selector returns an empty/nil list with no error) is not itself an error --
// it's equivalent to not passing --profile at all.
func TestResolveProfileSelectionSentinel_EmptySelectionNoError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	withTestProfileSelector(t, func(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error) {
		return nil, nil
	})

	resolved, err := resolveProfileSelectionSentinel(&schema.AtmosConfiguration{}, []string{ProfileFlagSelectValue})
	require.NoError(t, err)
	assert.Empty(t, resolved)
}

// TestLoadConfig_ResolvesBareProfileSentinelViaSelector is an end-to-end test proving the
// resolved profile names actually flow through LoadConfig into the final loaded configuration,
// not just through the isolated resolveProfileSelectionSentinel helper.
func TestLoadConfig_ResolvesBareProfileSentinelViaSelector(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))

	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "profiles", "picked"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(`
base_path: .
profiles:
  base_path: profiles
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "profiles", "picked", "atmos.yaml"), []byte(`
logs:
  level: Debug
`), 0o644))

	var gotPreselected []string
	withTestProfileSelector(t, func(atmosConfig *schema.AtmosConfiguration, preselected []string) ([]string, error) {
		gotPreselected = preselected
		return []string{"picked"}, nil
	})

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ProfilesFromArg: []string{ProfileFlagSelectValue},
	}
	atmosConfig, err := LoadConfig(&configAndStacksInfo)
	require.NoError(t, err)

	assert.Empty(t, gotPreselected, "bare --profile alone has no explicit names to preselect")
	assert.Equal(t, "Debug", atmosConfig.Logs.Level,
		"the profile the selector resolved to ('picked') should have actually loaded")
	assert.Equal(t, []string{"picked"}, configAndStacksInfo.ProfilesFromArg,
		"ProfilesFromArg should be replaced with the resolved (non-sentinel) profile names")
}

// TestLoadConfig_BareProfileSentinel_NilSelectorReturnsClearError verifies the full LoadConfig
// path surfaces ErrProfileSelectionUnavailable -- not a raw pflag parse error (fixed in the flag-
// parsing layer separately) and not a confusing "profile '__SELECT__' not found" error -- when no
// selector is registered (e.g. a caller that imports pkg/config directly without pkg/flags).
func TestLoadConfig_BareProfileSentinel_NilSelectorReturnsClearError(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	require.NoError(t, os.Unsetenv("ATMOS_PROFILE"))

	viper.Reset()
	t.Cleanup(viper.Reset)

	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(`base_path: .`), 0o644))

	withTestProfileSelector(t, nil)

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ProfilesFromArg: []string{ProfileFlagSelectValue},
	}
	_, err := LoadConfig(&configAndStacksInfo)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrProfileSelectionUnavailable)
	assert.NotContains(t, err.Error(), "flag needs an argument",
		"must not resurface the old raw pflag parse error")
	assert.NotContains(t, err.Error(), "'__SELECT__' not found",
		"must not surface a confusing 'profile not found' error naming the sentinel")
}
