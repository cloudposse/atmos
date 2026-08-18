package flags

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// isolatedAtmosConfig returns an *schema.AtmosConfiguration pointed at fresh, empty temp
// directories for both project-local profile discovery (CliConfigPath) and XDG user profile
// discovery (ATMOS_XDG_CONFIG_HOME), so profile.NewProfileManager().ListProfiles() deterministically
// discovers zero profiles without ever touching the real developer machine's project or
// $HOME/.config directories.
func isolatedAtmosConfig(t *testing.T) *schema.AtmosConfiguration {
	t.Helper()

	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())

	return &schema.AtmosConfiguration{
		CliConfigPath: t.TempDir(),
	}
}

// TestSelectProfilesInteractively_NotInteractive follows the existing test pattern in
// interactive_test.go (e.g. TestPromptForMultipleValues) of forcing the non-interactive gate via
// viper.Set("interactive", false) rather than attempting to drive a real huh form without a TTY.
func TestSelectProfilesInteractively_NotInteractive(t *testing.T) {
	originalInteractive := viper.GetBool("interactive")
	t.Cleanup(func() {
		viper.Set("interactive", originalInteractive)
	})

	t.Run("zero discovered profiles", func(t *testing.T) {
		viper.Set("interactive", false)

		atmosConfig := isolatedAtmosConfig(t)
		_, err := selectProfilesInteractively(atmosConfig, nil)

		require.Error(t, err)
		// isInteractive() is checked before the empty-options check inside
		// PromptForMultipleValues, so this is the error surfaced regardless of how many
		// profiles were discovered.
		assert.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable)
	})

	t.Run("with preselected names, still gated by non-interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		atmosConfig := isolatedAtmosConfig(t)
		_, err := selectProfilesInteractively(atmosConfig, []string{"foo"})

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable)
	})
}

// TestMergeProfileSelections tests the pure union logic used to guarantee that profile names
// explicitly typed by the user (e.g. `--profile foo --profile`) always survive the interactive
// picker, regardless of what the user does with the multi-select form.
func TestMergeProfileSelections(t *testing.T) {
	tests := []struct {
		name        string
		preselected []string
		selected    []string
		want        []string
	}{
		{
			name:        "no preselected, some selected",
			preselected: nil,
			selected:    []string{"a", "b"},
			want:        []string{"a", "b"},
		},
		{
			name:        "preselected not present in selected is still included",
			preselected: []string{"foo"},
			selected:    []string{"a", "b"},
			want:        []string{"foo", "a", "b"},
		},
		{
			name:        "preselected already present in selected is not duplicated",
			preselected: []string{"a"},
			selected:    []string{"a", "b"},
			want:        []string{"a", "b"},
		},
		{
			name:        "both empty",
			preselected: nil,
			selected:    nil,
			want:        []string{},
		},
		{
			name:        "user deselected everything, preselected still survives",
			preselected: []string{"foo", "bar"},
			selected:    nil,
			want:        []string{"foo", "bar"},
		},
		{
			name:        "duplicates within selected are collapsed",
			preselected: nil,
			selected:    []string{"a", "a", "b"},
			want:        []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeProfileSelections(tt.preselected, tt.selected)
			assert.Equal(t, tt.want, got)
			if len(tt.want) > 0 {
				assert.Equal(t, tt.want[0], got[0], "first element must match for order guarantees")
				assert.Equal(t, tt.want[len(tt.want)-1], got[len(got)-1], "last element must match for order guarantees")
			}
		})
	}
}

// selectProfilesInteractivelyIsProfileSelector is a compile-time guard: selectProfilesInteractively
// must remain assignable to cfg.ProfileSelector, the dependency-injection seam this package's
// init() registers it against via cfg.SetProfileSelector(). If the function signature ever drifts
// out of sync with the pkg/config hook, this line fails to compile.
var _ cfg.ProfileSelector = selectProfilesInteractively

// TestProfileSelector_RegisteredByPackageInit verifies that importing this package (as every
// cmd/* package does transitively) actually runs the init() in profile_selector.go and leaves
// selectProfilesInteractively wired in as the real, non-nil implementation -- the functional
// proof that the dependency-injection seam is connected, distinct from the compile-time type
// guard above. End-to-end behavior (discovery, prompting, merge semantics) is covered by the
// other tests in this file and by pkg/config's own fake-selector tests.
//
// Runs against an isolated temp directory (own CWD, own atmos.yaml, own XDG/git-root overrides)
// so it never touches this repository's real atmos.yaml, profiles/ directory, or the developer
// machine's real $HOME/.config/atmos/profiles.
func TestProfileSelector_RegisteredByPackageInit(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("interactive", false)

	tempDir := t.TempDir()
	t.Chdir(tempDir)
	t.Setenv("TEST_GIT_ROOT", tempDir)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", t.TempDir())
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte("base_path: .\n"), 0o644))

	_, err := cfg.LoadConfig(&schema.ConfigAndStacksInfo{
		ProfilesFromArg: []string{cfg.ProfileFlagSelectValue},
	})

	require.Error(t, err)
	// A registered-but-gated selector returns ErrInteractiveModeNotAvailable. A nil selector
	// (init() never ran / never wired up) would instead return ErrProfileSelectionUnavailable --
	// so seeing this specific error is proof the real selectProfilesInteractively is registered
	// and was actually invoked.
	assert.ErrorIs(t, err, errUtils.ErrInteractiveModeNotAvailable)
}
