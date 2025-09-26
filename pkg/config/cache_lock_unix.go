//go:build !windows

package config

import (
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/gofrs/flock"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

func init() {
	// Set the platform-specific locking function.
	withCacheFileLock = withCacheFileLockUnix
	loadCacheWithReadLock = loadCacheWithReadLockUnix
}

func withCacheFileLockUnix(cacheFile string, fn func() error) error {
	lock := flock.New(cacheFile)
	// Try to acquire lock but don't retry too many times
	// This prevents hanging tests on systems with different locking semantics.
	const maxRetries = 3 // Only retry 3 times with 10ms between
	var locked bool
	var err error

	for i := 0; i < maxRetries; i++ {
		locked, err = lock.TryLock()
		if err != nil {
			return errors.Wrap(err, "error trying to acquire file lock")
		}
		if locked {
			break
		}
		// Wait a very short time before retrying.
		time.Sleep(10 * time.Millisecond)
	}

	if !locked {
		// If we can't get lock quickly, skip the cache operation
		// Cache is not critical for functionality.
		return fmt.Errorf("%w: cache file is locked by another process", errUtils.ErrCacheLocked)
	}

	defer func() {
		_ = lock.Unlock()
	}()
	return fn()
}

func loadCacheWithReadLockUnix(cacheFile string) (CacheConfig, error) {
	var cfg CacheConfig

	// Use file locking to prevent reading while another process is writing
	// Use TryRLock to avoid blocking indefinitely which can cause deadlocks in PTY tests.
	lock := flock.New(cacheFile)
	locked, err := lock.TryRLock()
	if err != nil {
		return cfg, errors.Wrap(err, "error trying to acquire read lock")
	}
	if !locked {
		// If we can't get the lock immediately, return empty config
		// This prevents deadlocks during concurrent access.
		return cfg, nil
	}
	defer func() {
		_ = lock.Unlock()
	}()

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
