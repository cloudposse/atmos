//go:build !windows

package config

import (
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/gofrs/flock"
	"github.com/spf13/viper"
)

func init() {
	// Set the platform-specific locking function.
	withCacheFileLock = withCacheFileLockUnix
	loadCacheWithReadLock = loadCacheWithReadLockUnix
}

func withCacheFileLockUnix(cacheFile string, fn func() error) error {
	// Use a dedicated lock file to prevent lock loss during atomic rename.
	lockPath := cacheFile + ".lock"
	lock := flock.New(lockPath)
	// Try to acquire lock with reasonable retries for concurrent access.
	// This allows concurrent operations to succeed while preventing indefinite blocking.
	const maxRetries = 50 // Retry up to 50 times with 10ms between (500ms total).
	var locked bool
	var err error

	for i := 0; i < maxRetries; i++ {
		locked, err = lock.TryLock()
		if err != nil {
			return errors.Join(errUtils.ErrCacheLocked, err)
		}
		if locked {
			break
		}
		// Wait a short time before retrying.
		time.Sleep(10 * time.Millisecond)
	}

	if !locked {
		// If we can't get lock after retries, skip the cache operation.
		// Cache is not critical for functionality.
		return fmt.Errorf("%w: cache file is locked by another process", errUtils.ErrCacheLocked)
	}

	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Trace("Failed to unlock cache file", "error", err, "path", lockPath)
		}
	}()
	return fn()
}

func loadCacheWithReadLockUnix(cacheFile string) (CacheConfig, error) {
	var cfg CacheConfig

	// Use file locking to prevent reading while another process is writing
	// Use TryRLock to avoid blocking indefinitely which can cause deadlocks in PTY tests.
	// Use a dedicated lock file to prevent lock loss during atomic rename.
	lockPath := cacheFile + ".lock"
	lock := flock.New(lockPath)
	locked, err := lock.TryRLock()
	if err != nil {
		return cfg, errors.Join(errUtils.ErrCacheLocked, err)
	}
	if !locked {
		// If we can't get the lock immediately, return empty config
		// This prevents deadlocks during concurrent access.
		return cfg, nil
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Trace("Failed to unlock cache file during read", "error", err, "path", lockPath)
		}
	}()

	v := viper.New()
	v.SetConfigFile(cacheFile)
	if err := v.ReadInConfig(); err != nil {
		// If the config file doesn't exist, return empty config (no error)
		// This matches the Windows fallback behavior and test expectations.
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if errors.As(err, &configFileNotFoundError) {
			return cfg, nil
		}
		return cfg, errors.Join(errUtils.ErrCacheRead, err)
	}
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, errors.Join(errUtils.ErrCacheUnmarshal, err)
	}
	return cfg, nil
}
