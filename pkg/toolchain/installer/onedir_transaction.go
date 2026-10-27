package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/filesystem"
)

// onedirManifestSnapshot retains the previous metadata until the old flat
// entrypoint is removed. An unsuccessful migration must preserve both layouts.
type onedirManifestSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func snapshotOnedirManifest(versionDir string) (onedirManifestSnapshot, error) {
	snapshot := onedirManifestSnapshot{path: filepath.Join(versionDir, onedirManifestName)}
	info, err := os.Lstat(snapshot.path)
	if os.IsNotExist(err) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, fmt.Errorf("%w: failed to inspect onedir manifest: %w", ErrFileOperation, err)
	}
	if !info.Mode().IsRegular() {
		return snapshot, fmt.Errorf("%w: onedir manifest is not a regular file", ErrFileOperation)
	}
	data, err := os.ReadFile(snapshot.path) // #nosec G304 -- installer-managed manifest path.
	if err != nil {
		return snapshot, fmt.Errorf("%w: failed to back up onedir manifest: %w", ErrFileOperation, err)
	}
	snapshot.data, snapshot.mode, snapshot.exists = data, info.Mode().Perm(), true
	return snapshot, nil
}

func (snapshot onedirManifestSnapshot) restore() error {
	if snapshot.exists {
		if err := filesystem.NewOSFileSystem().WriteFileAtomic(snapshot.path, snapshot.data, snapshot.mode); err != nil {
			return fmt.Errorf("%w: failed to restore onedir manifest: %w", ErrFileOperation, err)
		}
		return nil
	}
	if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: failed to remove new onedir manifest: %w", ErrFileOperation, err)
	}
	return nil
}

// rollbackOnedirInstall restores the prior tree and manifest. Its errors are
// returned alongside the installation error, so recovery failures remain visible.
func rollbackOnedirInstall(treeDir, backupDir string, hadExisting bool, manifest onedirManifestSnapshot) error {
	removeErr := os.RemoveAll(treeDir)
	var restoreErr error
	if hadExisting {
		restoreErr = os.Rename(backupDir, treeDir)
	}
	return errors.Join(removeErr, restoreErr, manifest.restore())
}
