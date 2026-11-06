package config

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// CacheDirPermissions is the default permission for cache directory (read/write/execute for owner, read/execute for group and others).
	CacheDirPermissions  = 0o755
	cacheFilePermissions = 0o644
)

// CacheConfig holds persistent application state for version checks,
// telemetry preferences, and session-level warnings.
type CacheConfig struct {
	LastChecked                int64                      `mapstructure:"last_checked" yaml:"last_checked"`
	InstallationId             string                     `mapstructure:"installation_id" yaml:"installation_id"`
	TelemetryDisclosureShown   bool                       `mapstructure:"telemetry_disclosure_shown" yaml:"telemetry_disclosure_shown"`
	BrowserSessionWarningShown bool                       `mapstructure:"browser_session_warning_shown" yaml:"browser_session_warning_shown"`
	ExperimentalWarnings       []ExperimentalWarningState `mapstructure:"experimental_warnings" yaml:"experimental_warnings,omitempty"`
}

// ExperimentalWarningState records when an experimental feature was last shown.
// A sequence of records preserves feature names literally through Viper parsing.
type ExperimentalWarningState struct {
	Feature   string `mapstructure:"feature" yaml:"feature"`
	LastShown int64  `mapstructure:"last_shown" yaml:"last_shown"`
}

// GetCacheFilePath returns the filesystem path to the Atmos cache file.
// It respects ATMOS_XDG_CACHE_HOME and XDG_CACHE_HOME environment variables for cache directory location.
// Returns an error if xdg.GetXDGCacheDir fails or if the cache directory cannot be created.
func GetCacheFilePath() (string, error) {
	cacheDir, err := xdg.GetXDGCacheDir("", CacheDirPermissions)
	if err != nil {
		return "", errors.Join(errUtils.ErrCacheDir, err)
	}

	return filepath.Join(cacheDir, "cache.yaml"), nil
}

// getCacheFileLock returns a FileLock for the cache file.
func getCacheFileLock(cacheFile string) cache.FileLock {
	return cache.NewFileLock(cacheFile)
}

// LoadCache loads the cache configuration from the cache file.
// Uses platform-specific file locking to prevent concurrent read/write issues.
func LoadCache() (CacheConfig, error) {
	return loadCache(context.Background(), true)
}

// LoadCacheContext loads the cache with a cancellation-aware lock.
func LoadCacheContext(ctx context.Context) (CacheConfig, error) {
	if err := ctx.Err(); err != nil {
		return CacheConfig{}, err
	}
	return loadCache(ctx, false)
}

func loadCache(ctx context.Context, bestEffort bool) (CacheConfig, error) {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return CacheConfig{}, err
	}

	var cfg CacheConfig
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		// No file yet, return default.
		return cfg, nil
	}

	var readErr error
	lock := getCacheFileLock(cacheFile)
	withLock := lock.WithRLock
	if !bestEffort {
		withLock = func(fn func() error) error { return lock.WithLockContext(ctx, fn) }
	}
	lockErr := withLock(func() error {
		cfg, readErr = readCacheFile(cacheFile)
		return nil
	})

	// Lock errors are non-critical for cache.
	if lockErr != nil {
		if !bestEffort {
			return cfg, lockErr
		}
		log.Trace("Failed to acquire cache lock (non-critical)", "error", lockErr, "file", cacheFile)
		return cfg, nil
	}

	// On Windows, cache read errors are silently ignored because file locking
	// is a no-op and corrupted cache files should not block normal operation.
	if readErr != nil {
		if runtime.GOOS == "windows" {
			log.Trace("Cache read error ignored on Windows", "error", readErr, "file", cacheFile)
			return CacheConfig{}, nil
		}
		return CacheConfig{}, readErr
	}

	return cfg, nil
}

// SaveCache writes the provided cache configuration to the cache file atomically.
// The function acquires an exclusive lock to prevent concurrent writes and ensures
// data consistency across multiple processes.
//
// Parameters:
//   - cfg: The CacheConfig to save to disk.
//
// Returns an error if the cache file cannot be created or written.
// Callers can check for specific failure types using errors.Is() with the
// following sentinel errors:
//   - ErrCacheMarshal: Failed to marshal cache content to YAML
//   - ErrCacheWrite: Failed to write the cache file
func SaveCache(cfg CacheConfig) error {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return err
	}

	lock := getCacheFileLock(cacheFile)
	// Use file locking to prevent concurrent writes.
	return lock.WithLock(func() error {
		// A caller may have loaded its snapshot before another process claimed a
		// warning. Preserve the newest timestamps when saving that older snapshot.
		mergeExperimentalWarnings(cacheFile, &cfg)
		// Prepare the config data.
		data := map[string]interface{}{
			"last_checked":                  cfg.LastChecked,
			"installation_id":               cfg.InstallationId,
			"telemetry_disclosure_shown":    cfg.TelemetryDisclosureShown,
			"browser_session_warning_shown": cfg.BrowserSessionWarningShown,
		}

		if len(cfg.ExperimentalWarnings) > 0 {
			data["experimental_warnings"] = cfg.ExperimentalWarnings
		}

		// Marshal to YAML.
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(data); err != nil {
			return errors.Join(errUtils.ErrCacheMarshal, err)
		}

		// Write atomically using filesystem package.
		fs := filesystem.NewOSFileSystem()
		if err := fs.WriteFileAtomic(cacheFile, buf.Bytes(), cacheFilePermissions); err != nil {
			return errors.Join(errUtils.ErrCacheWrite, err)
		}
		return nil
	})
}

// UpdateCache atomically updates the cache file by acquiring a lock,
// loading the current configuration, applying the update function,
// and saving the result. This prevents race conditions when multiple
// processes try to update different fields simultaneously.
//
// Parameters:
//   - update: A function that modifies the provided CacheConfig in place.
//
// Returns an error if the cache file cannot be accessed, read, or written.
// Callers can check for specific failure types using errors.Is() with the
// following sentinel errors:
//   - ErrCacheRead: Failed to read the cache file
//   - ErrCacheUnmarshal: Failed to unmarshal cache content
//   - ErrCacheWrite: Failed to write the cache file
//   - ErrCacheMarshal: Failed to marshal cache content
func UpdateCache(update func(*CacheConfig)) error {
	return updateCache(context.Background(), true, update)
}

// UpdateCacheContext atomically updates the cache with a cancellation-aware lock.
func UpdateCacheContext(ctx context.Context, update func(*CacheConfig)) error {
	return updateCache(ctx, false, update)
}

func updateCache(ctx context.Context, bestEffort bool, update func(*CacheConfig)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return err
	}
	lock := getCacheFileLock(cacheFile)
	withLock := lock.WithLock
	if !bestEffort {
		withLock = func(fn func() error) error { return lock.WithLockContext(ctx, fn) }
	}
	return withLock(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := readCacheFile(cacheFile)
		if err != nil {
			return err
		}
		update(&current)
		return writeCacheFile(ctx, cacheFile, current)
	})
}

// readCacheFile requires the caller to hold the appropriate cache lock.
func readCacheFile(path string) (CacheConfig, error) {
	var current CacheConfig
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return current, nil
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return current, nil
		}
		return current, errors.Join(errUtils.ErrCacheRead, err)
	}
	if err := v.Unmarshal(&current); err != nil {
		return current, errors.Join(errUtils.ErrCacheUnmarshal, err)
	}
	return current, nil
}

// writeCacheFile requires the caller to hold the exclusive cache lock.
func writeCacheFile(ctx context.Context, path string, current CacheConfig) error {
	data := map[string]interface{}{
		"last_checked":                  current.LastChecked,
		"installation_id":               current.InstallationId,
		"telemetry_disclosure_shown":    current.TelemetryDisclosureShown,
		"browser_session_warning_shown": current.BrowserSessionWarningShown,
	}
	if len(current.ExperimentalWarnings) > 0 {
		data["experimental_warnings"] = current.ExperimentalWarnings
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(data); err != nil {
		return errors.Join(errUtils.ErrCacheMarshal, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fs := filesystem.NewOSFileSystem()
	if err := fs.WriteFileAtomic(path, buf.Bytes(), cacheFilePermissions); err != nil {
		return errors.Join(errUtils.ErrCacheWrite, err)
	}
	return nil
}

// shouldCheckForUpdatesAt is a helper for testing that checks if an update is due
// based on the provided timestamps and frequency.
func shouldCheckForUpdatesAt(lastChecked int64, frequency string, now int64) bool {
	interval, err := duration.Parse(frequency)
	if err != nil {
		// Log warning and default to daily if we can't parse.
		log.Warn("Unsupported check for update frequency encountered. Defaulting to daily", "frequency", frequency)
		interval = 86400 // daily
	}
	return now-lastChecked >= interval
}

// ShouldCheckForUpdates determines whether an update check is due based on the
// configured frequency and the time of the last check.
func ShouldCheckForUpdates(lastChecked int64, frequency string) bool {
	return shouldCheckForUpdatesAt(lastChecked, frequency, time.Now().Unix())
}
