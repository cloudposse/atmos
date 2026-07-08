package cache

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// useTempCacheHome points the XDG cache root at a temp dir for the test.
func useTempCacheHome(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
}

func TestResolveConfig_Defaults(t *testing.T) {
	useTempCacheHome(t)

	cfg, err := ResolveConfig(nil)
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, autoOff, cfg.Auto)
	assert.Equal(t, compressionGzip, cfg.Compression)
	assert.NotEmpty(t, cfg.Root)
	// Default key + restore key derive from the namespace prefix.
	assert.Contains(t, cfg.Key, defaultKeyPrefix)
	require.Len(t, cfg.RestoreKeys, 1)
	assert.True(t, len(cfg.RestoreKeys[0]) < len(cfg.Key))
}

func TestResolveConfig_AutoModes(t *testing.T) {
	useTempCacheHome(t)

	cases := []struct {
		auto        string
		wantRestore bool
		wantSave    bool
	}{
		{autoOff, false, false},
		{autoRestore, true, false},
		{autoSave, false, true},
		{autoBoth, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.auto, func(t *testing.T) {
			ac := &schema.AtmosConfiguration{}
			ac.CI.Cache.Enabled = true
			ac.CI.Cache.Auto = tc.auto
			cfg, err := ResolveConfig(ac)
			require.NoError(t, err)
			assert.Equal(t, tc.wantRestore, cfg.AutoRestoreEnabled())
			assert.Equal(t, tc.wantSave, cfg.AutoSaveEnabled())
		})
	}
}

func TestResolveConfig_ExplicitKeyTemplate(t *testing.T) {
	useTempCacheHome(t)

	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.Key = "my-{{.OS}}-cache"
	ac.CI.Cache.RestoreKeys = []string{"my-{{.OS}}-"}
	cfg, err := ResolveConfig(ac)
	require.NoError(t, err)
	assert.Contains(t, cfg.Key, "my-")
	assert.Contains(t, cfg.Key, "-cache")
	// Explicit key means no auto-added default restore key beyond what we set.
	require.Len(t, cfg.RestoreKeys, 1)
	assert.True(t, len(cfg.RestoreKeys[0]) < len(cfg.Key))
}

func TestResolveConfig_RootOverride(t *testing.T) {
	useTempCacheHome(t)
	override := t.TempDir()

	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.Root = override
	cfg, err := ResolveConfig(ac)
	require.NoError(t, err)

	abs, _ := filepath.Abs(override)
	assert.Equal(t, abs, cfg.Root)
}

func TestNormalizeIncludes(t *testing.T) {
	assert.Nil(t, normalizeIncludes(nil))
	assert.Nil(t, normalizeIncludes([]string{}))

	got := normalizeIncludes([]string{"", ".", "a", filepath.FromSlash("./b/")})
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestResolveConfig_WithIncludes(t *testing.T) {
	useTempCacheHome(t)
	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.Paths = []string{"toolchain", "."}
	cfg, err := ResolveConfig(ac)
	require.NoError(t, err)
	assert.Equal(t, []string{"toolchain"}, cfg.Includes)
}

func TestResolveConfig_AllowUnsafeAuthCache_Default(t *testing.T) {
	useTempCacheHome(t)

	cfg, err := ResolveConfig(nil)
	require.NoError(t, err)
	assert.False(t, cfg.AllowUnsafeAuthCache)
}

func TestResolveConfig_AllowUnsafeAuthCache_Explicit(t *testing.T) {
	useTempCacheHome(t)

	ac := &schema.AtmosConfiguration{}
	ac.CI.Cache.AllowUnsafeAuthCache = true
	cfg, err := ResolveConfig(ac)
	require.NoError(t, err)
	assert.True(t, cfg.AllowUnsafeAuthCache)
}

func TestDefaultLockfilePath(t *testing.T) {
	root := t.TempDir()

	t.Run("explicit relative lockfile resolves to absolute", func(t *testing.T) {
		ac := &schema.AtmosConfiguration{}
		ac.Toolchain.LockFile = filepath.Join("sub", "toolchain.lock.yaml")
		got := defaultLockfilePath(ac, root)
		assert.True(t, filepath.IsAbs(got))
		assert.True(t, strings.HasSuffix(filepath.ToSlash(got), "sub/toolchain.lock.yaml"))
	})

	t.Run("nil config falls back to root toolchain path", func(t *testing.T) {
		got := defaultLockfilePath(nil, root)
		assert.Equal(t, filepath.Join(root, "toolchain", toolchainLockFilename), got)
	})

	t.Run("empty lockfile falls back to root toolchain path", func(t *testing.T) {
		got := defaultLockfilePath(&schema.AtmosConfiguration{}, root)
		assert.Equal(t, filepath.Join(root, "toolchain", toolchainLockFilename), got)
	})
}

func TestConfig_Validate(t *testing.T) {
	// Missing key fails.
	require.ErrorIs(t, (&Config{Root: "/x"}).validate(), errUtils.ErrCacheKeyRequired)
	// Missing root fails.
	require.ErrorIs(t, (&Config{Key: "k"}).validate(), errUtils.ErrCacheInvalidArgs)
	// Both present succeeds.
	assert.NoError(t, (&Config{Key: "k", Root: "/x"}).validate())
}
