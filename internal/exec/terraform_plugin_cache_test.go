package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsValidPluginCacheDir(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "valid path",
			path:     "/home/user/.cache/terraform",
			expected: true,
		},
		{
			name:     "valid relative path",
			path:     ".terraform-cache",
			expected: true,
		},
		{
			name:     "empty string is invalid",
			path:     "",
			expected: false,
		},
		{
			name:     "root path is invalid",
			path:     "/",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidPluginCacheDir(tt.path, "test")
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetValidUserPluginCacheDir(t *testing.T) {
	tests := []struct {
		name           string
		osEnvVar       string
		globalEnvDir   string
		expectedResult string
	}{
		{
			name:           "valid OS env var takes precedence",
			osEnvVar:       "/custom/cache",
			globalEnvDir:   "/global/cache",
			expectedResult: "/custom/cache",
		},
		{
			name:           "fallback to global env when OS env not set",
			osEnvVar:       "",
			globalEnvDir:   "/global/cache",
			expectedResult: "/global/cache",
		},
		{
			name:           "no env vars set returns empty",
			osEnvVar:       "",
			globalEnvDir:   "",
			expectedResult: "",
		},
		{
			name:           "empty OS env var is invalid, uses default",
			osEnvVar:       "SET_BUT_EMPTY", // Special marker to set env var to empty.
			globalEnvDir:   "/global/cache",
			expectedResult: "", // OS env is set but empty, so it's invalid.
		},
		{
			name:           "root OS env var is invalid, uses default",
			osEnvVar:       "/",
			globalEnvDir:   "/global/cache",
			expectedResult: "", // OS env is set to "/", so it's invalid.
		},
		{
			name:           "root global env var is invalid",
			osEnvVar:       "",
			globalEnvDir:   "/",
			expectedResult: "", // Global env is set to "/", so it's invalid.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up OS environment variable.
			if tt.osEnvVar == "SET_BUT_EMPTY" {
				t.Setenv("TF_PLUGIN_CACHE_DIR", "")
			} else if tt.osEnvVar != "" {
				t.Setenv("TF_PLUGIN_CACHE_DIR", tt.osEnvVar)
			}

			// Set up atmosConfig with global env.
			atmosConfig := &schema.AtmosConfiguration{
				Env: make(map[string]string),
			}
			if tt.globalEnvDir != "" {
				atmosConfig.Env["TF_PLUGIN_CACHE_DIR"] = tt.globalEnvDir
			}

			result := getValidUserPluginCacheDir(atmosConfig)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestConfigurePluginCache(t *testing.T) {
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
			pluginCacheDir:    "/custom/terraform/plugins",
			osEnvVar:          "",
			globalEnvDir:      "",
			expectEnvVars:     true,
			expectCustomDir:   true,
			expectedDirPrefix: "/custom/terraform/plugins",
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
			osEnvVar:        "/user/custom/cache",
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
			globalEnvDir:    "/global/custom/cache",
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
