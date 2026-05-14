package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// CacheDirPermissions is the default permission for cache directory (read/write/execute for owner, read/execute for group and others).
	CacheDirPermissions = 0o755
)

type CacheConfig struct {
	LastChecked                int64  `mapstructure:"last_checked" yaml:"last_checked"`
	InstallationId             string `mapstructure:"installation_id" yaml:"installation_id"`
	TelemetryDisclosureShown   bool   `mapstructure:"telemetry_disclosure_shown" yaml:"telemetry_disclosure_shown"`
	BrowserSessionWarningShown bool   `mapstructure:"browser_session_warning_shown" yaml:"browser_session_warning_shown"`
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

// withCacheFileLock is a platform-specific function for file locking.
// It is set during init() in cache_lock_unix.go or cache_lock_windows.go.
var withCacheFileLock func(cacheFile string, fn func() error) error

// loadCacheWithReadLock is a platform-specific function for loading cache with read locks.
// It is set during init() in cache_lock_unix.go.
var loadCacheWithReadLock func(cacheFile string) (CacheConfig, error)

func LoadCache() (CacheConfig, error) {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return CacheConfig{}, err
	}

	var cfg CacheConfig
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		// No file yet, return default
		return cfg, nil
	}

	// On Windows, skip read locks entirely to avoid timeout issues.
	if runtime.GOOS == "windows" {
		v := viper.New()
		v.SetConfigFile(cacheFile)
		// Ignore read errors on Windows - cache is non-critical.
		if err := v.ReadInConfig(); err != nil {
			log.Trace("Failed to read cache file on Windows (non-critical)", "error", err, "file", cacheFile)
		}
		// Ignore unmarshal errors on Windows - cache is non-critical.
		if err := v.Unmarshal(&cfg); err != nil {
			log.Trace("Failed to unmarshal cache on Windows (non-critical)", "error", err, "file", cacheFile)
		}
		return cfg, nil
	}

	// Unix: Use the platform-specific read lock function
	return loadCacheWithReadLock(cacheFile)
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

	// Use file locking to prevent concurrent writes
	return withCacheFileLock(cacheFile, func() error {
		// Prepare the config data.
		data := map[string]interface{}{
			"last_checked":               cfg.LastChecked,
			"installation_id":            cfg.InstallationId,
			"telemetry_disclosure_shown": cfg.TelemetryDisclosureShown,
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
		if err := fs.WriteFileAtomic(cacheFile, buf.Bytes(), 0o644); err != nil {
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
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return err
	}

	// Use file locking to prevent concurrent updates
	return withCacheFileLock(cacheFile, func() error {
		// Load current configuration
		var cfg CacheConfig
		if _, err := os.Stat(cacheFile); err == nil {
			v := viper.New()
			v.SetConfigFile(cacheFile)
			if err := v.ReadInConfig(); err != nil {
				return errors.Join(errUtils.ErrCacheRead, err)
			}
			if err := v.Unmarshal(&cfg); err != nil {
				return errors.Join(errUtils.ErrCacheUnmarshal, err)
			}
		}

		// Apply the update
		update(&cfg)

		// Prepare the updated configuration data.
		data := map[string]interface{}{
			"last_checked":               cfg.LastChecked,
			"installation_id":            cfg.InstallationId,
			"telemetry_disclosure_shown": cfg.TelemetryDisclosureShown,
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
		if err := fs.WriteFileAtomic(cacheFile, buf.Bytes(), 0o644); err != nil {
			return errors.Join(errUtils.ErrCacheWrite, err)
		}
		return nil
	})
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
