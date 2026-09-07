package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolve(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	configuredCacheDir := filepath.Join(t.TempDir(), "configured-cache")
	userCacheDir := filepath.Join(t.TempDir(), "user-cache")

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
			terraform: schema.Terraform{PluginCache: true, PluginCacheDir: configuredCacheDir},
			wantDir:   configuredCacheDir,
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
			terraform:   schema.Terraform{PluginCache: true, PluginCacheDir: configuredCacheDir},
			override:    userCacheDir,
			overrideSet: true,
			wantDir:     userCacheDir,
		},
		{
			name:      "disabled cache stays disabled",
			terraform: schema.Terraform{PluginCache: false},
		},
		{
			name:        "invalid explicit override falls back to automatic cache",
			terraform:   schema.Terraform{PluginCache: true, PluginCacheDir: configuredCacheDir},
			override:    "/",
			overrideSet: true,
			wantDir:     configuredCacheDir,
			automatic:   true,
		},
		{
			name:        "empty explicit override falls back to automatic cache",
			terraform:   schema.Terraform{PluginCache: true, PluginCacheDir: configuredCacheDir},
			override:    "",
			overrideSet: true,
			wantDir:     configuredCacheDir,
			automatic:   true,
		},
		{
			name:        "relative explicit override falls back to automatic cache",
			terraform:   schema.Terraform{PluginCache: true, PluginCacheDir: configuredCacheDir},
			override:    filepath.Join("relative", "cache"),
			overrideSet: true,
			wantDir:     configuredCacheDir,
			automatic:   true,
		},
		{
			name:      "relative configured cache directory disables cache",
			terraform: schema.Terraform{PluginCache: true, PluginCacheDir: filepath.Join("relative", "cache")},
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

	cache := Resolve(&schema.AtmosConfiguration{Components: schema.Components{Terraform: schema.Terraform{PluginCache: true, PluginCacheDir: configuredCacheDir}}}, "", false)
	require.NotEmpty(t, cache.InitLockPath())
	assert.Equal(t, cache.InitLockPath(), cache.InitLockPath())
	assert.Empty(t, (Cache{}).InitLockPath())
	assert.Empty(t, Resolve(nil, "", false).Directory)
}

func TestResolveXDGCacheFallbacks(t *testing.T) {
	original := getXDGCacheDir
	t.Cleanup(func() { getXDGCacheDir = original })

	config := &schema.AtmosConfiguration{Components: schema.Components{Terraform: schema.Terraform{PluginCache: true}}}
	tests := []struct {
		name   string
		lookup func(string, os.FileMode) (string, error)
	}{
		{
			name: "XDG cache directory lookup fails",
			lookup: func(string, os.FileMode) (string, error) {
				return "", errors.New("unavailable")
			},
		},
		{
			name: "XDG cache directory lookup returns empty path",
			lookup: func(string, os.FileMode) (string, error) {
				return "", nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getXDGCacheDir = tt.lookup
			assert.Empty(t, Resolve(config, "", false).Directory)
		})
	}
}

func TestCacheInitLockPathUsesDirectoryWhenAbsolutePathFails(t *testing.T) {
	original := absolutePath
	t.Cleanup(func() { absolutePath = original })
	absolutePath = func(string) (string, error) {
		return "", errors.New("unavailable")
	}

	cache := Cache{Directory: "relative/cache"}
	firstLockPath := cache.InitLockPath()
	assert.NotEmpty(t, firstLockPath)
	assert.Equal(t, firstLockPath, cache.InitLockPath())
	assert.NotEqual(t, firstLockPath, Cache{Directory: "other/cache"}.InitLockPath())
}

func TestCacheInitLockPathForWorkdirUsesRelativeCacheDirectory(t *testing.T) {
	cache := Cache{Directory: "relative/cache"}
	workdir := t.TempDir()

	assert.Equal(t, cache.InitLockPathForWorkdir(workdir), cache.InitLockPathForWorkdir(workdir))
	assert.NotEqual(t, cache.InitLockPathForWorkdir(workdir), cache.InitLockPathForWorkdir(t.TempDir()))
}
