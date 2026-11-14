package materialize

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

const (
	stagingDirMode  fs.FileMode = 0o700
	stagingFileMode fs.FileMode = 0o600
)

// tree writes verified entries below a root and remembers directory modes to apply last.
type tree struct {
	root      *os.Root
	inventory map[string]*artifact.Entry
	written   map[string]bool
}

func newTree(root *os.Root, record *artifact.Record) *tree {
	inventory := make(map[string]*artifact.Entry, len(record.Inventory))
	for index := range record.Inventory {
		inventory[record.Inventory[index].Path] = &record.Inventory[index]
	}
	return &tree{root: root, inventory: inventory, written: make(map[string]bool, len(inventory))}
}

// entryFor returns the inventory entry a member must match, rejecting unsafe or unknown paths.
func (t *tree) entryFor(name string) (*artifact.Entry, error) {
	if err := artifact.ValidatePayloadPath(name); err != nil {
		return nil, err
	}
	entry, ok := t.inventory[name]
	if !ok {
		return nil, fmt.Errorf("%w: member %q is not in the artifact inventory", errUtils.ErrArtifactIntegrityFailed, name)
	}
	if t.written[name] {
		return nil, fmt.Errorf("%w: member %q appears more than once", errUtils.ErrArtifactIntegrityFailed, name)
	}
	t.written[name] = true
	return entry, nil
}

func (t *tree) mkdir(entry *artifact.Entry) error {
	if err := t.root.MkdirAll(filepath.FromSlash(entry.Path), stagingDirMode); err != nil {
		return fmt.Errorf("%w: create directory %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, err)
	}
	return nil
}

// writeFile writes a regular file, enforcing the declared size and digest.
func (t *tree) writeFile(entry *artifact.Entry, src io.Reader) error {
	if err := t.root.MkdirAll(filepath.FromSlash(path.Dir(entry.Path)), stagingDirMode); err != nil {
		return fmt.Errorf("%w: create parent of %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, err)
	}
	file, err := t.root.OpenFile(filepath.FromSlash(entry.Path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, stagingFileMode)
	if err != nil {
		return fmt.Errorf("%w: create file %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(src, entry.Size+1))
	if closeErr := file.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return fmt.Errorf("%w: write %q: %w", errUtils.ErrArtifactIntegrityFailed, entry.Path, copyErr)
	}
	if written > entry.Size {
		return fmt.Errorf("%w: %q is larger than its declared %d bytes", errUtils.ErrArtifactTooLarge, entry.Path, entry.Size)
	}
	if written != entry.Size || digest.NewDigest(digest.SHA256, hasher) != entry.Digest {
		return fmt.Errorf("%w: %q does not match its inventory size and digest", errUtils.ErrArtifactIntegrityFailed, entry.Path)
	}
	return nil
}

func (t *tree) symlink(entry *artifact.Entry) error {
	if err := t.root.MkdirAll(filepath.FromSlash(path.Dir(entry.Path)), stagingDirMode); err != nil {
		return fmt.Errorf("%w: create parent of %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, err)
	}
	if err := t.root.Symlink(filepath.FromSlash(entry.Target), filepath.FromSlash(entry.Path)); err != nil {
		return fmt.Errorf("%w: this platform cannot create symlink %q -> %q (it is not silently replaced): %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, entry.Target, err)
	}
	return nil
}

// complete checks that every inventory entry was produced and applies final modes: files first,
// then directories deepest-first so a restrictive directory mode never blocks earlier chmods.
func (t *tree) complete() error {
	paths := make([]string, 0, len(t.inventory))
	for entryPath := range t.inventory {
		if !t.written[entryPath] {
			return fmt.Errorf("%w: inventory entry %q is missing from the content", errUtils.ErrArtifactIntegrityFailed, entryPath)
		}
		paths = append(paths, entryPath)
	}
	sort.Slice(paths, func(i, j int) bool {
		return len(paths[i]) > len(paths[j]) || (len(paths[i]) == len(paths[j]) && paths[i] > paths[j])
	})
	for _, entryPath := range paths {
		entry := t.inventory[entryPath]
		if entry.Type == artifact.EntrySymlink {
			continue
		}
		if err := t.root.Chmod(filepath.FromSlash(entryPath), fs.FileMode(entry.Mode)); err != nil {
			return fmt.Errorf("%w: set mode %#o on %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Mode, entryPath, err)
		}
	}
	return nil
}
