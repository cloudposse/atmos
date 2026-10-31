package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// filePayloadMediaType is the media type of one copied file.
const filePayloadMediaType = "application/octet-stream"

// Read implements [artifact.Repository]. It opens the verified streams of exactly the pinned
// Record. The reference must still name that Record; the streams open their objects lazily, and
// when the caller reads one to its end the reference generation is rechecked, so bytes that changed
// underneath the reader are reported instead of served under the old digest. ETags play no part:
// every stream is verified against the digest in the Record.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Read(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Content, error) {
	defer perf.Track(nil, "s3.Repository.Read")()

	v, err := r.openView(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	if err := r.verifyMetadata(ctx, v); err != nil {
		return nil, err
	}
	streams, err := r.streams(ctx, v, r.guard(ctx, v.ref.ref))
	if err != nil {
		closeStreams(streams)
		return nil, err
	}
	return &artifact.Content{Record: v.record, Representation: v.rep.Artifact(r.uri(v.rep.Location)), Streams: streams}, nil
}

func closeStreams(streams []artifact.ContentStream) {
	for index := range streams {
		if streams[index].Reader != nil {
			_ = streams[index].Reader.Close()
		}
	}
}

// metadataKey is the adjacent metadata copy of the Record next to the payload.
func metadataKey(rep *layout.Representation) string {
	if isArchive(rep.Packaging) {
		return layout.MetadataSidecarPath(rep.Location)
	}
	return layout.DistributionPath(rep.Location, layout.ManifestName)
}

// documentKey is the object that holds a distribution document.
func documentKey(rep *layout.Representation, role artifact.DocumentRole) string {
	if isArchive(rep.Packaging) {
		return layout.DocumentSidecarPath(rep.Location, role)
	}
	return layout.DistributionPath(rep.Location, layout.DocumentFileName(role))
}

// verifyMetadata checks the adjacent metadata copy against the stored Record bytes.
func (r *Repository) verifyMetadata(ctx context.Context, v *view) error {
	key := metadataKey(v.rep)
	got, _, err := r.readObject(ctx, key, artifact.MaxRecordSize)
	switch {
	case err != nil:
		return err
	case got == nil:
		return fmt.Errorf("%w: metadata %s is missing", errUtils.ErrArtifactIntegrityFailed, r.uri(key))
	case !bytes.Equal(got, v.recordBytes):
		return fmt.Errorf("%w: metadata %s does not match the stored Record", errUtils.ErrArtifactIntegrityFailed, r.uri(key))
	}
	return nil
}

// streams opens the verified payload and document streams of a completed artifact.
func (r *Repository) streams(ctx context.Context, v *view, guard func() error) ([]artifact.ContentStream, error) {
	var streams []artifact.ContentStream
	if isArchive(v.rep.Packaging) {
		if len(v.rep.Payload) != 1 {
			return nil, fmt.Errorf("%w: an archive representation must have exactly one payload, found %d", errUtils.ErrArtifactIntegrityFailed, len(v.rep.Payload))
		}
		descriptor := v.rep.Payload[0]
		streams = append(streams, artifact.ContentStream{
			Descriptor: descriptor, Role: artifact.BlobPayload,
			Reader: r.objectReader(ctx, v.rep.Location, descriptor.Digest, descriptor.Size, guard),
		})
	}
	for index := range v.record.Inventory {
		entry := &v.record.Inventory[index]
		if entry.Type != artifact.EntryFile || isArchive(v.rep.Packaging) {
			continue
		}
		streams = append(streams, artifact.ContentStream{
			Descriptor: ocispec.Descriptor{MediaType: filePayloadMediaType, Digest: entry.Digest, Size: entry.Size},
			Role:       artifact.BlobPayload,
			Path:       entry.Path,
			Reader:     r.objectReader(ctx, v.rep.Location+"/"+entry.Path, entry.Digest, entry.Size, guard),
		})
	}
	for index := range v.record.Documents {
		doc := &v.record.Documents[index]
		streams = append(streams, artifact.ContentStream{
			Descriptor: ocispec.Descriptor{MediaType: doc.MediaType, Digest: doc.Digest, Size: doc.Size},
			Role:       artifact.BlobDocument,
			Reader:     r.objectReader(ctx, documentKey(v.rep, doc.Role), doc.Digest, doc.Size, guard),
		})
	}
	return streams, nil
}

// guard returns the generation recheck for a reader that observed ref. It reads the reference
// again and fails when the generation, state, or Record changed, because the payload location is
// mutable and a changed reference means the bytes may belong to a different version.
func (r *Repository) guard(ctx context.Context, observed *layout.Reference) func() error {
	return func() error {
		current, err := r.loadReference(ctx, observed.Key())
		switch {
		case err != nil:
			return err
		case current == nil:
			return fmt.Errorf("%w: %s:%s was deleted while it was being read", errUtils.ErrArtifactNotFound, observed.Name, observed.Version)
		case current.ref.Generation != observed.Generation || !current.ref.Available() || current.ref.RecordDigest != observed.RecordDigest:
			return fmt.Errorf("%w: %s:%s changed while it was being read (generation %d to %d); the bytes read are discarded rather than served under the old digest",
				errUtils.ErrArtifactPublicationUnavailable, observed.Name, observed.Version, observed.Generation, current.ref.Generation)
		}
		return nil
	}
}

// objectReader returns a verifying reader over one payload or document object. The object is
// requested when the first byte is read, so opening many streams holds no connections.
func (r *Repository) objectReader(ctx context.Context, key string, want digest.Digest, size int64, guard func() error) io.ReadCloser {
	return newVerifyingReader(&lazyObject{repo: r, ctx: ctx, key: key, size: size}, want, size, guard)
}

// OpenDocument implements [artifact.Repository] without reading the payload. The reader verifies
// the document digest and size and rechecks the reference generation at EOF.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) OpenDocument(ctx context.Context, pinned artifact.PinnedReference, role artifact.DocumentRole) (*artifact.DocumentStream, error) {
	defer perf.Track(nil, "s3.Repository.OpenDocument")()

	v, err := r.openView(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	doc, ok := documentDescriptor(v.record, role)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactDocumentMissing, role)
	}
	reader := r.objectReader(ctx, documentKey(v.rep, role), doc.Digest, doc.Size, r.guard(ctx, v.ref.ref))
	return &artifact.DocumentStream{Document: *doc, Reader: reader}, nil
}

// lazyObject requests an object on first read and checks its size against the declared one.
type lazyObject struct {
	repo   *Repository
	ctx    context.Context
	key    string
	size   int64
	body   io.ReadCloser
	opened bool
	err    error
}

// Read implements io.Reader.
func (l *lazyObject) Read(p []byte) (int, error) {
	defer perf.Track(nil, "s3.lazyObject.Read")()

	if !l.opened {
		l.opened = true
		l.err = l.open()
	}
	if l.err != nil {
		return 0, l.err
	}
	return l.body.Read(p)
}

func (l *lazyObject) open() error {
	object, err := l.repo.store.Get(l.ctx, l.key, blob.GetOptions{})
	switch {
	case blob.IsNotFound(err):
		return fmt.Errorf("%w: object %s is missing", errUtils.ErrArtifactIntegrityFailed, l.repo.uri(l.key))
	case err != nil:
		return l.repo.mapErr(opRead, l.key, err)
	case object.Size != l.size:
		_ = object.Body.Close()
		return fmt.Errorf("%w: object %s is %d bytes; its descriptor declares %d", errUtils.ErrArtifactIntegrityFailed, l.repo.uri(l.key), object.Size, l.size)
	}
	l.body = object.Body
	return nil
}

// Close implements io.Closer.
func (l *lazyObject) Close() error {
	defer perf.Track(nil, "s3.lazyObject.Close")()

	if l.body == nil {
		return nil
	}
	return l.body.Close()
}

// verifyingReader verifies size and digest as the content flows through. At EOF it compares the
// digest and then runs the optional generation recheck, so a clean EOF means the bytes were intact
// and still belonged to the pinned reference. On a mismatch the final read reports the failure
// instead of io.EOF, which also makes it abort an upload that is consuming the reader.
type verifyingReader struct {
	source   io.ReadCloser
	hasher   hash.Hash
	want     digest.Digest
	size     int64
	read     int64
	guard    func() error
	finished bool
	failure  error
}

func newVerifyingReader(source io.ReadCloser, want digest.Digest, size int64, guard func() error) *verifyingReader {
	return &verifyingReader{source: source, hasher: want.Algorithm().Hash(), want: want, size: size, guard: guard}
}

// Read implements io.Reader.
func (v *verifyingReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "s3.verifyingReader.Read")()

	if v.failure != nil {
		return 0, v.failure
	}
	n, err := v.source.Read(p)
	v.read += int64(n)
	_, _ = v.hasher.Write(p[:n])
	switch {
	case v.read > v.size:
		v.failure = fmt.Errorf("%w: content is larger than its declared %d bytes", errUtils.ErrArtifactIntegrityFailed, v.size)
		return n, v.failure
	case errors.Is(err, io.EOF) && !v.finished:
		v.finished = true
		if verr := v.finish(); verr != nil {
			v.failure = verr
			return n, verr
		}
	case err != nil && !errors.Is(err, io.EOF):
		v.failure = integrityOrCause(err)
		return n, v.failure
	}
	return n, err
}

// integrityOrCause keeps typed artifact and context errors and marks anything else as an integrity
// failure of the read.
func integrityOrCause(err error) error {
	if isArtifactError(err) || blob.IsNotFound(err) {
		return err
	}
	return fmt.Errorf("%w: %w", errUtils.ErrArtifactIntegrityFailed, err)
}

func (v *verifyingReader) finish() error {
	if v.read != v.size {
		return fmt.Errorf("%w: content is %d bytes; its descriptor declares %d", errUtils.ErrArtifactIntegrityFailed, v.read, v.size)
	}
	if got := digest.NewDigest(v.want.Algorithm(), v.hasher); got != v.want {
		return fmt.Errorf("%w: content digest %s does not match its descriptor %s", errUtils.ErrArtifactIntegrityFailed, got, v.want)
	}
	if v.guard != nil {
		return v.guard()
	}
	return nil
}

// Close implements io.Closer.
func (v *verifyingReader) Close() error {
	defer perf.Track(nil, "s3.verifyingReader.Close")()

	return v.source.Close()
}
