package materialize

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	scratchPattern = ".atmos-pull-*"
	parentDirMode  = 0o755
	treeDirName    = "tree"
)

// errMismatch stops a destination walk at the first difference. It never leaves this package.
var errMismatch = errors.New("destination differs from the artifact inventory")

// DestinationState classifies an output directory relative to the artifact being pulled.
type DestinationState int

// Destination states.
const (
	// DestinationAbsent means the destination does not exist.
	DestinationAbsent DestinationState = iota

	// DestinationEmpty means the destination is an existing empty directory.
	DestinationEmpty

	// DestinationMatches means the destination already holds exactly the artifact's inventory.
	DestinationMatches

	// DestinationDiffers means the destination holds other content; replacing it requires force.
	DestinationDiffers
)

// InspectDestination classifies dest without modifying it. A symlink or non-directory
// destination is an error: a replacement must never follow a link out of the requested root.
func InspectDestination(dest string, record *artifact.Record) (DestinationState, error) {
	defer perf.Track(nil, "materialize.InspectDestination")()

	info, err := os.Lstat(dest)
	switch {
	case os.IsNotExist(err):
		return DestinationAbsent, nil
	case err != nil:
		return DestinationDiffers, fmt.Errorf("%w: destination %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	case info.Mode()&os.ModeSymlink != 0:
		return DestinationDiffers, fmt.Errorf("%w: destination %q is a symlink; refusing to replace content through it", errUtils.ErrArtifactUnsafePath, dest)
	case !info.IsDir():
		return DestinationDiffers, fmt.Errorf("%w: destination %q exists and is not a directory", errUtils.ErrArtifactConflict, dest)
	}
	children, err := os.ReadDir(dest)
	if err != nil {
		return DestinationDiffers, fmt.Errorf("%w: destination %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	}
	if len(children) == 0 {
		return DestinationEmpty, nil
	}
	if treeMatches(dest, record) {
		return DestinationMatches, nil
	}
	return DestinationDiffers, nil
}

// treeMatches reports whether dir holds exactly the Record inventory: the same paths, types, link
// targets, file sizes and digests, and (off Windows) permission bits. Any unreadable entry counts
// as a difference.
func treeMatches(dir string, record *artifact.Record) bool {
	want := make(map[string]artifact.Entry, len(record.Inventory))
	for _, entry := range record.Inventory {
		want[entry.Path] = entry
	}
	matched := 0
	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		relative, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		entry, ok := want[filepath.ToSlash(relative)]
		if !ok || !entryMatches(p, d, &entry) {
			return errMismatch
		}
		matched++
		return nil
	})
	return walkErr == nil && matched == len(want)
}

func entryMatches(p string, d fs.DirEntry, entry *artifact.Entry) bool {
	info, err := d.Info()
	if err != nil {
		return false
	}
	switch entry.Type {
	case artifact.EntryDirectory:
		return info.IsDir() && modeMatches(info, entry)
	case artifact.EntrySymlink:
		return linkMatches(p, info, entry)
	default:
		return fileMatches(p, info, entry)
	}
}

func linkMatches(p string, info os.FileInfo, entry *artifact.Entry) bool {
	target, err := os.Readlink(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0 && filepath.ToSlash(target) == entry.Target
}

func fileMatches(p string, info os.FileInfo, entry *artifact.Entry) bool {
	return info.Mode().IsRegular() && info.Size() == entry.Size && modeMatches(info, entry) && fileDigest(p) == entry.Digest
}

func modeMatches(info os.FileInfo, entry *artifact.Entry) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return uint32(info.Mode().Perm()) == entry.Mode
}

func fileDigest(p string) digest.Digest {
	file, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return ""
	}
	return digest.NewDigest(digest.SHA256, hasher)
}

// Materialize extracts content into a private staging directory beside dest and then replaces dest
// with it. Staging is removed on every path. An existing destination keeps no stale file: its
// children are removed inside the destination root only, then the staged children move in. The
// caller decides beforehand (see [InspectDestination]) whether replacing is permitted.
func Materialize(ctx context.Context, content *artifact.Content, dest string, opts ...Option) error {
	defer perf.Track(nil, "materialize.Materialize")()

	if dest == "" {
		return fmt.Errorf("%w: the destination is empty", errUtils.ErrArtifactConfigInvalid)
	}
	parent := filepath.Dir(dest)
	if err := os.MkdirAll(parent, parentDirMode); err != nil {
		return fmt.Errorf("%w: create %q: %w", errUtils.ErrArtifactPackagingInvalid, parent, err)
	}
	scratch, err := os.MkdirTemp(parent, scratchPattern)
	if err != nil {
		return fmt.Errorf("%w: create staging beside %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	}
	defer os.RemoveAll(scratch)

	stage := filepath.Join(scratch, treeDirName)
	if err := os.Mkdir(stage, stagingDirMode); err != nil {
		return fmt.Errorf("%w: create staging tree: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	if err := Extract(ctx, content, stage, scratch, opts...); err != nil {
		return err
	}
	if err := os.Chmod(stage, parentDirMode); err != nil {
		return fmt.Errorf("%w: set staging mode: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	return replaceDestination(stage, dest, applyOptions(opts).replaceGuard)
}

// replaceDestination moves the staged tree into place without merging it over old files. An
// existing destination is opened once and every later step is checked against that same directory,
// so a symlink swapped in after the initial check cannot redirect the removal or the moves.
func replaceDestination(stage, dest string, guard func(dest string) error) error {
	info, err := os.Lstat(dest)
	switch {
	case os.IsNotExist(err):
		return rename(stage, dest)
	case err != nil:
		return fmt.Errorf("%w: destination %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	case info.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("%w: destination %q is a symlink; refusing to replace content through it", errUtils.ErrArtifactUnsafePath, dest)
	case !info.IsDir():
		return fmt.Errorf("%w: destination %q exists and is not a directory", errUtils.ErrArtifactConflict, dest)
	}
	bound, err := bindDestination(dest, info)
	if err != nil {
		return err
	}
	defer bound.root.Close()
	if guard != nil {
		if err := guard(dest); err != nil {
			return err
		}
		if err := bound.verify(); err != nil {
			return err
		}
	}
	if err := clearDirectory(bound); err != nil {
		return err
	}
	return moveChildren(stage, bound)
}

// boundDestination is a destination directory opened once, with the identity it must keep.
type boundDestination struct {
	path string
	root *os.Root
	info os.FileInfo
}

// bindDestination opens dest and confirms the opened directory is the one that was checked. A
// destination swapped for a symlink or another directory between the check and the open fails.
func bindDestination(dest string, checked os.FileInfo) (*boundDestination, error) {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return nil, fmt.Errorf("%w: open destination %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("%w: inspect destination %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	}
	if !os.SameFile(checked, opened) {
		_ = root.Close()
		return nil, changedDestination(dest)
	}
	return &boundDestination{path: dest, root: root, info: opened}, nil
}

// verify confirms the destination path still names the bound directory and is not a symlink.
func (b *boundDestination) verify() error {
	now, err := os.Lstat(b.path)
	if err != nil || now.Mode()&os.ModeSymlink != 0 || !os.SameFile(now, b.info) {
		return changedDestination(b.path)
	}
	return nil
}

func changedDestination(dest string) error {
	return fmt.Errorf("%w: destination %q changed while it was being replaced", errUtils.ErrArtifactUnsafePath, dest)
}

// clearDirectory removes every child of the bound destination through its os.Root, so removal can
// never leave the directory that was checked.
func clearDirectory(bound *boundDestination) error {
	children, err := fs.ReadDir(bound.root.FS(), ".")
	if err != nil {
		return fmt.Errorf("%w: read destination %q: %w", errUtils.ErrArtifactPackagingInvalid, bound.path, err)
	}
	for _, child := range children {
		if err := bound.root.RemoveAll(child.Name()); err != nil {
			return fmt.Errorf("%w: remove stale %q: %w", errUtils.ErrArtifactPackagingInvalid, child.Name(), err)
		}
	}
	return nil
}

// moveChildren renames each staged child into the destination. The standard library cannot rename
// across two directory handles, so the destination identity is re-verified before every move.
func moveChildren(stage string, bound *boundDestination) error {
	children, err := os.ReadDir(stage)
	if err != nil {
		return fmt.Errorf("%w: read staging tree: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	for _, child := range children {
		if err := bound.verify(); err != nil {
			return err
		}
		if err := rename(filepath.Join(stage, child.Name()), filepath.Join(bound.path, child.Name())); err != nil {
			return err
		}
	}
	return nil
}

func rename(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("%w: move %q into place: %w", errUtils.ErrArtifactPackagingInvalid, to, err)
	}
	return nil
}
