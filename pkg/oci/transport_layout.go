//nolint:gocritic,lintroller // Descriptor-by-value parameters and thin store delegators mirror the ORAS content contracts so stores stay interchangeable.
package oci

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	layoutIndexMode       = 0o644
	maxAttachmentManifest = 4 << 20
)

// Layout is a writable OCI image layout that can hold any number of named roots.
//
// It keeps index.json limited to the roots a caller explicitly tagged. Child manifests
// and blobs live only under blobs/, exactly like the single-release layouts written by
// SaveIndex, so a layout that holds one release still satisfies LoadIndex. A layout with
// several references is intentionally rejected by LoadIndex; read it through Layout.
//
// Layout does not coordinate concurrent writers in other processes.
type Layout struct {
	root  string
	store *oci.Store
	mu    sync.Mutex
	index []ocispec.Descriptor
}

var (
	_ GraphStore         = (*Layout)(nil)
	_ ReadOnlyGraphStore = (*oci.ReadOnlyStore)(nil)
)

// OpenLayout opens or creates a layout directory.
func OpenLayout(ctx context.Context, directory string) (*Layout, error) {
	defer perf.Track(nil, "oci.OpenLayout")()

	store, err := oci.NewWithContext(ctx, directory)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	// The ORAS store would otherwise list every pushed manifest in index.json.
	store.AutoSaveIndex = false
	store.AutoGC = false
	layout := &Layout{root: filepath.Clean(directory), store: store}
	if err := layout.loadIndex(); err != nil {
		return nil, err
	}
	return layout, nil
}

// OpenLayoutReadOnly opens an existing layout without ever writing to it, so committed
// fixtures and offline media stay untouched.
func OpenLayoutReadOnly(ctx context.Context, directory string) (ReadOnlyGraphStore, error) {
	defer perf.Track(nil, "oci.OpenLayoutReadOnly")()

	store, err := oci.NewFromFS(ctx, os.DirFS(directory))
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	return store, nil
}

func (l *Layout) indexPath() string { return filepath.Join(l.root, ocispec.ImageIndexFile) }

func (l *Layout) loadIndex() error {
	raw, err := os.ReadFile(l.indexPath())
	if err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	var index ocispec.Index
	if err := json.Unmarshal(raw, &index); err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	l.index = index.Manifests
	return nil
}

// commitIndex persists entries as index.json and only then adopts them as the in-memory state, so a
// failed write leaves the layout exactly as persisted and a retry repeats the whole update. Callers
// hold l.mu.
func (l *Layout) commitIndex(entries []ocispec.Descriptor) error {
	if err := writeIndexFile(l.indexPath(), entries); err != nil {
		return err
	}
	l.index = entries
	return nil
}

func writeIndexFile(indexPath string, entries []ocispec.Descriptor) error {
	index := ocispec.Index{MediaType: ocispec.MediaTypeImageIndex, Manifests: entries}
	index.SchemaVersion = 2
	if index.Manifests == nil {
		index.Manifests = []ocispec.Descriptor{}
	}
	raw, err := json.Marshal(index)
	if err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	if err := filesystem.NewOSFileSystem().WriteFileAtomic(indexPath, raw, layoutIndexMode); err != nil {
		return errors.Join(errUtils.ErrOCILayout, err)
	}
	return nil
}

// Fetch streams a blob from the layout.
func (l *Layout) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	reader, err := l.store.Fetch(ctx, target)
	return reader, layoutError(err)
}

// Exists reports whether the blob is in the layout.
func (l *Layout) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	exists, err := l.store.Exists(ctx, target)
	return exists, layoutError(err)
}

// Push writes a digest-verified blob. Pushing existing content reports ErrAlreadyExists.
// A manifest that names a subject (a signature, SBOM, or other attachment) is also
// recorded as an unnamed root, because that is the only way its relationship to the
// subject survives reopening the layout.
func (l *Layout) Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error {
	defer perf.Track(nil, "oci.Layout.Push")()

	err := l.store.Push(ctx, expected, content)
	if err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return layoutError(err)
	}
	if recordErr := l.recordAttachment(ctx, expected); recordErr != nil {
		return recordErr
	}
	return layoutError(err)
}

func (l *Layout) recordAttachment(ctx context.Context, desc ocispec.Descriptor) error {
	if desc.MediaType != ocispec.MediaTypeImageManifest && desc.MediaType != ocispec.MediaTypeImageIndex {
		return nil
	}
	subject, err := readSubject(ctx, l.store, &desc)
	if err != nil {
		return layoutError(err)
	}
	if subject == nil {
		return nil
	}
	// The size cap protects attachment handling only; a large manifest without a subject is valid.
	if desc.Size > maxAttachmentManifest {
		return layoutError(errUtils.ErrOCIIntegrity)
	}
	return l.recordRoot(desc)
}

// recordRoot lists desc as an unnamed root in index.json unless its digest is already listed, so the
// manifest and its graph edges still resolve after the layout is reopened.
func (l *Layout) recordRoot(desc ocispec.Descriptor) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.recordRootLocked(desc)
}

func (l *Layout) recordRootLocked(desc ocispec.Descriptor) error {
	for i := range l.index {
		if l.index[i].Digest == desc.Digest {
			return nil
		}
	}
	entries := append(slices.Clone(l.index), ocispec.Descriptor{MediaType: desc.MediaType, Digest: desc.Digest, Size: desc.Size})
	return l.commitIndex(entries)
}

// Resolve resolves a reference name or digest to a descriptor.
func (l *Layout) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.Layout.Resolve")()

	resolved, err := l.store.Resolve(ctx, reference)
	return resolved, layoutError(err)
}

// Predecessors lists manifests in the layout that reference the node.
func (l *Layout) Predecessors(ctx context.Context, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	found, err := l.store.Predecessors(ctx, node)
	return found, layoutError(err)
}

// Tag records reference as a named root in index.json. A digest reference is recorded as an
// unnamed root unless the digest is already listed. An unnamed root with the same digest is named
// in place, so a legacy single-release layout keeps exactly one entry.
func (l *Layout) Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error {
	defer perf.Track(nil, "oci.Layout.Tag")()

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.store.Tag(ctx, desc, reference); err != nil {
		return layoutError(err)
	}
	if reference == desc.Digest.String() {
		// A digest reference is never named, but the root must still be listed or reopening the
		// layout loses its media type and children.
		return l.recordRootLocked(desc)
	}
	// Copy the descriptor and its annotations so root metadata (artifact type, platform, other
	// annotations) survives reopening without mutating the caller's descriptor.
	named := desc
	named.Annotations = make(map[string]string, len(desc.Annotations)+1)
	maps.Copy(named.Annotations, desc.Annotations)
	named.Annotations[ocispec.AnnotationRefName] = reference
	return l.commitIndex(withReference(slices.Clone(l.index), named))
}

func withReference(entries []ocispec.Descriptor, named ocispec.Descriptor) []ocispec.Descriptor {
	reference := named.Annotations[ocispec.AnnotationRefName]
	for i := range entries {
		if entries[i].Annotations[ocispec.AnnotationRefName] == reference {
			entries[i] = named
			return entries
		}
	}
	for i := range entries {
		if entries[i].Digest == named.Digest && entries[i].Annotations[ocispec.AnnotationRefName] == "" {
			entries[i] = named
			return entries
		}
	}
	return append(entries, named)
}

// Untag removes a named root from index.json without touching blobs, which may be shared by
// other references. Digest references are implicit and cannot be untagged.
func (l *Layout) Untag(ctx context.Context, reference string) error {
	defer perf.Track(nil, "oci.Layout.Untag")()

	l.mu.Lock()
	defer l.mu.Unlock()
	kept := l.index[:0:0]
	for i := range l.index {
		if l.index[i].Annotations[ocispec.AnnotationRefName] != reference {
			kept = append(kept, l.index[i])
		}
	}
	// Persist index.json before touching the store: a failed write leaves the handle and the file
	// unchanged, so the same call can be retried.
	if err := l.commitIndex(kept); err != nil {
		return err
	}
	if err := l.store.Untag(ctx, reference); err != nil {
		return layoutError(err)
	}
	return nil
}

// References returns the named roots in index.json in insertion order.
func (l *Layout) References() []string {
	defer perf.Track(nil, "oci.Layout.References")()

	l.mu.Lock()
	defer l.mu.Unlock()
	var references []string
	for i := range l.index {
		if reference := l.index[i].Annotations[ocispec.AnnotationRefName]; reference != "" {
			references = append(references, reference)
		}
	}
	return references
}

func layoutError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.Join(errUtils.ErrOCILayout, err)
}
