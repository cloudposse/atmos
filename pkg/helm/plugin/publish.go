package plugin

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
)

type pluginBackup struct {
	original string
	dir      string
	parent   string
}

// publishInstall retains the previous plugin until the replacement is visible.
// Rollback data lives outside both HELM_PLUGINS and the disposable install stage.
func (i *Installer) publishInstall(pluginDir, replaceName string) error {
	backup, err := i.backupPlugin(replaceName)
	if err != nil {
		return err
	}
	destination := filepath.Join(i.dir, filepath.Base(pluginDir))
	if err := i.rename(pluginDir, destination); err != nil {
		publishErr := fmt.Errorf("%w: publish installed plugin: %w", errUtils.ErrHelmPluginInstall, err)
		if backup != nil {
			if restoreErr := i.rename(backup.dir, backup.original); restoreErr != nil {
				return fmt.Errorf("%w; rollback failed (previous plugin retained at %s): %w", publishErr, backup.dir, restoreErr)
			}
			_ = os.RemoveAll(backup.parent)
		}
		return publishErr
	}
	if backup != nil {
		_ = os.RemoveAll(backup.parent)
	}
	return nil
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
	if err := i.rename(original, backup.dir); err != nil {
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
