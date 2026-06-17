package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/schema"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
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

// TestViperBindEnv_TerraformRegistryCache tests that registry cache env vars are
// bound via Viper and populate the nested components.terraform.cache config.
func TestViperBindEnv_TerraformRegistryCache(t *testing.T) {
	customLocation := filepath.Join(t.TempDir(), "terraform-registry-cache")

	tests := []struct {
		name             string
		envEnabled       string
		envLocation      string
		expectedEnabled  bool
		expectedLocation string
	}{
		{
			name:            "registry cache enabled via env var",
			envEnabled:      "true",
			expectedEnabled: true,
		},
		{
			name:            "registry cache disabled via env var",
			envEnabled:      "false",
			expectedEnabled: false,
		},
		{
			name:             "registry cache with custom location",
			envLocation:      customLocation,
			expectedEnabled:  false,
			expectedLocation: customLocation,
		},
		{
			name:             "registry cache enabled with custom location",
			envEnabled:       "true",
			envLocation:      customLocation,
			expectedEnabled:  true,
			expectedLocation: customLocation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_COMPONENTS_TERRAFORM_CACHE_ENABLED", "false")
			t.Setenv("ATMOS_COMPONENTS_TERRAFORM_CACHE_LOCATION", "")

			if tt.envEnabled != "" {
				t.Setenv("ATMOS_COMPONENTS_TERRAFORM_CACHE_ENABLED", tt.envEnabled)
			}
			if tt.envLocation != "" {
				t.Setenv("ATMOS_COMPONENTS_TERRAFORM_CACHE_LOCATION", tt.envLocation)
			}

			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)

			require.NotNil(t, config.Components.Terraform.Cache)
			assert.Equal(t, tt.expectedEnabled, config.Components.Terraform.Cache.Enabled)
			assert.Equal(t, tt.expectedLocation, config.Components.Terraform.Cache.Location)
		})
	}
}

func TestViperBindEnv_TerraformRegistryCacheLocationExpands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	homedir.Reset()
	t.Cleanup(homedir.Reset)

	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_CACHE_LOCATION", "~/terraform-registry-cache")

	config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
	require.NoError(t, err)
	require.NotNil(t, config.Components.Terraform.Cache)

	root, err := tfcache.ResolveRoot(&config)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "terraform-registry-cache"), root)
}

func TestDefaultConfig_PluginCache(t *testing.T) {
	// Verify that the default config has plugin cache enabled.
	assert.True(t, defaultCliConfig.Components.Terraform.PluginCache)
	assert.Equal(t, "", defaultCliConfig.Components.Terraform.PluginCacheDir)
}
