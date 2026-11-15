package materialize

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

const (
	stagingDirMode  fs.FileMode = 0o700
	stagingFileMode fs.FileMode = 0o600

	// Owner bits that a temporary tree keeps so it can be read back.
	ownerFileAccess fs.FileMode = 0o400
	ownerDirAccess  fs.FileMode = 0o700
)

// tree writes verified entries below a root and remembers directory modes to apply last.
type tree struct {
	root      *os.Root
	inventory map[string]*artifact.Entry
	written   map[string]bool

	// links holds the symlinks validated so far. They are created after every other member
	// exists (see createLinks), so a platform that types a link by its target sees the target.
	links []*artifact.Entry

	// ownerAccess keeps owner read and traversal access on the final modes. The Record keeps the
	// original modes; this only protects a temporary tree that is read back before it is discarded.
	ownerAccess bool
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

// symlink records a symlink entry. The link itself is created by createLinks once every file and
// directory exists: Windows chooses between a file link and a directory link when the link is
// created, and a directory link to a target that does not exist yet becomes a file link that can
// never be traversed.
func (t *tree) symlink(entry *artifact.Entry) error {
	t.links = append(t.links, entry)
	return nil
}

// createLinks creates the recorded symlinks. A link whose target runs through another link is
// created after that link, so the target resolves to what it will be once the tree is complete.
func (t *tree) createLinks(ctx context.Context) error {
	for _, entry := range orderLinks(t.links) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.root.MkdirAll(filepath.FromSlash(path.Dir(entry.Path)), stagingDirMode); err != nil {
			return fmt.Errorf("%w: create parent of %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, err)
		}
		if err := t.root.Symlink(filepath.FromSlash(entry.Target), filepath.FromSlash(entry.Path)); err != nil {
			return fmt.Errorf("%w: this platform cannot create symlink %q -> %q (it is not silently replaced): %w", errUtils.ErrArtifactPackagingInvalid, entry.Path, entry.Target, err)
		}
	}
	return nil
}

// orderLinks sorts links by path and then moves each link after the link its target passes through.
// Links that depend on each other in a cycle keep their path order; they can never resolve anyway.
func orderLinks(links []*artifact.Entry) []*artifact.Entry {
	sorted := slices.Clone(links)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	pending, dependents := linkDependencies(sorted)
	ordered := make([]*artifact.Entry, 0, len(sorted))
	done := make([]bool, len(sorted))
	var ready []int
	for index := range sorted {
		if pending[index] == 0 {
			ready = append(ready, index)
		}
	}
	for len(ready) > 0 {
		index := ready[0]
		ready = ready[1:]
		done[index] = true
		ordered = append(ordered, sorted[index])
		ready = append(ready, releaseDependents(dependents[index], pending)...)
	}
	for index, link := range sorted {
		if !done[index] {
			ordered = append(ordered, link)
		}
	}
	return ordered
}

// linkDependencies counts, for each link, the links that must exist before it, and lists the links
// that wait on each link.
func linkDependencies(sorted []*artifact.Entry) (pending []int, dependents [][]int) {
	byPath := make(map[string]int, len(sorted))
	for index, link := range sorted {
		byPath[link.Path] = index
	}
	pending = make([]int, len(sorted))
	dependents = make([][]int, len(sorted))
	for index, link := range sorted {
		if dep, ok := linkThrough(link, byPath); ok && dep != index {
			pending[index]++
			dependents[dep] = append(dependents[dep], index)
		}
	}
	return pending, dependents
}

// releaseDependents counts one satisfied dependency for each waiting link and returns the links
// that are now ready.
func releaseDependents(waiting []int, pending []int) []int {
	var ready []int
	for _, index := range waiting {
		pending[index]--
		if pending[index] == 0 {
			ready = append(ready, index)
		}
	}
	return ready
}

// linkThrough finds the nearest link that the target of link passes through, given the cleaned
// target path relative to the archive root.
func linkThrough(link *artifact.Entry, byPath map[string]int) (int, bool) {
	target := path.Clean(path.Dir(link.Path) + "/" + link.Target)
	for target != "." && target != "/" && target != "" {
		if index, ok := byPath[target]; ok {
			return index, true
		}
		target = path.Dir(target)
	}
	return 0, false
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
		if err := t.root.Chmod(filepath.FromSlash(entryPath), t.finalMode(entry)); err != nil {
			return fmt.Errorf("%w: set mode %#o on %q: %w", errUtils.ErrArtifactPackagingInvalid, entry.Mode, entryPath, err)
		}
	}
	return nil
}

// finalMode is the mode applied to an entry: its recorded mode, plus owner access when the tree is a
// temporary one that is read back (read for files, read and traversal for directories).
func (t *tree) finalMode(entry *artifact.Entry) fs.FileMode {
	mode := fs.FileMode(entry.Mode)
	if !t.ownerAccess {
		return mode
	}
	if entry.Type == artifact.EntryDirectory {
		return mode | ownerDirAccess
	}
	return mode | ownerFileAccess
}
