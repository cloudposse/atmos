package plugin

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolve(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	tests := []struct {
		name        string
		terraform   schema.Terraform
		override    string
		overrideSet bool
		wantDir     string
		automatic   bool
	}{
		{
			name:      "uses configured cache directory",
			terraform: schema.Terraform{PluginCache: true, PluginCacheDir: "/configured/cache"},
			wantDir:   "/configured/cache",
			automatic: true,
		},
		{
			name:      "uses XDG default",
			terraform: schema.Terraform{PluginCache: true},
			wantDir:   filepath.Join(cacheHome, "atmos", "terraform", "plugins"),
			automatic: true,
		},
		{
			name:        "explicit override wins",
			terraform:   schema.Terraform{PluginCache: true, PluginCacheDir: "/configured/cache"},
			override:    "/user/cache",
			overrideSet: true,
			wantDir:     "/user/cache",
		},
		{
			name:      "disabled cache stays disabled",
			terraform: schema.Terraform{PluginCache: false},
		},
		{
			name:        "invalid explicit override falls back to automatic cache",
			terraform:   schema.Terraform{PluginCache: true, PluginCacheDir: "/configured/cache"},
			override:    "/",
			overrideSet: true,
			wantDir:     "/configured/cache",
			automatic:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := Resolve(&schema.AtmosConfiguration{Components: schema.Components{Terraform: tt.terraform}}, tt.override, tt.overrideSet)
			assert.Equal(t, tt.wantDir, cache.Directory)
			assert.Equal(t, tt.automatic, cache.Automatic)
			if tt.automatic {
				assert.Equal(t, tt.wantDir, cache.Environment[CacheDirEnv])
				assert.Equal(t, "true", cache.Environment[CacheMayBreakLockFileEnv])
			} else {
				assert.Empty(t, cache.Environment)
			}
		})
	}

	cache := Resolve(&schema.AtmosConfiguration{Components: schema.Components{Terraform: schema.Terraform{PluginCache: true, PluginCacheDir: "/configured/cache"}}}, "", false)
	require.NotEmpty(t, cache.InitLockPath())
	assert.Equal(t, cache.InitLockPath(), cache.InitLockPath())
}
