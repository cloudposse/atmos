package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessEnvVars_PluginCache(t *testing.T) {
	tests := []struct {
		name                   string
		envPluginCache         string
		envPluginCacheDir      string
		expectedPluginCache    bool
		expectedPluginCacheDir string
		expectError            bool
	}{
		{
			name:                   "plugin cache enabled via env var",
			envPluginCache:         "true",
			envPluginCacheDir:      "",
			expectedPluginCache:    true,
			expectedPluginCacheDir: "",
			expectError:            false,
		},
		{
			name:                   "plugin cache disabled via env var",
			envPluginCache:         "false",
			envPluginCacheDir:      "",
			expectedPluginCache:    false,
			expectedPluginCacheDir: "",
			expectError:            false,
		},
		{
			name:                   "plugin cache with custom dir",
			envPluginCache:         "",
			envPluginCacheDir:      "/custom/cache/path",
			expectedPluginCache:    true, // Default is true.
			expectedPluginCacheDir: "/custom/cache/path",
			expectError:            false,
		},
		{
			name:                   "invalid boolean for plugin cache",
			envPluginCache:         "not-a-boolean",
			envPluginCacheDir:      "",
			expectedPluginCache:    true, // Won't be set due to error.
			expectedPluginCacheDir: "",
			expectError:            true,
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

			// Create default config.
			config := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						PluginCache:    true, // Default value.
						PluginCacheDir: "",
					},
				},
			}

			// Call processEnvVars.
			err := processEnvVars(config)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedPluginCache, config.Components.Terraform.PluginCache)
			assert.Equal(t, tt.expectedPluginCacheDir, config.Components.Terraform.PluginCacheDir)
		})
	}
}

func TestDefaultConfig_PluginCache(t *testing.T) {
	// Verify that the default config has plugin cache enabled.
	assert.True(t, defaultCliConfig.Components.Terraform.PluginCache)
	assert.Equal(t, "", defaultCliConfig.Components.Terraform.PluginCacheDir)
}
