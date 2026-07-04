//go:build windows

package workdir

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

func init() {
	// Set the platform-specific locking functions.
	withMetadataFileLock = withMetadataFileLockWindows
	loadMetadataWithReadLock = loadMetadataWithReadLockWindows
}

func withMetadataFileLockWindows(metadataFile string, fn func() error) error {
	// No file locking on Windows to avoid timeout issues.
	// The metadata is non-critical functionality, so we can operate
	// without strict locking on Windows.

	// Add a small delay after operations to let Windows release file handles.
	defer func() {
		time.Sleep(50 * time.Millisecond)
	}()

	// Just execute the function without any locking.
	return fn()
}

func loadMetadataWithReadLockWindows(metadataFile string) (*WorkdirMetadata, error) {
	// On Windows, skip read locks entirely to avoid timeout issues.
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %w", errUtils.ErrWorkdirMetadata, err)
	}

	var metadata WorkdirMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("%w: failed to parse metadata: %w", errUtils.ErrWorkdirMetadata, err)
	}

	return &metadata, nil
}
