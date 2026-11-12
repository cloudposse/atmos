package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path through a temporary file in the same directory, so a reader
// never sees a partial file. The file ends up with the given mode.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := tmp.Chmod(mode); err != nil && !isWindowsChmod(err) {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("set mode on %s: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// isWindowsChmod reports whether err is the harmless "not supported" result of Chmod on Windows.
func isWindowsChmod(err error) bool {
	return err != nil && os.PathSeparator == '\\'
}
