package config

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExperimentalWarningPerFeatureAndExpiry(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	now := time.Unix(1_800_000_000, 0)
	require.NoError(t, SaveCache(CacheConfig{InstallationId: "existing", LastChecked: 123}))
	for _, feature := range []string{"toolchain", "devcontainer", "settings.yaml.key_delimiter", "edition"} {
		assert.True(t, claimExperimentalWarningAt(feature, now), feature)
		assert.False(t, claimExperimentalWarningAt(feature, now.Add(24*time.Hour-time.Second)), feature)
		assert.True(t, claimExperimentalWarningAt(feature, now.Add(24*time.Hour)), feature)
		assert.False(t, claimExperimentalWarningAt(feature, now.Add(24*time.Hour+time.Second)), feature)
	}
	cache, err := LoadCache()
	require.NoError(t, err)
	assert.Equal(t, "existing", cache.InstallationId)
	assert.Equal(t, int64(123), cache.LastChecked)
	assert.Equal(t, []ExperimentalWarningState{
		{Feature: "toolchain", LastShown: now.Add(24 * time.Hour).Unix()},
		{Feature: "devcontainer", LastShown: now.Add(24 * time.Hour).Unix()},
		{Feature: "settings.yaml.key_delimiter", LastShown: now.Add(24 * time.Hour).Unix()},
		{Feature: "edition", LastShown: now.Add(24 * time.Hour).Unix()},
	}, cache.ExperimentalWarnings)
}

func TestExperimentalWarningCacheFailures(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	path, err := GetCacheFilePath()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("invalid: [yaml"), 0o600))
	assert.True(t, ClaimExperimentalWarning("toolchain"))
	// An invalid cache directory is also nonfatal.
	t.Setenv("ATMOS_XDG_CACHE_HOME", path)
	assert.True(t, ClaimExperimentalWarning("toolchain"))
}

func TestExperimentalWarningSurvivesStaleSave(t *testing.T) {
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	now := time.Now()
	assert.True(t, claimExperimentalWarningAt("toolchain", now))
	stale, err := LoadCache()
	require.NoError(t, err)
	assert.True(t, claimExperimentalWarningAt("devcontainer", now))
	assert.True(t, claimExperimentalWarningAt("toolchain", now.Add(24*time.Hour)))
	require.NoError(t, SaveCache(stale))
	assert.False(t, claimExperimentalWarningAt("devcontainer", now))
	assert.False(t, claimExperimentalWarningAt("toolchain", now.Add(24*time.Hour)))
	require.NoError(t, UpdateCache(func(c *CacheConfig) { c.LastChecked = 456 }))
	assert.False(t, claimExperimentalWarningAt("devcontainer", now))
}

func TestExperimentalWarningConcurrentClaims(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Shared cache locking is best effort on Windows")
	}
	t.Setenv("ATMOS_XDG_CACHE_HOME", t.TempDir())
	var shown atomic.Int32
	var workers sync.WaitGroup
	now := time.Now()
	for range 10 {
		workers.Go(func() {
			if claimExperimentalWarningAt("toolchain", now) {
				shown.Add(1)
			}
		})
	}
	workers.Wait()
	assert.Equal(t, int32(1), shown.Load())
}
