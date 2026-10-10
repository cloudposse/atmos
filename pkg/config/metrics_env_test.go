package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMetricsEnabledEnvironmentBinding(t *testing.T) {
	tests := []struct {
		name string
		env  string
		file string
		want *bool
	}{
		{name: "unset leaves the default", want: nil},
		{name: "env disables", env: "false", want: boolPointer(false)},
		{name: "env enables", env: "true", want: boolPointer(true)},
		{name: "env overrides the file", env: "false", file: "settings:\n  metrics:\n    enabled: true\n", want: boolPointer(false)},
		{name: "file applies without env", file: "settings:\n  metrics:\n    enabled: false\n", want: boolPointer(false)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte("base_path: ./\n"+test.file), 0o644))
			t.Chdir(dir)
			t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
			t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
			t.Setenv("ATMOS_SETTINGS_METRICS_ENABLED", test.env)
			if test.env == "" {
				require.NoError(t, os.Unsetenv("ATMOS_SETTINGS_METRICS_ENABLED"))
			}
			atmosConfig, err := InitCliConfig(schema.ConfigAndStacksInfo{}, false)
			require.NoError(t, err)
			if test.want == nil {
				assert.Nil(t, atmosConfig.Settings.Metrics.Enabled)
				return
			}
			require.NotNil(t, atmosConfig.Settings.Metrics.Enabled)
			assert.Equal(t, *test.want, *atmosConfig.Settings.Metrics.Enabled)
		})
	}
}

func boolPointer(value bool) *bool { return &value }
