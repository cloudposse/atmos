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
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	scratchPattern = ".atmos-pull-*"
	parentDirMode  = 0o755
	treeDirName    = "tree"
	rootDirName    = "."
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
// with it. An existing destination is exchanged atomically with the staged tree. If atomic
// exchange is unavailable (including on Windows), replacing an existing directory fails without
// modifying it. The caller decides beforehand (see [InspectDestination]) whether replacement is permitted.
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
	scratchRoot, err := os.OpenRoot(scratch)
	if err != nil {
		_ = os.Remove(scratch)
		return fmt.Errorf("%w: open staging directory: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	preserveScratch := false
	defer func() {
		if !preserveScratch {
			if cleanupErr := removeScratch(scratchRoot, scratch); cleanupErr != nil {
				log.Warn("Failed to remove artifact staging directory", "path", scratch, "error", cleanupErr)
			}
		}
		_ = scratchRoot.Close()
	}()

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
	preserveScratch, err = replaceDestination(stage, dest, applyOptions(opts).replaceGuard)
	return err
}

// replaceDestination atomically exchanges the staged and existing trees. No children are
// removed or installed through the destination path, and a failed exchange leaves both intact.
// The boolean preserves scratch when a concurrent replacement cannot be safely rolled back.
func replaceDestination(stage, dest string, guard func(dest string) error) (bool, error) {
	info, err := os.Lstat(dest)
	switch {
	case os.IsNotExist(err):
		return false, rename(stage, dest)
	case err != nil:
		return false, fmt.Errorf("%w: destination %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	case info.Mode()&os.ModeSymlink != 0:
		return false, fmt.Errorf("%w: destination %q is a symlink; refusing to replace content through it", errUtils.ErrArtifactUnsafePath, dest)
	case !info.IsDir():
		return false, fmt.Errorf("%w: destination %q exists and is not a directory", errUtils.ErrArtifactConflict, dest)
	}
	bound, err := bindDestination(dest, info)
	if err != nil {
		return false, err
	}
	defer bound.root.Close()
	if guard != nil {
		if err := guard(dest); err != nil {
			return false, err
		}
	}
	if err := bound.verify(); err != nil {
		return false, err
	}
	return exchangeStagedTree(stage, dest, bound.info)
}

func exchangeStagedTree(stage, dest string, destInfo os.FileInfo) (bool, error) {
	stageInfo, err := os.Lstat(stage)
	if err != nil {
		return false, fmt.Errorf("%w: inspect staging tree: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	stageParent, err := os.Open(filepath.Dir(stage))
	if err != nil {
		return false, fmt.Errorf("%w: open staging parent: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	defer stageParent.Close()
	destParent, err := os.Open(filepath.Dir(dest))
	if err != nil {
		return false, fmt.Errorf("%w: open destination parent: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	defer destParent.Close()
	pair := exchangePair{stageParent: stageParent, stage: filepath.Base(stage), destParent: destParent, dest: filepath.Base(dest), stageInfo: stageInfo, destInfo: destInfo}
	return exchangeDestination(&pair, exchangeDirectories)
}

type exchangePair struct {
	stageParent *os.File
	stage       string
	destParent  *os.File
	dest        string
	stageInfo   os.FileInfo
	destInfo    os.FileInfo
}

type exchangeFunc func(stageParent *os.File, stage string, destParent *os.File, dest string) error

// exchangeDestination checks the identity displaced by the atomic exchange. If an attacker
// swapped the entry after verification, reverse the exchange; never remove an unrecognized tree.
func exchangeDestination(pair *exchangePair, exchange exchangeFunc) (bool, error) {
	stageParent, stage, destParent, dest := pair.stageParent, pair.stage, pair.destParent, pair.dest
	stageInfo, destInfo := pair.stageInfo, pair.destInfo
	stagingRoot, err := checkedParent(stageParent)
	if err != nil {
		return false, err
	}
	defer stagingRoot.Close()
	destinationRoot, err := checkedParent(destParent)
	if err != nil {
		return false, err
	}
	defer destinationRoot.Close()
	if !sameRootEntry(destinationRoot, dest, destInfo) {
		return false, changedDestination(filepath.Join(destParent.Name(), dest))
	}
	if err := exchange(stageParent, stage, destParent, dest); err != nil {
		return false, fmt.Errorf("%w: atomically replace destination: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	if pair.matchesExchange(stagingRoot, destinationRoot) {
		return false, nil
	}
	rollbackErr := exchange(stageParent, stage, destParent, dest)
	if rollbackErr == nil && sameRootEntry(stagingRoot, stage, stageInfo) {
		return false, changedDestination(filepath.Join(destParent.Name(), dest))
	}
	return true, errors.Join(fmt.Errorf("%w: destination changed during replacement; recovery data retained in %q", errUtils.ErrArtifactUnsafePath, stageParent.Name()), rollbackErr)
}

func (pair *exchangePair) matchesExchange(stageRoot, destRoot *os.Root) bool {
	return sameRootEntry(stageRoot, pair.stage, pair.destInfo) && sameRootEntry(destRoot, pair.dest, pair.stageInfo)
}

func sameRootEntry(root *os.Root, name string, expected os.FileInfo) bool {
	info, err := root.Lstat(name)
	return err == nil && os.SameFile(info, expected)
}

// checkedParent binds path-based inspection to the same directory as the syscall descriptor.
func checkedParent(parent *os.File) (*os.Root, error) {
	root, err := os.OpenRoot(parent.Name())
	if err != nil {
		return nil, changedDestination(parent.Name())
	}
	opened, err := parent.Stat()
	if err == nil {
		var current os.FileInfo
		current, err = root.Stat(rootDirName)
		if err == nil && os.SameFile(opened, current) {
			return root, nil
		}
	}
	_ = root.Close()
	return nil, changedDestination(parent.Name())
}

type boundDestination struct {
	path string
	root *os.Root
	info os.FileInfo
}

func bindDestination(dest string, checked os.FileInfo) (*boundDestination, error) {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return nil, fmt.Errorf("%w: open destination %q: %w", errUtils.ErrArtifactPackagingInvalid, dest, err)
	}
	opened, err := root.Stat(rootDirName)
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

// removeScratch restores owner access before removing a displaced restrictive tree. WalkDir
// never follows symlinks and Root bounds permission changes to the private staging tree.
func removeScratch(root *os.Root, scratch string) error {
	walkErr := fs.WalkDir(root.FS(), rootDirName, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return root.Chmod(path, info.Mode().Perm()|0o700)
		}
		return nil
	})
	children, readErr := fs.ReadDir(root.FS(), rootDirName)
	var removeErr error
	for _, child := range children {
		removeErr = errors.Join(removeErr, root.RemoveAll(child.Name()))
	}
	opened, statErr := root.Stat(rootDirName)
	if statErr == nil {
		var current os.FileInfo
		current, statErr = os.Lstat(scratch)
		if statErr == nil && !os.SameFile(opened, current) {
			statErr = changedDestination(scratch)
		}
	}
	closeErr := root.Close()
	if statErr == nil && closeErr == nil {
		statErr = os.Remove(scratch)
	}
	return errors.Join(walkErr, readErr, removeErr, statErr, closeErr)
}

func rename(from, to string) error {
	if err := renameExclusive(from, to); err != nil {
		return fmt.Errorf("%w: move %q into place: %w", errUtils.ErrArtifactPackagingInvalid, to, err)
	}
	return nil
}
