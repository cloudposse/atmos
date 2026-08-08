package toolchain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// deletedFilesFromCacheFormat is the shared success message format for reporting how many
// files were deleted from a cache directory (main cache or temp cache).
const deletedFilesFromCacheFormat = "Deleted **%d** files from %s cache"

// isTTYForStdoutFunc is a function variable for detecting whether stdout supports an
// interactive TTY. This allows tests to force the non-interactive branch of confirmClean
// deterministically, without depending on the ambient test-runner environment (which may or
// may not have a real TTY attached) and without exercising the huh prompt library, which
// requires a real /dev/tty.
var isTTYForStdoutFunc = term.IsTTYSupportForStdout

// CleanOptions configures optional behavior for RunClean.
type CleanOptions struct {
	// DryRun previews what would be deleted without deleting anything.
	DryRun bool
	// CacheOnly limits cleaning to the cache/temp-cache directories, leaving installed tools alone.
	CacheOnly bool
	// Force skips the interactive confirmation prompt.
	Force bool
}

// RunClean cleans the toolchain's installed-tools and cache directories according to opts.
//
// Unless opts.DryRun or opts.Force is set, it prompts for confirmation before deleting
// anything, showing the directories that will be removed. In a non-interactive session
// (no TTY), it refuses to delete without --force rather than silently proceeding.
func RunClean(toolsDir, cacheDir, tempCacheDir string, opts CleanOptions) error {
	defer perf.Track(nil, "toolchain.RunClean")()

	if opts.DryRun {
		return previewClean(toolsDir, cacheDir, tempCacheDir, opts.CacheOnly)
	}

	if !opts.Force {
		confirmed, err := confirmClean(toolsDir, cacheDir, tempCacheDir, opts.CacheOnly)
		if err != nil {
			return err
		}
		if !confirmed {
			ui.Warning("Clean aborted.")
			return nil
		}
	}

	if opts.CacheOnly {
		return cleanCachesOnly(cacheDir, tempCacheDir)
	}

	return CleanToolsAndCaches(toolsDir, cacheDir, tempCacheDir)
}

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

	ui.Successf("Deleted **%d** files/directories from %s", toolsCount, toolsDir)
	reportCacheDeletion(cacheCount, cacheDir)
	reportCacheDeletion(tempCacheCount, tempCacheDir)

	return nil
}

// cleanCachesOnly removes only the cache and temp-cache directories, leaving installed tools
// intact. Used for `atmos toolchain clean --cache-only`.
func cleanCachesOnly(cacheDir, tempCacheDir string) error {
	defer perf.Track(nil, "toolchain.cleanCachesOnly")()

	cacheCount, _ := cleanDir(cacheDir, false) // warnings only
	tempCacheCount, _ := cleanDir(tempCacheDir, false)

	reportCacheDeletion(cacheCount, cacheDir)
	reportCacheDeletion(tempCacheCount, tempCacheDir)

	return nil
}

// reportCacheDeletion prints a success message for a cache directory's deletion count,
// if anything was actually deleted.
func reportCacheDeletion(count int, dir string) {
	if count > 0 {
		ui.Successf(deletedFilesFromCacheFormat, count, dir)
	}
}

// confirmClean shows the directories that will be deleted and prompts the user to confirm.
// In a non-interactive session (no TTY), it returns ErrToolchainCleanRequiresConfirmation
// instead of prompting, so callers must pass --force to proceed non-interactively.
func confirmClean(toolsDir, cacheDir, tempCacheDir string, cacheOnly bool) (bool, error) {
	defer perf.Track(nil, "toolchain.confirmClean")()

	ui.Writeln("This will permanently delete:")
	if !cacheOnly {
		ui.Writeln(fmt.Sprintf("  - Installed tools: %s", toolsDir))
	}
	ui.Writeln(fmt.Sprintf("  - Cache: %s", cacheDir))
	ui.Writeln(fmt.Sprintf("  - Temp cache: %s", tempCacheDir))
	ui.Writeln("")

	if !isTTYForStdoutFunc() {
		ui.Error("Clean cancelled - use --force to clean in non-interactive mode")
		return false, errUtils.ErrToolchainCleanRequiresConfirmation
	}

	var confirmed bool
	confirmPrompt := uiutils.NewAtmosConfirm().
		Title("Are you sure you want to continue?").
		Affirmative("Yes, delete").
		Negative("No, cancel").
		Value(&confirmed).
		WithTheme(uiutils.NewAtmosHuhTheme())

	if err := confirmPrompt.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %w", errUtils.ErrToolchainCleanConfirmation, err)
	}

	return confirmed, nil
}

// previewClean walks the target directories and reports what would be deleted, without
// deleting anything. Used for `atmos toolchain clean --dry-run`.
func previewClean(toolsDir, cacheDir, tempCacheDir string, cacheOnly bool) error {
	defer perf.Track(nil, "toolchain.previewClean")()

	if !cacheOnly {
		if err := previewDir("installed tools", toolsDir); err != nil {
			return err
		}
	}
	if err := previewDir("cache", cacheDir); err != nil {
		return err
	}
	return previewDir("temp cache", tempCacheDir)
}

// previewDir reports the file count and total size that would be deleted under path.
func previewDir(label, path string) error {
	defer perf.Track(nil, "toolchain.previewDir")()

	count, size, err := countFilesAndSize(path)
	if err != nil {
		if os.IsNotExist(err) {
			ui.Writeln(fmt.Sprintf("Would delete 0 files (0 B) from %s: %s (does not exist)", label, path))
			return nil
		}
		return fmt.Errorf("%w: failed to inspect %s: %w", ErrFileOperation, path, err)
	}
	ui.Writeln(fmt.Sprintf("Would delete %d files (%s) from %s: %s", count, formatFileSize(size), label, path))
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
		ui.Warning(msg)
	}

	err = os.RemoveAll(path)
	if err != nil && !os.IsNotExist(err) {
		msg := fmt.Sprintf("failed to delete %s: %v", path, err)
		if fatal {
			return count, fmt.Errorf("%w: failed to delete %s: %w", ErrFileOperation, path, err)
		}
		ui.Warning(msg)
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

// countFilesAndSize walks root and returns the number of entries (files and directories, not
// counting root itself) and the total size in bytes of the regular files under it. Used for
// `atmos toolchain clean --dry-run` previews.
func countFilesAndSize(root string) (int, int64, error) {
	count := 0
	var size int64
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != root {
			count++
			if !info.IsDir() {
				size += info.Size()
			}
		}
		return nil
	})
	return count, size, err
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
