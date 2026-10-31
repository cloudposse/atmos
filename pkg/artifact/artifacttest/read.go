package artifacttest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Resolve implements [artifact.Repository]. A pending reference is unavailable to readers; a
// digest reference checks the Record's embedded name.
func (r *Repository) Resolve(_ context.Context, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "artifacttest.Repository.Resolve")()

	pinned, err := r.resolve(ref)
	r.mu.Lock()
	hook := r.opts.OnResolved
	r.mu.Unlock()
	if err == nil && ref.Version != "" && hook != nil {
		hook(ref)
	}
	return pinned, err
}

func (r *Repository) resolve(ref artifact.Ref) (artifact.PinnedReference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("Resolve")
	if err := ref.Validate(); err != nil {
		return artifact.PinnedReference{}, err
	}
	if ref.IsDigest() {
		s, ok := r.records[ref.Digest]
		if !ok || s.record.Name != ref.Name {
			return artifact.PinnedReference{}, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, ref)
		}
		return r.pinnedFor(ref, s), nil
	}
	key := refKey(ref.Name, ref.Version)
	if p, isPending := r.pendings[key]; isPending {
		return artifact.PinnedReference{}, fmt.Errorf("%w: %s has a pending publication by operation %s", errUtils.ErrArtifactPublicationUnavailable, ref, p.operationID)
	}
	current, ok := r.refs[key]
	if !ok {
		return artifact.PinnedReference{}, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, ref)
	}
	return r.pinnedFor(ref, r.records[current]), nil
}

// Inspect implements [artifact.Repository]. It reads only the retained record and descriptors.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Inspect(_ context.Context, pinned artifact.PinnedReference) (*artifact.Inspection, error) {
	defer perf.Track(nil, "artifacttest.Repository.Inspect")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("Inspect")
	s, ok := r.records[pinned.RecordDigest]
	if !ok {
		return nil, fmt.Errorf("%w: record %s", errUtils.ErrArtifactNotFound, pinned.RecordDigest)
	}
	record := s.record
	pinnedOut := r.pinnedFor(pinned.Ref, s)
	inspection := &artifact.Inspection{Pinned: pinnedOut, Record: &record, Representation: r.representation(s)}
	held := r.evidence[s.recordDigest]
	for index := range held {
		inspection.Evidence = append(inspection.Evidence, held[index].Evidence)
	}
	return inspection, nil
}

// OpenDocument implements [artifact.Repository] without touching the payload.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) OpenDocument(_ context.Context, pinned artifact.PinnedReference, role artifact.DocumentRole) (*artifact.DocumentStream, error) {
	defer perf.Track(nil, "artifacttest.Repository.OpenDocument")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("OpenDocument")
	s, ok := r.records[pinned.RecordDigest]
	if !ok {
		return nil, fmt.Errorf("%w: record %s", errUtils.ErrArtifactNotFound, pinned.RecordDigest)
	}
	for index := range s.record.Documents {
		if s.record.Documents[index].Role != role {
			continue
		}
		for blobIndex := range s.blobs {
			held := &s.blobs[blobIndex]
			if held.role == artifact.BlobDocument && held.documentRole == role {
				return &artifact.DocumentStream{Document: s.record.Documents[index], Reader: newVerifyingReader(held.data, held.descriptor.Digest)}, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactDocumentMissing, role)
}

// Read implements [artifact.Repository]. It serves exactly the pinned digest; once the payload of
// a replaced or deleted record is gone it reports not found rather than the current bytes.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Read(_ context.Context, pinned artifact.PinnedReference) (*artifact.Content, error) {
	defer perf.Track(nil, "artifacttest.Repository.Read")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("Read")
	s, ok := r.records[pinned.RecordDigest]
	if !ok || !s.payloadAvailable {
		return nil, fmt.Errorf("%w: the payload of %s is no longer available", errUtils.ErrArtifactNotFound, pinned.RecordDigest)
	}
	record := s.record
	content := &artifact.Content{Record: &record, Representation: r.representation(s)}
	for index := range s.blobs {
		held := &s.blobs[index]
		content.Streams = append(content.Streams, artifact.ContentStream{
			Descriptor: held.descriptor, Role: held.role, Path: held.path, Reader: newVerifyingReader(held.data, held.descriptor.Digest),
		})
	}
	return content, nil
}

// Pin implements [artifact.Repository]. It acquires no lock and grants no trust.
func (r *Repository) Pin(ctx context.Context, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "artifacttest.Repository.Pin")()

	pinned, err := r.Resolve(ctx, ref)
	if err != nil {
		return pinned, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("Pin")
	held := r.evidence[pinned.RecordDigest]
	for index := range held {
		pinned.Evidence = append(pinned.Evidence, held[index].Evidence.Descriptor)
	}
	return pinned, nil
}

// List implements [artifact.Repository], paginating over completed references only.
func (r *Repository) List(_ context.Context, query artifact.ListQuery) (*artifact.ListPage, error) {
	defer perf.Track(nil, "artifacttest.Repository.List")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.count("List")
	if query.Name != "" && !r.caps.EnumerateNamed || query.Name == "" && !r.caps.EnumerateCollection {
		return nil, artifact.NewUnsupportedCapability(r.opts.Alias, "list", "enumerate", "enumeration is not supported")
	}
	var entries []artifact.ListEntry
	for key, recordDigest := range r.refs {
		if _, isPending := r.pendings[key]; isPending {
			continue
		}
		s := r.records[recordDigest]
		if query.Name != "" && s.record.Name != query.Name {
			continue
		}
		if !labelsMatch(s.record.Metadata.Labels, query.Labels) {
			continue
		}
		entries = append(entries, r.listEntry(s))
	}
	sort.Slice(entries, func(i, j int) bool { return listKey(&entries[i]) < listKey(&entries[j]) })
	return r.page(entries, query), nil
}

func (r *Repository) listEntry(s *stored) artifact.ListEntry {
	ref := artifact.Ref{Name: s.record.Name, Version: s.record.Version}
	return artifact.ListEntry{
		Pinned:      r.pinnedFor(ref, s),
		Name:        s.record.Name,
		Version:     s.record.Version,
		Kind:        s.record.Kind,
		Description: s.record.Metadata.Description,
		Labels:      s.record.Metadata.Labels,
		Documents:   s.record.Documents,
	}
}

func listKey(entry *artifact.ListEntry) string {
	return entry.Name + "\x00" + entry.Version
}

func (r *Repository) page(entries []artifact.ListEntry, query artifact.ListQuery) *artifact.ListPage {
	size := query.PageSize
	if size <= 0 {
		size = r.opts.PageSize
	}
	start := 0
	if query.Cursor != "" {
		start = sort.Search(len(entries), func(i int) bool { return listKey(&entries[i]) > query.Cursor })
	}
	end := start + size
	page := &artifact.ListPage{}
	if end < len(entries) {
		page.NextCursor = listKey(&entries[end-1])
	} else {
		end = len(entries)
	}
	page.Entries = append(page.Entries, entries[start:end]...)
	return page
}

func labelsMatch(have map[string]string, want []artifact.Label) bool {
	for _, label := range want {
		if value, ok := have[label.Key]; !ok || value != label.Value {
			return false
		}
	}
	return true
}

// verifyingReader verifies the digest of the bytes it yields at EOF.
type verifyingReader struct {
	reader   *bytes.Reader
	data     []byte
	expected digest.Digest
	done     bool
}

func newVerifyingReader(data []byte, expected digest.Digest) io.ReadCloser {
	return &verifyingReader{reader: bytes.NewReader(data), data: data, expected: expected}
}

func (v *verifyingReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "artifacttest.verifyingReader.Read")()

	n, err := v.reader.Read(p)
	if err == io.EOF && !v.done {
		v.done = true
		if digest.FromBytes(v.data) != v.expected {
			return n, fmt.Errorf("%w: blob does not match descriptor %s", errUtils.ErrArtifactIntegrityFailed, v.expected)
		}
	}
	return n, err
}

func (v *verifyingReader) Close() error {
	defer perf.Track(nil, "artifacttest.verifyingReader.Close")()

	return nil
}
