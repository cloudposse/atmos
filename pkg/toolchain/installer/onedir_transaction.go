package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/filesystem"
)

const onedirRecoveryPermissions = 0o600

// onedirManifestRecovery is persisted before tree mutation. Retaining the raw
// bytes and permissions also preserves malformed or noncanonical old metadata.
type onedirManifestRecovery struct {
	Data   []byte      `json:"data"`
	Mode   os.FileMode `json:"mode"`
	Exists bool        `json:"exists"`
}

// requireNoOnedirRecovery preserves ambiguous recovery artifacts. The snapshot
// does not distinguish an interrupted replacement from a committed installation
// whose cleanup failed, so guessing a rollback could undo a successful install.
func requireNoOnedirRecovery(versionDir string) error {
	path := filepath.Join(versionDir, onedirManifestName+onedirBackupSuffix)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: failed to inspect onedir recovery snapshot: %w", ErrFileOperation, err)
	}
	return fmt.Errorf("%w: unfinished onedir recovery at %s; stop processes using this tool, move its version directory aside, and reinstall the pinned version; see https://atmos.tools/cli/commands/toolchain/install#recovering-an-interrupted-install", ErrFileOperation, path)
}

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
		return snapshot, snapshot.persist()
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
	return snapshot, snapshot.persist()
}

// persist records recovery metadata before any part of the active layout moves.
func (snapshot onedirManifestSnapshot) persist() error {
	data, err := json.Marshal(onedirManifestRecovery{Data: snapshot.data, Mode: snapshot.mode, Exists: snapshot.exists})
	if err != nil {
		return fmt.Errorf("%w: failed to encode recovery snapshot: %w", ErrFileOperation, err)
	}
	if err := os.MkdirAll(filepath.Dir(snapshot.path), defaultMkdirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create recovery directory: %w", ErrFileOperation, err)
	}
	if err := filesystem.NewOSFileSystem().WriteFileAtomic(snapshot.path+onedirBackupSuffix, data, onedirRecoveryPermissions); err != nil {
		return fmt.Errorf("%w: failed to persist recovery snapshot: %w", ErrFileOperation, err)
	}
	return nil
}

// detach removes active metadata before changing the tree it describes. A new
// manifest is published atomically only after its replacement tree is complete.
func (snapshot onedirManifestSnapshot) detach() error {
	if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: failed to detach onedir manifest: %w", ErrFileOperation, err)
	}
	return nil
}

func (snapshot onedirManifestSnapshot) discard() error {
	if err := os.Remove(snapshot.path + onedirBackupSuffix); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: failed to remove recovery snapshot: %w", ErrFileOperation, err)
	}
	return nil
}

func (snapshot onedirManifestSnapshot) restore() error {
	if snapshot.exists {
		if err := filesystem.NewOSFileSystem().WriteFileAtomic(snapshot.path, snapshot.data, snapshot.mode); err != nil {
			return fmt.Errorf("%w: failed to restore onedir manifest: %w", ErrFileOperation, err)
		}
	}
	return snapshot.discard()
}

// rollbackOnedirInstall restores metadata only after its tree is restored. Each
// failure retains the persisted snapshot and any surviving tree backup so a
// subsequent install cannot destroy the recovery artifacts.
func rollbackOnedirInstall(treeDir, backupDir string, hadExisting bool, manifest onedirManifestSnapshot) error {
	if err := manifest.detach(); err != nil {
		return err
	}
	if err := os.RemoveAll(treeDir); err != nil {
		return fmt.Errorf("%w: failed to remove replacement tree; recovery snapshot retained: %w", ErrFileOperation, err)
	}
	if hadExisting {
		if err := os.Rename(backupDir, treeDir); err != nil {
			return fmt.Errorf("%w: failed to restore backup tree; recovery snapshot retained: %w", ErrFileOperation, err)
		}
	}
	return manifest.restore()
}
