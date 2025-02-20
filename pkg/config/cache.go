package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/pkg/errors"
	"github.com/spf13/viper"

	u "github.com/cloudposse/atmos/pkg/utils"
)

type CacheConfig struct {
	LastChecked int64 `mapstructure:"last_checked"`
}

func GetCacheFilePath() (string, error) {
	xdgCacheHome := os.Getenv("XDG_CACHE_HOME")

	var cacheDir string

	if xdgCacheHome == "" {
		cacheDir = filepath.Join(".", ".atmos")
	} else {
		cacheDir = filepath.Join(xdgCacheHome, "atmos")
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", errors.Wrap(err, "error creating cache directory")
	}

	return filepath.Join(cacheDir, "cache.yaml"), nil
}

func withCacheFileLock(cacheFile string, fn func() error) error {
	lock := flock.New(cacheFile)
	err := lock.Lock()
	if err != nil {
		return errors.Wrap(err, "error acquiring file lock")
	}
	defer lock.Unlock()

	return fn()
}

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

	v := viper.New()
	v.SetConfigFile(cacheFile)

	if err := v.ReadInConfig(); err != nil {
		return cfg, errors.Wrap(err, "failed to read cache file")
	}

	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, errors.Wrap(err, "failed to unmarshal cache file")
	}

	return cfg, nil
}

func SaveCache2(cfg CacheConfig) error {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return err
	}

	return withCacheFileLock(cacheFile, func() error {
		v := viper.New()
		v.Set("last_checked", cfg.LastChecked)

		if err := v.WriteConfigAs(cacheFile); err != nil {
			return errors.Wrap(err, "failed to write cache file")
		}

		return nil
	})
}

func SaveCache(cfg CacheConfig) error {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return err
	}

	v := viper.New()
	v.Set("last_checked", cfg.LastChecked)

	if err := v.WriteConfigAs(cacheFile); err != nil {
		return errors.Wrap(err, "failed to write cache file")
	}

	return nil
}

func ShouldCheckForUpdates(lastChecked int64, frequency string) bool {
	now := time.Now().Unix()

	interval, err := parseFrequency(frequency)
	if err != nil {
		// Log warning and default to daily if we can’t parse
		u.LogWarning(fmt.Sprintf("Unsupported frequency '%s' encountered. Defaulting to daily.", frequency))

		interval = 86400 // daily
	}

	return now-lastChecked >= interval
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
