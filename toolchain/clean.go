package toolchain

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// CleanToolsAndCaches handles the business logic for cleaning tools and cache directories.
// It performs file counting, deletion, and writes UI messages to stderr.
func CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir string) error {
	defer perf.Track(nil, "toolchain.CleanToolsAndCaches")()

	toolsCount, err := cleanDir(toolsDir, true)
	if err != nil {
		return err
	}

	cacheCount, _ := cleanDir(cacheDir, false) // warnings only
	tempCacheCount, _ := cleanDir(tempCacheDir, false)

	_ = ui.Successf("Deleted **%d** files/directories from %s", toolsCount, toolsDir)
	if cacheCount > 0 {
		_ = ui.Successf("Deleted **%d** files from %s cache", cacheCount, cacheDir)
	}
	if tempCacheCount > 0 {
		_ = ui.Successf("Deleted **%d** files from %s cache", tempCacheCount, tempCacheDir)
	}

	return nil
}

func cleanDir(path string, fatal bool) (int, error) {
	// Defensive: refuse to operate on empty or root-like paths.
	if isDangerousPath(path) {
		return 0, fmt.Errorf("%w: refusing to delete dangerous path: %s", ErrFileOperation, path)
	}

	count, err := countFiles(path)
	if err != nil && !os.IsNotExist(err) {
		msg := fmt.Sprintf("failed to count files in %s: %v", path, err)
		if fatal {
			return 0, fmt.Errorf("%w: failed to count files in %s: %w", ErrFileOperation, path, err)
		}
		_ = ui.Warning(msg)
	}

	err = os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		msg := fmt.Sprintf("failed to delete %s: %v", path, err)
		if fatal {
			return count, fmt.Errorf("%w: failed to delete %s: %w", ErrFileOperation, path, err)
		}
		_ = ui.Warning(msg)
	}

	return count, nil
}

func countFiles(root string) (int, error) {
	count := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != root {
			count++
		}
		return nil
	})
	return count, err
}

// isDangerousPath checks if a path is dangerous to delete (empty, root, or drive root).
func isDangerousPath(path string) bool {
	// Clean the path first to normalize it (handles //, /./, /../, etc.).
	cleaned := filepath.Clean(path)

	if cleaned == "" || cleaned == "." || cleaned == "/" {
		return true
	}

	// Check for drive roots on Windows (C:, D:, etc.).
	if len(cleaned) == 2 && cleaned[1] == ':' {
		return true
	}

	// Also check for drive roots with slash (C:\, D:\).
	if len(cleaned) == 3 && cleaned[1] == ':' && (cleaned[2] == '\\' || cleaned[2] == '/') {
		return true
	}

	return false
}
