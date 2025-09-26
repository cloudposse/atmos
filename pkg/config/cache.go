package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/xdg"
	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
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

// writeFileAtomic is a platform-specific function for atomic file writing.
// It is set during init() in cache_atomic_unix.go or cache_atomic_windows.go.
var writeFileAtomic func(filename string, data []byte, perm os.FileMode) error

// WriteCacheConfig writes the cache config to the cache file using
// platform-appropriate locking mechanisms.
func WriteCacheConfig(cacheConfig CacheConfig) error {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCacheWrite, err)
	}

	var marshalError error
	err = withCacheFileLock(cacheFile, func() error {
		// Marshal the cache config
		yamlData, mErr := yaml.Marshal(cacheConfig)
		if mErr != nil {
			marshalError = fmt.Errorf("%w: %w", errUtils.ErrCacheMarshal, mErr)
			return marshalError
		}

		// Write atomically
		if wErr := writeFileAtomic(cacheFile, yamlData, 0o600); wErr != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrCacheWrite, wErr)
		}

		return nil
	})

	// Return the more specific error if marshaling failed
	if marshalError != nil {
		return marshalError
	}

	return err
}

// UpdateCache atomically updates the cache configuration using a user-provided
// update function. This method ensures thread-safe updates by using file locking and
// atomic file writes.
//
// The update function receives a pointer to the current cache configuration
// and can modify it in place. UpdateCache handles all file I/O operations and ensures
// data consistency across multiple processes.
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
				return fmt.Errorf(errUtils.ErrValueWrappingFormat, errUtils.ErrCacheRead, err)
			}
			if err := v.Unmarshal(&cfg); err != nil {
				return fmt.Errorf(errUtils.ErrValueWrappingFormat, errUtils.ErrCacheUnmarshal, err)
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

		// Marshal to YAML with proper formatting
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(data); err != nil {
			return fmt.Errorf(errUtils.ErrValueWrappingFormat, errUtils.ErrCacheMarshal, err)
		}

		// Write atomically.
		if err := writeFileAtomic(cacheFile, buf.Bytes(), 0o600); err != nil {
			return fmt.Errorf(errUtils.ErrValueWrappingFormat, errUtils.ErrCacheWrite, err)
		}

		return nil
	})
}

// LoadCache loads the cache from the cache file.
// For Unix systems, this uses a read lock. For Windows, it uses the standard lock.
func LoadCache() (CacheConfig, error) {
	cacheFile, err := GetCacheFilePath()
	if err != nil {
		return CacheConfig{}, fmt.Errorf("%w: %w", errUtils.ErrCacheRead, err)
	}

	// Use platform-specific loading function if available (Unix with read locks)
	if loadCacheWithReadLock != nil {
		return loadCacheWithReadLock(cacheFile)
	}

	// Fallback for platforms without read lock support (Windows)
	var cache CacheConfig
	err = withCacheFileLock(cacheFile, func() error {
		data, readErr := os.ReadFile(cacheFile)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				// File doesn't exist yet, return empty cache
				return nil
			}
			return fmt.Errorf("%w: %w", errUtils.ErrCacheRead, readErr)
		}

		if len(data) > 0 {
			if unmarshalErr := yaml.Unmarshal(data, &cache); unmarshalErr != nil {
				return fmt.Errorf("%w: %w", errUtils.ErrCacheUnmarshal, unmarshalErr)
			}
		}
		return nil
	})

	return cache, err
}

func GetCachePath() string {
	v := viper.Get("atmos_cache_file")
	if v != nil {
		path := v.(string)
		if path != "" {
			return path
		}
	}

	path, err := os.UserCacheDir()
	if err != nil {
		path = os.TempDir()
	}

	return filepath.Join(path, "atmos", "cache.yaml")
}

func CheckTelemetryDisclosure(cache CacheConfig) {
	if !cache.TelemetryDisclosureShown {
		// Prepare message parts
		const separatorLength = 60
		separator := strings.Repeat("─", separatorLength)
		line1 := "This command will anonymously report telemetric information"
		line2 := "essential for improving Atmos."
		line3 := "To disable it, please run:"

		// Print disclosure message
		fmt.Fprintf(os.Stderr, "%s\n", separator)
		fmt.Fprintf(os.Stderr, "%s\n%s\n\n%s\n", line1, line2, line3)
		fmt.Fprintf(os.Stderr, "  export ATMOS_TELEMETRY_DISABLED=true\n\n")
		fmt.Fprintf(os.Stderr, "Visit https://atmos.tools/cli/telemetry for more details.\n")
		fmt.Fprintf(os.Stderr, "%s\n", separator)

		// Update cache to record that disclosure has been shown
		err := UpdateCache(func(cfg *CacheConfig) {
			cfg.TelemetryDisclosureShown = true
		})
		if err != nil {
			log.Debug("Failed to update telemetry disclosure flag in cache", "error", err)
		}
	}
}

// SaveCache atomically saves the provided cache configuration to disk.
// This method ensures thread-safe writes by using file locking and atomic file operations.
//
// SaveCache is useful for saving a complete CacheConfig state, such as when
// initializing the cache or replacing the entire configuration. For partial updates,
// consider using UpdateCache instead, which provides a safer way to modify specific
// fields without race conditions.
//
// The cache file is written with proper YAML formatting and appropriate file permissions.
// On Unix systems, the file is created with mode 0600 to ensure it's only readable by the owner.
// The write operation is atomic, meaning the file is either fully written or not modified at all,
// preventing corruption from partial writes or system crashes.
//
// SaveCache uses platform-specific file locking mechanisms to coordinate access across
// multiple Atmos processes. This ensures that concurrent executions don't corrupt the cache
// data.
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
			return fmt.Errorf(errUtils.ErrValueWrappingFormat, errUtils.ErrCacheMarshal, err)
		}

		// Write atomically
		if err := writeFileAtomic(cacheFile, buf.Bytes(), 0o600); err != nil {
			return fmt.Errorf(errUtils.ErrValueWrappingFormat, errUtils.ErrCacheWrite, err)
		}

		return nil
	})
}

// GetLastChecked returns the last time Atmos checked for updates.
func GetLastChecked() int64 {
	cacheConfig, err := LoadCache()
	if err != nil {
		return 0
	}
	return cacheConfig.LastChecked
}

// SetLastChecked sets the last time Atmos checked for updates.
func SetLastChecked() {
	now := time.Now().Unix()
	err := UpdateCache(func(cfg *CacheConfig) {
		cfg.LastChecked = now
	})
	if err != nil {
		log.Debug("Failed to update last checked timestamp", "error", err)
	}
}

// GetInstallationId returns the unique installation ID for telemetry.
func GetInstallationId() string {
	cacheConfig, err := LoadCache()
	if err != nil {
		return ""
	}
	return cacheConfig.InstallationId
}

// SetInstallationId sets the unique installation ID for telemetry.
func SetInstallationId(id string) {
	err := UpdateCache(func(cfg *CacheConfig) {
		cfg.InstallationId = id
	})
	if err != nil {
		log.Debug("Failed to update installation ID", "error", err)
	}
}

// DisableVersionCheck returns true if version checking is disabled.
func DisableVersionCheck() bool {
	// Prioritize environment variable
	if disabled, _ := strconv.ParseBool(os.Getenv("ATMOS_VERSION_CHECK_DISABLED")); disabled { //nolint:forbidigo // Direct env check needed for priority
		return true
	}

	// Fall back to Viper configuration
	return viper.GetBool("atmos_check_disabled")
}

func UpdateCheckFrequency() int64 {
	return viper.GetInt64("atmos_update_check_frequency")
}

// IsCI returns true if running in a CI/CD environment.
func IsCI() bool {
	// Common CI environment variables
	ciVars := []string{
		"CI",
		"CONTINUOUS_INTEGRATION",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"CIRCLECI",
		"JENKINS_HOME",
		"TRAVIS",
		"DRONE",
		"BUILD_ID",
		"TEAMCITY_VERSION",
		"TF_BUILD", // Azure DevOps
	}

	for _, v := range ciVars {
		if os.Getenv(v) != "" { //nolint:forbidigo // Checking multiple CI env vars
			return true
		}
	}

	return false
}

// IsDockerContainer returns true if running inside a Docker container.
func IsDockerContainer() bool {
	// Check for .dockerenv file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check cgroup on Linux
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
			return bytes.Contains(data, []byte("docker")) || bytes.Contains(data, []byte("containerd"))
		}
	}

	return false
}

// IsTelemetryDisabled returns true if telemetry reporting is disabled.
func IsTelemetryDisabled() bool {
	// Check if explicitly disabled
	if disabled, _ := strconv.ParseBool(os.Getenv("ATMOS_TELEMETRY_DISABLED")); disabled { //nolint:forbidigo // Direct env check needed for priority
		return true
	}

	// Also disable in CI/CD environments
	if IsCI() {
		return true
	}

	// Also disable in Docker containers
	if IsDockerContainer() {
		return true
	}

	// Check Viper configuration
	return viper.GetBool("atmos_telemetry_disabled")
}

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
// last check time and the configured frequency.
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
