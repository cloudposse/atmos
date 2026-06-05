package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDefaultArgs_MergedSettingsAndProfileOverride validates the data the default-args
// injector consumes: the fully merged settings map (what load.go stores as
// AtmosConfiguration.RawConfig via viper.AllSettings) exposes `<command>.args` and a profile
// overrides those args. This is the mechanism that makes per-command default args
// profile-compatible.
func TestDefaultArgs_MergedSettingsAndProfileOverride(t *testing.T) {
	tmpDir := t.TempDir()
	cleanup := withTestXDGConfigHome(t, tmpDir)
	t.Cleanup(cleanup)

	// Profile flips describe.args to enable function processing.
	profileDir := filepath.Join(tmpDir, "profiles", "funcson")
	require.NoError(t, os.MkdirAll(profileDir, 0o755))
	profileYAML := "describe:\n  args:\n    - --process-functions=true\n"
	require.NoError(t, os.WriteFile(filepath.Join(profileDir, "settings.yaml"), []byte(profileYAML), 0o644))

	baseYAML := `
args:
  - --identity=false
describe:
  args:
    - --process-functions=false
`

	describeArgs := func(raw map[string]any) []string {
		describe, _ := raw["describe"].(map[string]any)
		out := []string{}
		if list, ok := describe["args"].([]any); ok {
			for _, e := range list {
				out = append(out, e.(string))
			}
		}
		return out
	}

	t.Run("merged settings expose global and per-command args", func(t *testing.T) {
		v := viper.New()
		v.SetConfigType("yaml")
		require.NoError(t, v.MergeConfig(strings.NewReader(baseYAML)))

		raw := v.AllSettings()
		assert.Equal(t, []any{"--identity=false"}, raw["args"], "global top-level args")
		assert.Equal(t, []string{"--process-functions=false"}, describeArgs(raw))
	})

	t.Run("profile overrides per-command args", func(t *testing.T) {
		v := viper.New()
		v.SetConfigType("yaml")
		require.NoError(t, v.MergeConfig(strings.NewReader(baseYAML)))

		atmosConfig := &schema.AtmosConfiguration{
			CliConfigPath: tmpDir,
			Profiles:      schema.ProfilesConfig{BasePath: "profiles"},
		}
		require.NoError(t, loadProfiles(v, []string{"funcson"}, atmosConfig))

		assert.Equal(t, []string{"--process-functions=true"}, describeArgs(v.AllSettings()),
			"profile describe.args must override the base")
	})
}
