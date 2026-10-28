package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.VersionCheckCI{Enabled: true, MinorThreshold: 10}

func TestCIVersionCheckConfig(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, enabled, threshold string
		wantEnabled                    bool
		wantThreshold                  int
	}{
		{name: "defaults", wantEnabled: true, wantThreshold: 10},
		{name: "yaml", yaml: "version:\n  check:\n    ci:\n      enabled: false\n      minor_threshold: 20", wantThreshold: 20},
		{name: "env overrides yaml", yaml: "version:\n  check:\n    ci:\n      enabled: true\n      minor_threshold: 20", enabled: "false", threshold: "15", wantThreshold: 15},
		{name: "env enables", enabled: "true", threshold: "3", wantEnabled: true, wantThreshold: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ATMOS_VERSION_CHECK_CI_ENABLED", tc.enabled)
			t.Setenv("ATMOS_VERSION_CHECK_CI_MINOR_THRESHOLD", tc.threshold)
			v := viper.New()
			setDefaultConfiguration(v)
			setEnv(v)
			v.SetConfigType("yaml")
			require.NoError(t, v.ReadConfig(strings.NewReader(tc.yaml)))
			var config schema.AtmosConfiguration
			require.NoError(t, v.Unmarshal(&config))
			assert.Equal(t, tc.wantEnabled, config.Version.Check.CI.Enabled)
			assert.Equal(t, tc.wantThreshold, config.Version.Check.CI.MinorThreshold)
		})
	}
}

func TestVersionCacheContextPreservesFields(t *testing.T) {
	defer withTestXDGHome(t, t.TempDir())()
	original := CacheConfig{InstallationId: "installation", TelemetryDisclosureShown: true, ExperimentalWarnings: []ExperimentalWarningState{{Feature: "test", LastShown: 42}}}
	require.NoError(t, SaveCache(original))
	require.NoError(t, UpdateCacheContext(context.Background(), func(c *CacheConfig) { c.LastChecked = 123 }))
	got, err := LoadCacheContext(context.Background())
	require.NoError(t, err)
	original.LastChecked = 123
	assert.Equal(t, original, got)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, UpdateCacheContext(ctx, func(c *CacheConfig) { c.LastChecked = 456 }))
	got, err = LoadCacheContext(context.Background())
	require.NoError(t, err)
	assert.EqualValues(t, 123, got.LastChecked)
}

func TestVersionCacheCanceledRead(t *testing.T) {
	defer withTestXDGHome(t, t.TempDir())()
	original := CacheConfig{LastChecked: 123, InstallationId: "installation"}
	require.NoError(t, SaveCache(original))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := LoadCacheContext(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, got)
	got, err = LoadCacheContext(t.Context())
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

func TestVersionCacheCanceledUpdatePreservesFile(t *testing.T) {
	defer withTestXDGHome(t, t.TempDir())()
	original := CacheConfig{LastChecked: 123, InstallationId: "installation", TelemetryDisclosureShown: true}
	require.NoError(t, SaveCache(original))
	path, err := GetCacheFilePath()
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	err = UpdateCacheContext(ctx, func(c *CacheConfig) {
		c.LastChecked = 456
		cancel()
	})
	require.ErrorIs(t, err, context.Canceled)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a canceled check must not advance the timestamp or rewrite the cache")
}

func TestVersionCacheInvalidTimestampPreservesFile(t *testing.T) {
	defer withTestXDGHome(t, t.TempDir())()
	path, err := GetCacheFilePath()
	require.NoError(t, err)
	content := []byte("last_checked: not-a-timestamp\ninstallation_id: installation\n")
	require.NoError(t, os.WriteFile(path, content, 0o600))
	err = UpdateCacheContext(t.Context(), func(*CacheConfig) {
		t.Fatal("invalid cache contents must not reach the update callback")
	})
	require.ErrorIs(t, err, errUtils.ErrCacheUnmarshal)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, after, "failed checks must preserve existing cache contents")
}

func TestVersionCacheInvalidDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	defer withTestXDGHome(t, path)()
	_, err := LoadCacheContext(t.Context())
	require.ErrorIs(t, err, errUtils.ErrCacheDir)
	err = UpdateCacheContext(t.Context(), func(*CacheConfig) {
		t.Fatal("an inaccessible cache directory must not reach the update callback")
	})
	require.ErrorIs(t, err, errUtils.ErrCacheDir)
}
