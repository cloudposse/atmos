package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestViperBindEnv_PluginCache tests that plugin cache env vars are properly bound
// via Viper's bindEnv in setEnv() and populated during Unmarshal.
// This is the correct pattern per pkg/flags guidelines - not direct os.Getenv.
func TestViperBindEnv_PluginCache(t *testing.T) {
	tests := []struct {
		name                   string
		envPluginCache         string
		envPluginCacheDir      string
		expectedPluginCache    bool
		expectedPluginCacheDir string
	}{
		{
			name:                   "plugin cache enabled via env var",
			envPluginCache:         "true",
			envPluginCacheDir:      "",
			expectedPluginCache:    true,
			expectedPluginCacheDir: "",
		},
		{
			name:                   "plugin cache disabled via env var",
			envPluginCache:         "false",
			envPluginCacheDir:      "",
			expectedPluginCache:    false,
			expectedPluginCacheDir: "",
		},
		{
			name:                   "plugin cache with custom dir",
			envPluginCache:         "",
			envPluginCacheDir:      "/custom/cache/path",
			expectedPluginCache:    true, // Default is true.
			expectedPluginCacheDir: "/custom/cache/path",
		},
		{
			name:                   "both env vars set",
			envPluginCache:         "true",
			envPluginCacheDir:      "/my/cache",
			expectedPluginCache:    true,
			expectedPluginCacheDir: "/my/cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test env vars using t.Setenv for automatic cleanup.
			if tt.envPluginCache != "" {
				t.Setenv("ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE", tt.envPluginCache)
			}
			if tt.envPluginCacheDir != "" {
				t.Setenv("ATMOS_COMPONENTS_TERRAFORM_PLUGIN_CACHE_DIR", tt.envPluginCacheDir)
			}

			// Use LoadConfig which handles Viper env bindings properly.
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)

			assert.Equal(t, tt.expectedPluginCache, config.Components.Terraform.PluginCache,
				"PluginCache mismatch")
			assert.Equal(t, tt.expectedPluginCacheDir, config.Components.Terraform.PluginCacheDir,
				"PluginCacheDir mismatch")
		})
	}
}

func TestDefaultConfig_PluginCache(t *testing.T) {
	// Verify that the default config has plugin cache enabled.
	assert.True(t, defaultCliConfig.Components.Terraform.PluginCache)
	assert.Equal(t, "", defaultCliConfig.Components.Terraform.PluginCacheDir)
}
