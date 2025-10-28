package toolchain

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
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

	u.PrintfMarkdownToTUI("%s Deleted **%d** files/directories from %s\n", theme.Styles.Checkmark, toolsCount, toolsDir)
	if cacheCount > 0 {
		u.PrintfMarkdownToTUI("%s Deleted **%d** files from %s cache\n", theme.Styles.Checkmark, cacheCount, cacheDir)
	}
	if tempCacheCount > 0 {
		u.PrintfMarkdownToTUI("%s Deleted **%d** files from %s cache\n", theme.Styles.Checkmark, tempCacheCount, tempCacheDir)
	}

	return nil
}

func cleanDir(path string, fatal bool) (int, error) {
	count, err := countFiles(path)
	if err != nil && !os.IsNotExist(err) {
		msg := fmt.Sprintf("failed to count files in %s: %v", path, err)
		if fatal {
			return 0, fmt.Errorf("%w: failed to count files in %s: %w", ErrFileOperation, path, err)
		}
		u.PrintfMessageToTUI("Warning: %s\n", msg)
	}

	err = os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		msg := fmt.Sprintf("failed to delete %s: %v", path, err)
		if fatal {
			return count, fmt.Errorf("%w: failed to delete %s: %w", ErrFileOperation, path, err)
		}
		u.PrintfMessageToTUI("Warning: %s\n", msg)
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
