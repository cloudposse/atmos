package toolchain

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanToolsAndCaches handles the business logic for cleaning tools and cache directories.
// It performs file counting, deletion, and writes UI messages to stderr.
func CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir string) error {
	toolsCount := 0
	err := filepath.Walk(toolsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != toolsDir {
			toolsCount++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to count files in %s: %w", toolsDir, err)
	}
	err = os.RemoveAll(toolsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete %s: %w", toolsDir, err)
	}

	cacheCount := 0
	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != cacheDir {
			cacheCount++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to count files in %s: %v\n", cacheDir, err)
	}
	err = os.RemoveAll(cacheDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to delete %s: %v\n", cacheDir, err)
	}

	tempCacheCount := 0
	err = filepath.Walk(tempCacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != tempCacheDir {
			tempCacheCount++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to count files in %s: %v\n", tempCacheDir, err)
	}
	err = os.RemoveAll(tempCacheDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to delete %s: %v\n", tempCacheDir, err)
	}

	fmt.Printf("%s Deleted %d files/directories from %s\n", checkMark.Render(), toolsCount, toolsDir)
	if cacheCount > 0 {
		fmt.Printf("%s Deleted %d files from %s cache\n", checkMark.Render(), cacheCount, cacheDir)
	}
	if tempCacheCount > 0 {
		fmt.Printf("%s Deleted %d files from %s cache\n", checkMark.Render(), tempCacheCount, tempCacheDir)
	}

	return nil
}
