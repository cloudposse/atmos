package lockfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cp "github.com/otiai10/copy"

	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	errUnsupportedRollbackFile = errors.New("cannot snapshot special file in vendor target")
	errRollbackOverlapsState   = errors.New("vendor target overlaps a receipt or mutation lock")
)

// targetSnapshot owns private backups for one materialization transaction.
// Whole target trees preserve unowned files, empty directories, and stale files pruned on commit.
type targetSnapshot struct {
	dir     string
	entries []targetBackup
}

type targetBackup struct {
	path   string
	backup string
	exists bool
}

// snapshotTargets runs under mutation locks, before copying or stale-file pruning begins.
func snapshotTargets(config *schema.AtmosConfiguration, previous *LockFile, record *PreparedRecord) (*targetSnapshot, error) {
	base, err := projectBase(config)
	if err != nil {
		return nil, err
	}
	base, err = canonicalPath(base)
	if err != nil {
		return nil, err
	}
	roots, err := rollbackRoots(config, previous, record, base)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "atmos-vendor-rollback-")
	if err != nil {
		return nil, err
	}
	snapshot := &targetSnapshot{dir: dir}
	for i, path := range roots {
		entry := targetBackup{path: path, backup: filepath.Join(dir, fmt.Sprint(i))}
		_, statErr := os.Lstat(path)
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, errors.Join(statErr, snapshot.close())
		}
		entry.exists = statErr == nil
		if entry.exists {
			if err := copyRollbackTree(path, entry.backup); err != nil {
				return nil, errors.Join(err, snapshot.close())
			}
		}
		snapshot.entries = append(snapshot.entries, entry)
	}
	return snapshot, nil
}

// rollbackRoots collapses overlapping targets and includes destinations reached through symlinks.
// Missing ancestors are included so a failed first install removes directories it created.
func rollbackRoots(config *schema.AtmosConfiguration, previous *LockFile, record *PreparedRecord, base string) ([]string, error) {
	artifacts := []Artifact{record.artifact}
	if old, ok := previous.Artifacts[record.id]; ok {
		artifacts = append(artifacts, old)
	}
	var paths []string
	for i := range artifacts {
		owned, err := artifactRollbackPaths(config, &artifacts[i])
		if err != nil {
			return nil, err
		}
		paths = append(paths, owned...)
	}
	for i, path := range paths {
		root, err := rollbackRoot(path, base)
		if err != nil {
			return nil, err
		}
		paths[i] = root
	}
	roots := collapseRollbackRoots(paths)
	if err := validateRollbackRoots(config, base, roots); err != nil {
		return nil, err
	}
	return roots, nil
}

// artifactRollbackPaths includes the target tree and any symlink-reachable owned destinations.
func artifactRollbackPaths(config *schema.AtmosConfiguration, artifact *Artifact) ([]string, error) {
	normalized := *artifact
	relative, err := projectRelativeTarget(config, artifact.Target)
	if err != nil {
		return nil, err
	}
	normalized.Target = relative
	root, err := lockTargetRoot(config, relative)
	if err != nil {
		return nil, err
	}
	owned, err := newArtifactPaths(config, normalized)
	if err != nil {
		return nil, err
	}
	paths := []string{root}
	for path := range owned {
		paths = append(paths, path)
	}
	return paths, nil
}

// collapseRollbackRoots avoids copying or restoring the same subtree more than once.
func collapseRollbackRoots(paths []string) []string {
	sort.Strings(paths)
	var roots []string
	for _, path := range paths {
		covered := false
		for _, root := range roots {
			covered = covered || withinRollbackRoot(root, path)
		}
		if !covered {
			roots = append(roots, path)
		}
	}
	return roots
}

// validateRollbackRoots keeps receipt bytes and held advisory-lock inodes outside copied trees.
func validateRollbackRoots(config *schema.AtmosConfiguration, base string, roots []string) error {
	receipt, err := canonicalPath(Path(config))
	if err != nil {
		return err
	}
	for _, state := range []string{Path(config), receipt, receipt + ".lock", filepath.Join(base, ".atmos", "vendor-mutation.lock")} {
		resolved, err := canonicalPath(state)
		if err != nil {
			return err
		}
		for _, root := range roots {
			if withinRollbackRoot(root, state) || withinRollbackRoot(root, resolved) {
				return fmt.Errorf("%w: %s", errRollbackOverlapsState, root)
			}
		}
	}
	return nil
}

// rollbackRoot refuses symlink escapes and finds the earliest directory a copy could create.
func rollbackRoot(path, base string) (string, error) {
	if err := validateRollbackSymlinks(path); err != nil {
		return "", err
	}
	root, err := canonicalPath(path)
	if err != nil {
		return "", err
	}
	if root == base || !withinRollbackRoot(base, root) {
		return "", fmt.Errorf("%w: %s", ErrVendorLockTargetEscapesRoot, path)
	}
	for parent := filepath.Dir(root); parent != base; parent = filepath.Dir(root) {
		if _, err := lstatRollbackPath(parent); !os.IsNotExist(err) {
			if err != nil {
				return "", err
			}
			break
		}
		root = parent
	}
	return root, nil
}

// validateRollbackSymlinks rejects dangling links that canonicalPath treats as missing paths.
func validateRollbackSymlinks(path string) error {
	for candidate := path; candidate != filepath.Dir(candidate); candidate = filepath.Dir(candidate) {
		info, err := lstatRollbackPath(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if _, err := filepath.EvalSymlinks(candidate); err != nil {
				return err
			}
		}
	}
	return nil
}

// withinRollbackRoot compares complete path components rather than lexical prefixes.
func withinRollbackRoot(root, path string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

// copyRollbackTree preserves links themselves and refuses entries a backup cannot reproduce.
func copyRollbackTree(source, target string) error {
	return cp.Copy(source, target, cp.Options{
		PreserveTimes: true,
		OnSymlink:     func(string) cp.SymlinkAction { return cp.Shallow },
		Skip: func(info os.FileInfo, src, _ string) (bool, error) {
			if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				return false, fmt.Errorf("%w: %s", errUnsupportedRollbackFile, src)
			}
			return false, nil
		},
	})
}

// restore reverses only this job's changes while the project and receipt locks remain held.
func (s *targetSnapshot) restore() error {
	var failures []error
	for _, entry := range s.entries {
		if err := entry.restore(); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// restore keeps an existing destination inode when possible, so a read-only parent is supported.
func (entry targetBackup) restore() error {
	if !entry.exists {
		return removeRollbackTree(entry.path)
	}
	before, err := os.Lstat(entry.backup)
	if err != nil {
		return err
	}
	if err := entry.prepareRestore(before); err != nil {
		return err
	}
	if err := copyRollbackTree(entry.backup, entry.path); err != nil {
		return err
	}
	return os.Chmod(entry.path, before.Mode())
}

// prepareRestore retains writable destination inodes when their parents are read-only.
func (entry targetBackup) prepareRestore(before os.FileInfo) error {
	current, err := os.Lstat(entry.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if before.IsDir() && current.IsDir() {
		return entry.prepareDirectoryRestore()
	}
	if before.Mode().IsRegular() && current.Mode().IsRegular() {
		return os.Chmod(entry.path, current.Mode()|0o600)
	}
	return removeRollbackTree(entry.path)
}

// lstatRollbackPath inspects one entry through a handle to its containing directory.
func lstatRollbackPath(path string) (os.FileInfo, error) {
	parent, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	return parent.Lstat(filepath.Base(path))
}

// prepareDirectoryRestore preserves regular-file inodes so hardlink aliases are restored too.
func (entry targetBackup) prepareDirectoryRestore() error {
	info, err := os.Stat(entry.path)
	if err != nil {
		return err
	}
	if err := os.Chmod(entry.path, info.Mode()|0o700); err != nil {
		return err
	}
	children, err := os.ReadDir(entry.path)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := entry.prepareChildRestore(child.Name()); err != nil {
			return err
		}
	}
	return nil
}

// prepareChildRestore removes new entries and makes surviving entries ready for restoration.
func (entry targetBackup) prepareChildRestore(name string) error {
	child := targetBackup{path: filepath.Join(entry.path, name), backup: filepath.Join(entry.backup, name)}
	before, err := os.Lstat(child.backup)
	if os.IsNotExist(err) {
		return removeRollbackTree(child.path)
	}
	if err != nil {
		return err
	}
	return child.prepareRestore(before)
}

// removeRollbackTree makes private backup directories writable without following symlinks.
func removeRollbackTree(path string) error {
	parent, err := os.OpenRoot(filepath.Dir(path))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	err = fs.WalkDir(parent.FS(), filepath.Base(path), func(relative string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return parent.Chmod(relative, info.Mode()|0o700)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return parent.RemoveAll(filepath.Base(path))
}

// close removes the private backup after a commit or a successful rollback.
func (s *targetSnapshot) close() error {
	return removeRollbackTree(s.dir)
}
