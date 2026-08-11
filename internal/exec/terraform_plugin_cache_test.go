package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigurePluginCache(t *testing.T) {
	customCacheDir := filepath.Join(t.TempDir(), "custom", "terraform", "plugins")
	userCacheDir := filepath.Join(t.TempDir(), "user", "custom", "cache")
	globalCacheDir := filepath.Join(t.TempDir(), "global", "custom", "cache")

	tests := []struct {
		name              string
		pluginCache       bool
		pluginCacheDir    string
		osEnvVar          string
		globalEnvDir      string
		expectEnvVars     bool
		expectCustomDir   bool
		expectedDirPrefix string
	}{
		{
			name:            "caching enabled uses XDG default",
			pluginCache:     true,
			pluginCacheDir:  "",
			osEnvVar:        "",
			globalEnvDir:    "",
			expectEnvVars:   true,
			expectCustomDir: false,
		},
		{
			name:              "caching enabled with custom dir",
			pluginCache:       true,
			pluginCacheDir:    customCacheDir,
			osEnvVar:          "",
			globalEnvDir:      "",
			expectEnvVars:     true,
			expectCustomDir:   true,
			expectedDirPrefix: customCacheDir,
		},
		{
			name:            "caching disabled returns no env vars",
			pluginCache:     false,
			pluginCacheDir:  "",
			osEnvVar:        "",
			globalEnvDir:    "",
			expectEnvVars:   false,
			expectCustomDir: false,
		},
		{
			name:            "user OS env var takes precedence",
			pluginCache:     true,
			pluginCacheDir:  "",
			osEnvVar:        userCacheDir,
			globalEnvDir:    "",
			expectEnvVars:   false, // User manages their own cache.
			expectCustomDir: false,
		},
		{
			name:            "invalid root OS env var uses default",
			pluginCache:     true,
			pluginCacheDir:  "",
			osEnvVar:        "/",
			globalEnvDir:    "",
			expectEnvVars:   true, // Root is invalid, so we use our default.
			expectCustomDir: false,
		},
		{
			name:            "user global env var takes precedence",
			pluginCache:     true,
			pluginCacheDir:  "",
			osEnvVar:        "",
			globalEnvDir:    globalCacheDir,
			expectEnvVars:   false, // User manages their own cache.
			expectCustomDir: false,
		},
		{
			name:            "invalid root global env var uses default",
			pluginCache:     true,
			pluginCacheDir:  "",
			osEnvVar:        "",
			globalEnvDir:    "/",
			expectEnvVars:   true, // Root is invalid, so we use our default.
			expectCustomDir: false,
		},
		{
			name:            "empty OS env var is set but invalid",
			pluginCache:     true,
			pluginCacheDir:  "",
			osEnvVar:        "SET_BUT_EMPTY", // Special marker.
			globalEnvDir:    "",
			expectEnvVars:   true, // Empty is invalid, so we use our default.
			expectCustomDir: false,
		},
		{
			name:            "empty global env var is invalid",
			pluginCache:     true,
			pluginCacheDir:  "",
			osEnvVar:        "",
			globalEnvDir:    "SET_BUT_EMPTY", // Special marker.
			expectEnvVars:   true,            // Empty is invalid, so we use our default.
			expectCustomDir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up temp XDG cache for tests that need it.
			tmpDir := t.TempDir()
			t.Setenv("XDG_CACHE_HOME", tmpDir)

			// Set up OS environment variable.
			if tt.osEnvVar == "SET_BUT_EMPTY" {
				t.Setenv("TF_PLUGIN_CACHE_DIR", "")
			} else if tt.osEnvVar != "" {
				t.Setenv("TF_PLUGIN_CACHE_DIR", tt.osEnvVar)
			}

			// Set up atmosConfig.
			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						PluginCache:    tt.pluginCache,
						PluginCacheDir: tt.pluginCacheDir,
					},
				},
				Env: make(map[string]string),
			}
			if tt.globalEnvDir == "SET_BUT_EMPTY" {
				atmosConfig.Env["TF_PLUGIN_CACHE_DIR"] = ""
			} else if tt.globalEnvDir != "" {
				atmosConfig.Env["TF_PLUGIN_CACHE_DIR"] = tt.globalEnvDir
			}

			result := configurePluginCache(atmosConfig)

			if !tt.expectEnvVars {
				assert.Empty(t, result, "Expected no env vars")
				return
			}

			assert.Len(t, result, 2, "Expected 2 env vars")
			assert.Contains(t, result[0], "TF_PLUGIN_CACHE_DIR=")
			assert.Equal(t, "TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=true", result[1])

			if tt.expectCustomDir {
				assert.Contains(t, result[0], tt.expectedDirPrefix)
			}
		})
	}
}

func TestDisableTerraformPluginCacheForExecutionFiltersConfiguredCache(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				PluginCache:    true,
				PluginCacheDir: "/configured/cache",
			},
		},
		Env: map[string]string{
			terraformPluginCacheDirEnv:              "/global/cache",
			terraformPluginCacheMayBreakLockFileEnv: "true",
			"KEEP_ME":                               "1",
		},
	}
	info := &schema.ConfigAndStacksInfo{
		DisablePluginCache: true,
		ComponentEnvSection: schema.AtmosSectionMapType{
			terraformPluginCacheDirEnv:              "/component/cache",
			terraformPluginCacheMayBreakLockFileEnv: "true",
			"KEEP_COMPONENT":                        "1",
		},
		ComponentEnvList: []string{
			terraformPluginCacheDirEnv + "=/component/list/cache",
			terraformPluginCacheMayBreakLockFileEnv + "=true",
			"KEEP_LIST=1",
		},
		SanitizedEnv: []string{
			terraformPluginCacheDirEnv + "=/sanitized/cache",
			terraformPluginCacheMayBreakLockFileEnv + "=true",
			"KEEP_SANITIZED=1",
		},
	}

	disableTerraformPluginCacheForExecution(atmosConfig, info)

	assert.False(t, atmosConfig.Components.Terraform.PluginCache)
	assert.Empty(t, atmosConfig.Components.Terraform.PluginCacheDir)
	assert.NotContains(t, atmosConfig.Env, terraformPluginCacheDirEnv)
	assert.NotContains(t, atmosConfig.Env, terraformPluginCacheMayBreakLockFileEnv)
	assert.Contains(t, atmosConfig.Env, "KEEP_ME")
	assert.NotContains(t, info.ComponentEnvSection, terraformPluginCacheDirEnv)
	assert.NotContains(t, info.ComponentEnvSection, terraformPluginCacheMayBreakLockFileEnv)
	assert.Contains(t, info.ComponentEnvSection, "KEEP_COMPONENT")
	assert.False(t, envListContainsKey(info.ComponentEnvList, terraformPluginCacheDirEnv))
	assert.False(t, envListContainsKey(info.ComponentEnvList, terraformPluginCacheMayBreakLockFileEnv))
	assert.True(t, envListContainsKey(info.ComponentEnvList, "KEEP_LIST"))
	assert.False(t, envListContainsKey(info.SanitizedEnv, terraformPluginCacheDirEnv))
	assert.False(t, envListContainsKey(info.SanitizedEnv, terraformPluginCacheMayBreakLockFileEnv))
	assert.True(t, envListContainsKey(info.SanitizedEnv, "KEEP_SANITIZED"))
}

func TestDisableTerraformPluginCacheForExecutionFiltersProcessEnvWhenUnsanitized(t *testing.T) {
	t.Setenv(terraformPluginCacheDirEnv, "/process/cache")
	t.Setenv(terraformPluginCacheMayBreakLockFileEnv, "true")
	t.Setenv("ATMOS_TEST_KEEP_ENV", "1")

	atmosConfig := &schema.AtmosConfiguration{Env: map[string]string{}}
	info := &schema.ConfigAndStacksInfo{DisablePluginCache: true}

	disableTerraformPluginCacheForExecution(atmosConfig, info)

	assert.False(t, envListContainsKey(info.SanitizedEnv, terraformPluginCacheDirEnv))
	assert.False(t, envListContainsKey(info.SanitizedEnv, terraformPluginCacheMayBreakLockFileEnv))
	assert.True(t, envListContainsKey(info.SanitizedEnv, "ATMOS_TEST_KEEP_ENV"))
}

func TestDisableTerraformPluginCacheForExecutionNoopWithoutFlag(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{PluginCache: true, PluginCacheDir: "/configured/cache"},
		},
		Env: map[string]string{terraformPluginCacheDirEnv: "/global/cache"},
	}
	info := &schema.ConfigAndStacksInfo{
		ComponentEnvList: []string{terraformPluginCacheDirEnv + "=/component/cache"},
		SanitizedEnv:     []string{terraformPluginCacheDirEnv + "=/sanitized/cache"},
	}

	disableTerraformPluginCacheForExecution(atmosConfig, info)

	assert.True(t, atmosConfig.Components.Terraform.PluginCache)
	assert.Equal(t, "/configured/cache", atmosConfig.Components.Terraform.PluginCacheDir)
	assert.Contains(t, atmosConfig.Env, terraformPluginCacheDirEnv)
	assert.True(t, envListContainsKey(info.ComponentEnvList, terraformPluginCacheDirEnv))
	assert.True(t, envListContainsKey(info.SanitizedEnv, terraformPluginCacheDirEnv))
}

func envListContainsKey(env []string, key string) bool {
	for _, entry := range env {
		if envKey(entry) == key {
			return true
		}
	}
	return false
}
