package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// checkSourceDestination rejects a destination that traverses a symlink and, for an incoming file,
// one whose type differs from the source path.
func checkSourceDestination(target, name string, install bool) error {
	if err := rejectSourceSymlinkAncestors(target, name); err != nil {
		return err
	}
	if !install {
		return nil
	}
	return rejectSourceTypeChange(target, name)
}

// rejectSourceTypeChange fails before publication when an incoming file would replace a nonempty
// directory, or would need a directory where a file exists. Refresh replaces files in place and cannot
// change a path's type transactionally, so the error names the path and how to recover. An empty
// directory is still replaced by publication, as before.
func rejectSourceTypeChange(target, name string) error {
	const recovery = "; refresh cannot change a path between a file and a directory in place. Move only that path out of the component directory (keep anything it holds that you still need), then refresh again; do not delete the whole directory, which would also remove local state, `.terraform` and saved plans"
	local := filepath.FromSlash(name)
	if entries, err := os.ReadDir(filepath.Join(target, local)); err == nil && len(entries) > 0 {
		return fmt.Errorf("%w: source file %s conflicts with a nonempty directory in the target%s",
			errUtils.ErrSourceCopyFailed, name, recovery)
	}
	for dir := filepath.Dir(local); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
		if info, err := os.Lstat(filepath.Join(target, dir)); err == nil && info.Mode().IsRegular() {
			return fmt.Errorf("%w: source path %s needs a directory where the target has the file %s%s",
				errUtils.ErrSourceCopyFailed, name, filepath.ToSlash(dir), recovery)
		}
	}
	return nil
}

// sourceChange distinguishes files owned by the source from runtime artifacts.
type sourceChangeFlags struct{ install, owned bool }

func sourceChange(stage, target, name string, flags sourceChangeFlags, protected map[string]bool) (bool, error) {
	install, owned := flags.install, flags.owned
	if err := checkSourceDestination(target, name, install); err != nil {
		return false, err
	}
	path := filepath.Join(target, filepath.FromSlash(name))
	runtime, err := runtimeOwned(path, name, protected)
	if err != nil {
		return false, fmt.Errorf("%w: inspect runtime artifact %s: %w", errUtils.ErrSourceCopyFailed, name, err)
	}
	if runtime {
		return false, validateRuntimeSource(stage, path, name, flags, protected)
	}
	if !install {
		_, err := os.Lstat(path)
		return !os.IsNotExist(err), nil
	}
	if !owned {
		if err := validateUnownedIncoming(stage, target, name); err != nil {
			return false, err
		}
	}
	return true, nil
}

// identicalSourceFile permits unchanged source fixtures without replacing runtime-modified contents.
func identicalSourceFile(stage, path, name string) bool {
	old, oldErr := os.ReadFile(path)
	fresh, freshErr := os.ReadFile(filepath.Join(stage, filepath.FromSlash(name)))
	return oldErr == nil && freshErr == nil && string(old) == string(fresh)
}

// validateRuntimeSource allows only preserved locks and unchanged source-owned fixtures.
func validateRuntimeSource(stage, path, name string, flags sourceChangeFlags, protected map[string]bool) error {
	if !flags.install || strings.HasSuffix(strings.ToLower(filepath.Base(name)), ".terraform.lock.hcl") {
		return nil
	}
	if flags.owned && !protectedRuntimePath(path, name, protected) && identicalSourceFile(stage, path, name) {
		return nil
	}
	return fmt.Errorf("%w: downloaded source collides with runtime artifact %s", errUtils.ErrSourceCopyFailed, name)
}
