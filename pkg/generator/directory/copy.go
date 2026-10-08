// Package directory copies initialization sources without interpreting templates.
package directory

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	genfs "github.com/cloudposse/atmos/pkg/generator/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

type entry struct {
	path string
	mode fs.FileMode
}

const directoryMode = 0o755

// Copy copies a directory verbatim, excluding Git metadata. It checks the full
// tree before writing, rejects symlinks, and never merges or renders content.
func Copy(ctx context.Context, source, target string, force bool) error {
	defer perf.Track(nil, "directory.Copy")()

	src, err := validateCopyPaths(source, target, force)
	if err != nil {
		return err
	}
	entries, err := inspectTree(ctx, src, target)
	if err != nil {
		return err
	}
	return copyEntries(ctx, src, target, entries)
}

func validateCopyPaths(source, target string, force bool) (string, error) {
	src, err := resolveAncestor(source)
	if err != nil {
		return "", err
	}
	dst, err := resolveAncestor(target)
	if err != nil {
		return "", err
	}
	if containsPath(src, dst) || containsPath(dst, src) {
		return "", fmt.Errorf("%w: source and target directories must not overlap", errUtils.ErrPathTraversal)
	}
	if err := ValidateTarget(target, force); err != nil {
		return "", err
	}
	return src, nil
}

// ValidateTarget checks a copy destination before fetching its source. Copy
// repeats this validation before writing, since the destination may change.
func ValidateTarget(target string, force bool) error {
	defer perf.Track(nil, "directory.ValidateTarget")()

	if err := rejectDestinationSymlinks(target, "."); err != nil {
		return err
	}
	if err := genfs.ValidateTargetDirectory(target, force, false); err != nil {
		// Copy sources do not support the generic validator's --update hint.
		if errors.Is(err, errUtils.ErrTargetDirectoryNotEmpty) {
			return errUtils.Build(errUtils.ErrTargetDirectoryNotEmpty).
				WithExplanationf("Directory `%s` already contains files", target).
				WithHint("Choose another directory or use --force to overwrite matching files").WithExitCode(2).Err()
		}
		return err
	}
	return nil
}

func copyEntries(ctx context.Context, source, target string, entries []entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(target, directoryMode); err != nil {
		return err
	}
	for _, item := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyEntry(source, target, item); err != nil {
			return err
		}
	}
	return nil
}

func inspectTree(ctx context.Context, source, target string) ([]entry, error) {
	var entries []entry
	err := filepath.WalkDir(source, func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := regularEntryInfo(name, d)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, name)
		if err != nil || rel == "." {
			return err
		}
		if err := rejectDestinationSymlinks(target, rel); err != nil {
			return err
		}
		if err := checkDestinationType(target, rel, info.IsDir()); err != nil {
			return err
		}
		entries = append(entries, entry{path: rel, mode: info.Mode()})
		return nil
	})
	return entries, err
}

func regularEntryInfo(name string, d fs.DirEntry) (fs.FileInfo, error) {
	info, err := d.Info()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: copy source contains a symlink or special file: %s", errUtils.ErrInitialization, name)
	}
	return info, nil
}

func checkDestinationType(target, relative string, isDir bool) error {
	existing, err := os.Lstat(filepath.Join(target, relative))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.IsDir() != isDir {
		return fmt.Errorf("%w: file/directory collision at %s", errUtils.ErrInitialization, relative)
	}
	return nil
}

func copyEntry(source, target string, item entry) error {
	if err := rejectDestinationSymlinks(target, item.path); err != nil {
		return err
	}
	destination := filepath.Join(target, item.path)
	if item.mode.IsDir() {
		return os.MkdirAll(destination, directoryMode)
	}
	content, err := os.ReadFile(filepath.Join(source, item.path))
	if err != nil {
		return err
	}
	// Atomic replacement also prevents overwrites from modifying other hard
	// links to an existing destination file. No template filename rules apply.
	return filesystem.NewOSFileSystem().WriteFileAtomic(destination, content, item.mode.Perm())
}

func rejectDestinationSymlinks(root, relative string) error {
	name := filepath.Join(root, relative)
	for {
		info, err := os.Lstat(name)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: refusing to write through %s", errUtils.ErrSymlinkWrite, name)
		}
		if filepath.Clean(name) == filepath.Clean(root) {
			return nil
		}
		name = filepath.Dir(name)
	}
}

func containsPath(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && filepath.IsLocal(relative)
}

// resolveAncestor resolves existing symlinks even when the final path does not
// exist, so an alias of a source cannot bypass the overlapping-tree check.
func resolveAncestor(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) || filepath.Dir(absolute) == absolute {
		return "", err
	}
	parent, err := resolveAncestor(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}
