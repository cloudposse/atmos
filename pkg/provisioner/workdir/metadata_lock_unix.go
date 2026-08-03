//go:build !windows

package workdir

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
)

func init() {
	// Set the platform-specific locking functions.
	withMetadataFileLock = withMetadataFileLockUnix
	loadMetadataWithReadLock = loadMetadataWithReadLockUnix
}

func withMetadataFileLockUnix(metadataFile string, fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := cache.NewFileLock(metadataFile).WithLockContext(ctx, fn); err != nil {
		if !errors.Is(err, errUtils.ErrCacheLocked) {
			return err
		}
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to acquire metadata lock").
			WithContext("path", metadataFile+".lock").
			Err()
	}
	return nil
}

func loadMetadataWithReadLockUnix(metadataFile string) (*WorkdirMetadata, error) {
	var metadata *WorkdirMetadata
	acquired, err := cache.NewFileLock(metadataFile).TryWithRLock(func() error {
		data, err := os.ReadFile(metadataFile)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to read metadata file").
				WithContext("path", metadataFile).
				Err()
		}

		var value WorkdirMetadata
		if err := json.Unmarshal(data, &value); err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to unmarshal metadata").
				WithContext("path", metadataFile).
				Err()
		}
		metadata = &value
		return nil
	})
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to acquire read lock for metadata").
			WithContext("path", metadataFile+".lock").
			Err()
	}
	if !acquired {
		return nil, nil
	}
	return metadata, nil
}
