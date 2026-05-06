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
		profiles        []string
		wantBasePath    string
		wantConfigFiles []string
		wantConfigDirs  []string
		wantProfiles    []string
	}{
		{
			name:            "all global flags empty returns zero-value struct",
			wantBasePath:    "",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{},
			wantProfiles:    []string{},
		},
		{
			name:            "base-path set propagates to AtmosBasePath",
			basePath:        "/custom/base",
			wantBasePath:    "/custom/base",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{},
			wantProfiles:    []string{},
		},
		{
			name:            "config set propagates to AtmosConfigFilesFromArg",
			configFiles:     []string{"/a.yaml", "/b.yaml"},
			wantBasePath:    "",
			wantConfigFiles: []string{"/a.yaml", "/b.yaml"},
			wantConfigDirs:  []string{},
			wantProfiles:    []string{},
		},
		{
			name:            "config-path set propagates to AtmosConfigDirsFromArg",
			configDirs:      []string{"/dir1", "/dir2"},
			wantBasePath:    "",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{"/dir1", "/dir2"},
			wantProfiles:    []string{},
		},
		{
			// Regression guard: without propagating --profile, the
			// profile-fallback re-exec path loses its picked profile
			// and auth commands see an empty identity list.
			name:            "profile set propagates to ProfilesFromArg",
			profiles:        []string{"managers"},
			wantBasePath:    "",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{},
			wantProfiles:    []string{"managers"},
		},
		{
			name:            "multiple profiles propagate in order",
			profiles:        []string{"prod", "us-east-1"},
			wantBasePath:    "",
			wantConfigFiles: []string{},
			wantConfigDirs:  []string{},
			wantProfiles:    []string{"prod", "us-east-1"},
		},
		{
			name:            "all flags set propagate together",
			basePath:        "/root",
			configFiles:     []string{"/root/atmos.yaml"},
			configDirs:      []string{"/root/configs"},
			profiles:        []string{"developer"},
			wantBasePath:    "/root",
			wantConfigFiles: []string{"/root/atmos.yaml"},
			wantConfigDirs:  []string{"/root/configs"},
			wantProfiles:    []string{"developer"},
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
			origProfile := v.Get("profile")
			t.Cleanup(func() {
				v.Set("base-path", origBasePath)
				v.Set("config", origConfig)
				v.Set("config-path", origConfigPath)
				v.Set("profile", origProfile)
			})

			v.Set("base-path", tt.basePath)
			v.Set("config", tt.configFiles)
			v.Set("config-path", tt.configDirs)
			v.Set("profile", tt.profiles)

			cmd := &cobra.Command{Use: "test"}
			info := newAuthConfigAndStacksInfo(cmd)

			assert.Equal(t, tt.wantBasePath, info.AtmosBasePath)
			// Compare slices with ElementsMatch so nil and []string{} both satisfy empty expectations.
			assert.ElementsMatch(t, tt.wantConfigFiles, info.AtmosConfigFilesFromArg)
			assert.ElementsMatch(t, tt.wantConfigDirs, info.AtmosConfigDirsFromArg)
			// Order matters for profiles (rightmost wins in profile merging).
			assert.Equal(t, tt.wantProfiles, normalizeStringSlice(info.ProfilesFromArg))
		})
	}
}

// normalizeStringSlice coerces a nil slice to []string{} so assertions stay
// symmetric regardless of whether the code under test returned nil or empty.
func normalizeStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
