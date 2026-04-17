package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestNewAuthConfigAndStacksInfo(t *testing.T) {
	tests := []struct {
		name            string
		basePath        string
		configFiles     []string
		configDirs      []string
		wantBasePath    string
		wantConfigFiles []string
		wantConfigDirs  []string
	}{
		{
			name:            "all global flags empty returns zero-value struct",
			wantBasePath:    "",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{},
		},
		{
			name:            "base-path set propagates to AtmosBasePath",
			basePath:        "/custom/base",
			wantBasePath:    "/custom/base",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{},
		},
		{
			name:            "config set propagates to AtmosConfigFilesFromArg",
			configFiles:     []string{"/a.yaml", "/b.yaml"},
			wantBasePath:    "",
			wantConfigFiles: []string{"/a.yaml", "/b.yaml"},
			wantConfigDirs:  []string{},
		},
		{
			name:            "config-path set propagates to AtmosConfigDirsFromArg",
			configDirs:      []string{"/dir1", "/dir2"},
			wantBasePath:    "",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{"/dir1", "/dir2"},
		},
		{
			name:            "all three flags set propagate together",
			basePath:        "/root",
			configFiles:     []string{"/root/atmos.yaml"},
			configDirs:      []string{"/root/configs"},
			wantBasePath:    "/root",
			wantConfigFiles: []string{"/root/atmos.yaml"},
			wantConfigDirs:  []string{"/root/configs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)

			// Capture current values so we can restore them on cleanup — viper
			// is a global singleton, and leaving test values in place breaks
			// every downstream test that reads --config / --base-path.
			v := viper.GetViper()
			origBasePath := v.Get("base-path")
			origConfig := v.Get("config")
			origConfigPath := v.Get("config-path")
			t.Cleanup(func() {
				v.Set("base-path", origBasePath)
				v.Set("config", origConfig)
				v.Set("config-path", origConfigPath)
			})

			v.Set("base-path", tt.basePath)
			v.Set("config", tt.configFiles)
			v.Set("config-path", tt.configDirs)

			cmd := &cobra.Command{Use: "test"}
			info := newAuthConfigAndStacksInfo(cmd)

			assert.Equal(t, tt.wantBasePath, info.AtmosBasePath)
			// Compare slices with ElementsMatch so nil and []string{} both satisfy empty expectations.
			assert.ElementsMatch(t, tt.wantConfigFiles, info.AtmosConfigFilesFromArg)
			assert.ElementsMatch(t, tt.wantConfigDirs, info.AtmosConfigDirsFromArg)
		})
	}
}
