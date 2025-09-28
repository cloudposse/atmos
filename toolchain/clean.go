package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanToolsAndCaches handles the business logic for cleaning tools and cache directories.
// It performs file counting, deletion, and writes UI messages to stderr.
func CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir string) error {
	toolsCount, err := cleanDir(toolsDir, true)
	if err != nil {
		return err
	}

	cacheCount, _ := cleanDir(cacheDir, false) // warnings only
	tempCacheCount, _ := cleanDir(tempCacheDir, false)

	fmt.Printf("%s Deleted %d files/directories from %s\n", checkMark.Render(), toolsCount, toolsDir)
	if cacheCount > 0 {
		fmt.Printf("%s Deleted %d files from %s cache\n", checkMark.Render(), cacheCount, cacheDir)
	}
	if tempCacheCount > 0 {
		fmt.Printf("%s Deleted %d files from %s cache\n", checkMark.Render(), tempCacheCount, tempCacheDir)
	}

	return nil
}

func cleanDir(path string, fatal bool) (int, error) {
	count, err := countFiles(path)
	if err != nil && !os.IsNotExist(err) {
		msg := fmt.Sprintf("failed to count files in %s: %v", path, err)
		if fatal {
			return 0, fmt.Errorf(msg)
		}
		fmt.Println("Warning:", msg)
	}

	err = os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		msg := fmt.Sprintf("failed to delete %s: %v", path, err)
		if fatal {
			return count, fmt.Errorf(msg)
		}
		fmt.Println("Warning:", msg)
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
