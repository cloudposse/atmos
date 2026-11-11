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

	"github.com/opencontainers/go-digest"
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

	// Resolve the root once and give the same absolute path to the store and the layout: the store
	// anchors itself, so a relative root here would let a later working-directory change send index
	// commits to a different directory than the blobs.
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	store, err := oci.NewWithContext(ctx, root)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCILayout, err)
	}
	// The ORAS store would otherwise list every pushed manifest in index.json.
	store.AutoSaveIndex = false
	store.AutoGC = false
	layout := &Layout{root: root, store: store}
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

// recordRootLocked is recordRoot for a caller that already holds l.mu.
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

	// The resolver and the committed index are observed under one lock, so a concurrent Tag cannot move the
	// reference between the two reads and make an existing tag look unpersisted.
	l.mu.Lock()
	defer l.mu.Unlock()
	resolved, err := l.store.Resolve(ctx, reference)
	if err != nil {
		return resolved, layoutError(err)
	}
	if !l.persistedLocked(reference, resolved.Digest) {
		// The resolver is updated before index.json is written, so a failed write leaves a tag that
		// resolves now and disappears when the layout is reopened. Never report such a tag as published.
		return ocispec.Descriptor{}, layoutError(errdef.ErrNotFound)
	}
	return resolved, nil
}

// persistedLocked reports whether a reference is backed by index.json. A digest reference always is,
// because it names content, not a tag; a named reference is only when the persisted index lists it for
// the digest it resolved to. The caller holds l.mu.
func (l *Layout) persistedLocked(reference string, resolved digest.Digest) bool {
	if _, err := digest.Parse(reference); err == nil {
		return true
	}
	for i := range l.index {
		if l.index[i].Annotations[ocispec.AnnotationRefName] == reference && l.index[i].Digest == resolved {
			return true
		}
	}
	return false
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

	// The mutex covers the resolver update and the index commit so concurrent updates to the same
	// reference are ordered: the last resolver write is also the last index write.
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
	displaced, moved := displacedByReference(l.index, reference, desc.Digest)
	entries := withReference(slices.Clone(l.index), named)
	if moved {
		var err error
		if entries, err = l.keepDisplacedAttachment(ctx, entries, displaced); err != nil {
			return err
		}
	}
	return l.commitIndex(entries)
}

// displacedByReference returns the root that reference names when it points at a different digest than
// the one about to be tagged: moving the tag replaces that root's entry in index.json.
func displacedByReference(entries []ocispec.Descriptor, reference string, tagged digest.Digest) (ocispec.Descriptor, bool) {
	for i := range entries {
		if entries[i].Annotations[ocispec.AnnotationRefName] == reference && entries[i].Digest != tagged {
			return entries[i], true
		}
	}
	return ocispec.Descriptor{}, false
}

// keepDisplacedAttachment lists a root that lost its tag as an unnamed root when it is an attachment,
// a manifest with a subject. Without its own entry the manifest would no longer be reachable from
// index.json, and reopening the layout would lose it as a referrer of its subject. A manifest without
// a subject, or one that is still listed, needs nothing.
func (l *Layout) keepDisplacedAttachment(ctx context.Context, entries []ocispec.Descriptor, displaced ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	if displaced.MediaType != ocispec.MediaTypeImageManifest && displaced.MediaType != ocispec.MediaTypeImageIndex {
		return entries, nil
	}
	for i := range entries {
		if entries[i].Digest == displaced.Digest {
			return entries, nil
		}
	}
	subject, err := readSubject(ctx, l.store, &displaced)
	if err != nil {
		return nil, layoutError(err)
	}
	if subject == nil {
		return entries, nil
	}
	return append(entries, ocispec.Descriptor{MediaType: displaced.MediaType, Digest: displaced.Digest, Size: displaced.Size}), nil
}

func withReference(entries []ocispec.Descriptor, named ocispec.Descriptor) []ocispec.Descriptor {
	reference := named.Annotations[ocispec.AnnotationRefName]
	for i := range entries {
		if entries[i].Annotations[ocispec.AnnotationRefName] == reference {
			entries[i] = named
			// An unnamed root for the same index would leave two entries for it, so the tag takes its place.
			return slices.DeleteFunc(entries, func(entry ocispec.Descriptor) bool {
				return entry.Digest == named.Digest && entry.Annotations[ocispec.AnnotationRefName] == ""
			})
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
	if err := l.store.Untag(ctx, reference); err != nil {
		return layoutError(err)
	}
	kept := l.index[:0:0]
	for i := range l.index {
		if l.index[i].Annotations[ocispec.AnnotationRefName] != reference {
			kept = append(kept, l.index[i])
		}
	}
	l.index = kept
	return l.writeIndex()
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
