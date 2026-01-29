package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
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

// getCacheFileLock returns a FileLock for the cache file.
func getCacheFileLock(cacheFile string) cache.FileLock {
	return cache.NewFileLock(cacheFile)
}

// LoadCache loads the cache configuration from the cache file.
// Uses platform-specific file locking to prevent concurrent read/write issues.
func LoadCache() (CacheConfig, error) {
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
	lockErr := lock.WithRLock(func() error {
		v := viper.New()
		v.SetConfigFile(cacheFile)
		if err := v.ReadInConfig(); err != nil {
			// If the config file doesn't exist, return empty config (no error).
			var configNotFound viper.ConfigFileNotFoundError
			if errors.As(err, &configNotFound) {
				return nil
			}
			readErr = errors.Join(errUtils.ErrCacheRead, err)
			return nil
		}
		if err := v.Unmarshal(&cfg); err != nil {
			readErr = errors.Join(errUtils.ErrCacheUnmarshal, err)
			return nil
		}
		return nil
	})

	// Lock errors are non-critical for cache.
	if lockErr != nil {
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
		return cfg, readErr
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

	lock := getCacheFileLock(cacheFile)
	// Use file locking to prevent concurrent updates.
	return lock.WithLock(func() error {
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
	interval, err := parseFrequency(frequency)
	if err != nil {
		// Log warning and default to daily if we can't parse
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

// parseFrequency attempts to parse the frequency string in three ways:
// 1. As an integer (seconds)
// 2. As a duration with a suffix (e.g., "1h", "5m", "30s")
// 3. As one of the predefined keywords (daily, hourly, etc.)
func parseFrequency(frequency string) (int64, error) {
	freq := strings.TrimSpace(frequency)

	if intVal, err := strconv.ParseInt(freq, 10, 64); err == nil {
		if intVal > 0 {
			return intVal, nil
		}
	}

	// Parse duration with suffix
	if len(freq) > 1 {
		unit := freq[len(freq)-1]
		valPart := freq[:len(freq)-1]
		if valInt, err := strconv.ParseInt(valPart, 10, 64); err == nil && valInt > 0 {
			switch unit {
			case 's':
				return valInt, nil
			case 'm':
				return valInt * 60, nil
			case 'h':
				return valInt * 3600, nil
			case 'd':
				return valInt * 86400, nil
			default:
				return 0, fmt.Errorf("unrecognized duration unit: %s", string(unit))
			}
		}
	}

	// Handle predefined keywords
	switch freq {
	case "minute":
		return 60, nil
	case "hourly":
		return 3600, nil
	case "daily":
		return 86400, nil
	case "weekly":
		return 604800, nil
	case "monthly":
		return 2592000, nil
	case "yearly":
		return 31536000, nil
	default:
		return 0, fmt.Errorf("unrecognized frequency: %s", freq)
	}
}
