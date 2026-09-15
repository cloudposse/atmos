package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
)

type pluginBackup struct {
	original string
	dir      string
	parent   string
}

// finishInstall restores the previous plugin after an unsuccessful replacement.
// Keep a failed rollback outside the managed path and report its location.
func (i *Installer) finishInstall(backup *pluginBackup, installErr error) error {
	if backup == nil {
		return installErr
	}
	if installErr != nil {
		if err := os.RemoveAll(backup.original); err != nil {
			return fmt.Errorf("%w; cannot restore previous plugin (retained at %s): %w", installErr, backup.dir, err)
		}
		if err := i.rename(backup.dir, backup.original); err != nil {
			return fmt.Errorf("%w; rollback failed (previous plugin retained at %s): %w", installErr, backup.dir, err)
		}
	}
	_ = os.RemoveAll(backup.parent)
	return installErr
}

func (i *Installer) backupPlugin(name string) (*pluginBackup, error) {
	if name == "" {
		return nil, nil
	}
	original, err := findPluginDirectory(i.dir, name)
	if err != nil {
		return nil, err
	}
	if original == "" {
		return nil, nil
	}
	parent, err := os.MkdirTemp(filepath.Dir(i.dir), ".helm-plugin-rollback-")
	if err != nil {
		return nil, fmt.Errorf("%w: create plugin rollback directory: %w", errUtils.ErrHelmPluginInstall, err)
	}
	backup := &pluginBackup{original: original, parent: parent, dir: filepath.Join(parent, "plugin")}
	if err := cp.Copy(original, backup.dir); err != nil {
		_ = os.RemoveAll(parent)
		return nil, fmt.Errorf("%w: retain previous plugin: %w", errUtils.ErrHelmPluginInstall, err)
	}
	return backup, nil
}

func findPluginDirectory(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("%w: inspect installed plugins: %w", errUtils.ErrHelmPluginInstall, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		metadata, err := readPluginMetadata(filepath.Join(path, "plugin.yaml"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("%w: read previous plugin: %w", errUtils.ErrHelmPluginInstall, err)
		}
		if metadata.Name == name {
			return path, nil
		}
	}
	return "", nil
}
