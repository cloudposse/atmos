package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/xdg"
	log "github.com/charmbracelet/log"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type CacheConfig struct {
	LastChecked              int64  `mapstructure:"last_checked"`
	InstallationId           string `mapstructure:"installation_id"`
	TelemetryDisclosureShown bool   `mapstructure:"telemetry_disclosure_shown"`
}

func GetCacheFilePath() (string, error) {
	// Use the XDG library which automatically handles XDG_CACHE_HOME
	// and falls back to the correct default based on the OS
	cacheDir := filepath.Join(xdg.CacheHome, "atmos")

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", errors.Wrap(err, "error creating cache directory")
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
		_ = v.ReadInConfig()
		// Ignore unmarshal errors on Windows - cache is non-critical.
		_ = v.Unmarshal(&cfg)
		return cfg, nil
	}

	// Unix: Use the platform-specific read lock function
	return loadCacheWithReadLock(cacheFile)
}

func SaveCache(cfg CacheConfig) error {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return err
	}

	// Use file locking to prevent concurrent writes
	return withCacheFileLock(cacheFile, func() error {
		v := viper.New()
		v.Set("last_checked", cfg.LastChecked)
		v.Set("installation_id", cfg.InstallationId)
		v.Set("telemetry_disclosure_shown", cfg.TelemetryDisclosureShown)
		if err := v.WriteConfigAs(cacheFile); err != nil {
			return errors.Wrap(err, "failed to write cache file")
		}
		return nil
	})
}

// UpdateCache atomically updates the cache file by acquiring a lock,
// loading the current configuration, applying the update function,
// and saving the result. This prevents race conditions when multiple
// processes try to update different fields simultaneously.
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
				return errors.Wrap(err, "failed to read cache file")
			}
			if err := v.Unmarshal(&cfg); err != nil {
				return errors.Wrap(err, "failed to unmarshal cache file")
			}
		}

		// Apply the update
		update(&cfg)

		// Save the updated configuration
		v := viper.New()
		v.Set("last_checked", cfg.LastChecked)
		v.Set("installation_id", cfg.InstallationId)
		v.Set("telemetry_disclosure_shown", cfg.TelemetryDisclosureShown)
		if err := v.WriteConfigAs(cacheFile); err != nil {
			return errors.Wrap(err, "failed to write cache file")
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
