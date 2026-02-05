package workdir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

// withMetadataFileLock is a platform-specific function for file locking.
// It is set during init() in metadata_lock_unix.go or metadata_lock_windows.go.
var withMetadataFileLock func(metadataFile string, fn func() error) error

// loadMetadataWithReadLock is a platform-specific function for loading metadata with read locks.
// It is set during init() in metadata_lock_unix.go.
var loadMetadataWithReadLock func(metadataFile string) (*WorkdirMetadata, error)

// ReadMetadata reads workdir metadata with read lock.
// Returns nil, nil if metadata file doesn't exist (not an error).
func ReadMetadata(workdirPath string) (*WorkdirMetadata, error) {
	defer perf.Track(nil, "workdir.ReadMetadata")()

	metadataPath := MetadataPath(workdirPath)

	// Check if metadata file exists.
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		// Try legacy location.
		legacyPath := filepath.Join(workdirPath, WorkdirMetadataFile)
		if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
			return nil, nil
		}
		metadataPath = legacyPath
	}

	// Use read lock for concurrent safety.
	return loadMetadataWithReadLock(metadataPath)
}

// WriteMetadata writes metadata atomically with exclusive lock.
// Creates the .atmos/ directory if it doesn't exist.
func WriteMetadata(workdirPath string, metadata *WorkdirMetadata) error {
	defer perf.Track(nil, "workdir.WriteMetadata")()

	atmosDir := filepath.Join(workdirPath, AtmosDir)
	if err := os.MkdirAll(atmosDir, DirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to create metadata directory").
			WithContext("path", atmosDir).
			Err()
	}

	metadataPath := MetadataPath(workdirPath)

	return withMetadataFileLock(metadataPath, func() error {
		data, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to marshal metadata").
				Err()
		}

		// Atomic write: temp file → rename.
		fs := filesystem.NewOSFileSystem()
		if err := fs.WriteFileAtomic(metadataPath, data, FilePermissionsStandard); err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to write metadata file").
				WithContext("path", metadataPath).
				Err()
		}
		return nil
	})
}

// UpdateLastAccessed atomically updates only the last accessed timestamp.
func UpdateLastAccessed(workdirPath string) error {
	defer perf.Track(nil, "workdir.UpdateLastAccessed")()

	metadataPath := MetadataPath(workdirPath)

	// Check if metadata file exists, try legacy location if not.
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		legacyPath := filepath.Join(workdirPath, WorkdirMetadataFile)
		if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
			// No metadata file, nothing to update.
			return nil
		}
		metadataPath = legacyPath
	}

	return withMetadataFileLock(metadataPath, func() error {
		// Read current metadata (without lock since we already have exclusive lock).
		metadata, err := readMetadataUnlocked(metadataPath)
		if err != nil {
			return err
		}

		// Update timestamp.
		metadata.LastAccessed = time.Now()

		// Write atomically.
		data, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to marshal metadata").
				Err()
		}

		fs := filesystem.NewOSFileSystem()
		if err := fs.WriteFileAtomic(metadataPath, data, FilePermissionsStandard); err != nil {
			return errUtils.Build(errUtils.ErrWorkdirMetadata).
				WithCause(err).
				WithExplanation("Failed to write metadata file").
				WithContext("path", metadataPath).
				Err()
		}
		return nil
	})
}

// readMetadataUnlocked reads metadata without acquiring a lock.
// This should only be called when the caller already holds the lock.
func readMetadataUnlocked(metadataPath string) (*WorkdirMetadata, error) {
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to read metadata file").
			WithContext("path", metadataPath).
			Err()
	}

	var metadata WorkdirMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithCause(err).
			WithExplanation("Failed to unmarshal metadata").
			WithContext("path", metadataPath).
			Err()
	}

	return &metadata, nil
}
