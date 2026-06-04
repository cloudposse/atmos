//go:build !windows

package workdir

import (
	"encoding/json"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/gofrs/flock"
)

func init() {
	// Set the platform-specific locking functions.
	withMetadataFileLock = withMetadataFileLockUnix
	loadMetadataWithReadLock = loadMetadataWithReadLockUnix
}

func withMetadataFileLockUnix(metadataFile string, fn func() error) error {
	// Use a dedicated lock file to prevent lock loss during atomic rename.
	lockPath := metadataFile + ".lock"
	lock := flock.New(lockPath)

	// Try to acquire lock with reasonable retries for concurrent access.
	// This allows concurrent operations to succeed while preventing indefinite blocking.
	const maxRetries = 50 // Retry up to 50 times with 10ms between (500ms total).
	var locked bool
	var err error

	for i := 0; i < maxRetries; i++ {
		locked, err = lock.TryLock()
		if err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to acquire metadata lock").
				WithContext("path", lockPath).
				Err()
		}
		if locked {
			break
		}
		// Wait a short time before retrying.
		time.Sleep(10 * time.Millisecond)
	}

	if !locked {
		// If we can't get lock after retries, skip the metadata operation.
		// Metadata is not critical for functionality.
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithExplanation("Metadata file is locked by another process").
			WithContext("path", lockPath).
			Err()
	}

	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Trace("Failed to unlock metadata file", "error", err, "path", lockPath)
		}
	}()

	return fn()
}

func loadMetadataWithReadLockUnix(metadataFile string) (*WorkdirMetadata, error) {
	// Use file locking to prevent reading while another process is writing.
	// Use TryRLock to avoid blocking indefinitely which can cause deadlocks in PTY tests.
	// Use a dedicated lock file to prevent lock loss during atomic rename.
	lockPath := metadataFile + ".lock"
	lock := flock.New(lockPath)
	locked, err := lock.TryRLock()
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to acquire read lock for metadata").
			WithContext("path", lockPath).
			Err()
	}
	if !locked {
		// If we can't get the lock immediately, return nil
		// This prevents deadlocks during concurrent access.
		return nil, nil
	}
	defer func() {
		if err := lock.Unlock(); err != nil {
			log.Trace("Failed to unlock metadata file during read", "error", err, "path", lockPath)
		}
	}()

	data, err := os.ReadFile(metadataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to read metadata file").
			WithContext("path", metadataFile).
			Err()
	}

	var metadata WorkdirMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to unmarshal metadata").
			WithContext("path", metadataFile).
			Err()
	}

	return &metadata, nil
}
