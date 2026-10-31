package local

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
)

// payloadStore abstracts how one repository kind lays a payload out on disk.
type payloadStore interface {
	// supports reports whether the kind can store and read the packaging.
	supports(packaging artifact.Packaging) bool

	// validate checks, before any state changes, that the staged blobs agree with the Record.
	validate(job *publishJob) error

	// stage writes the payload, documents, and sidecars of a publication into its staging directory.
	stage(ctx context.Context, t *tree, job *publishJob) error

	// install moves the staged payload into place at the resolved location, moving any previous
	// content aside first.
	install(t *tree, job *publishJob) error

	// streams opens the verified payload and document streams of a completed artifact.
	streams(ctx context.Context, t *tree, v *view, guard func() error) ([]artifact.ContentStream, error)

	// document opens one distribution document without touching the payload.
	document(t *tree, v *view, doc *artifact.Document, guard func() error) (io.ReadCloser, error)

	// verifySidecars checks the adjacent metadata copy against the stored Record bytes.
	verifySidecars(t *tree, v *view) error

	// remove deletes the files exclusively owned by the artifact and returns what it removed.
	remove(t *tree, v *view) ([]string, error)
}

// Read implements [artifact.Repository]. It opens the verified streams of exactly the pinned
// Record. The reference must still name that Record; after the caller reads the streams, the
// reference generation is rechecked, so bytes that changed underneath the reader are reported
// instead of served under the old digest.
//
//nolint:gocritic // The pinned reference is passed by value because artifact.Repository fixes the signature.
func (r *Repository) Read(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Content, error) {
	defer perf.Track(nil, "local.Repository.Read")()

	t, err := r.openTree(false)
	if err != nil {
		return nil, r.missingRoot(err, pinned.Ref)
	}
	defer t.Close()
	v, err := r.openView(ctx, t, &pinned)
	if err != nil {
		return nil, err
	}
	if err := r.store.verifySidecars(t, v); err != nil {
		return nil, err
	}
	guard := r.guard(v.ref)
	streams, err := r.store.streams(ctx, t, v, guard)
	if err != nil {
		closeStreams(streams)
		return nil, err
	}
	return &artifact.Content{
		Record:         v.record,
		Representation: v.rep.Artifact(t.absolute(v.rep.Location)),
		Streams:        streams,
	}, nil
}

// missingRoot maps an absent repository root to not found and passes other errors through.
func (r *Repository) missingRoot(err error, ref artifact.Ref) error {
	if errors.Is(err, errRootMissing) {
		return fmt.Errorf("%w: %s (the repository directory %q does not exist)", errUtils.ErrArtifactNotFound, ref, r.root)
	}
	return err
}

func closeStreams(streams []artifact.ContentStream) {
	for index := range streams {
		if streams[index].Reader != nil {
			_ = streams[index].Reader.Close()
		}
	}
}

// guard returns the generation recheck for a reader that observed ref. It reads the reference
// again and fails when the generation, state, or Record changed, because the payload location is
// mutable and a changed reference means the bytes may belong to a different version.
func (r *Repository) guard(observed *layout.Reference) func() error {
	return func() error {
		t, err := r.openTree(false)
		if err != nil {
			return fmt.Errorf("%w: the repository changed while reading %s:%s: %w", errUtils.ErrArtifactPublicationUnavailable, observed.Name, observed.Version, err)
		}
		defer t.Close()
		current, err := t.loadReference(observed.Key())
		switch {
		case err != nil:
			return err
		case current == nil:
			return fmt.Errorf("%w: %s:%s was deleted while it was being read", errUtils.ErrArtifactNotFound, observed.Name, observed.Version)
		case current.Generation != observed.Generation || !current.Available() || current.RecordDigest != observed.RecordDigest:
			return fmt.Errorf("%w: %s:%s changed while it was being read (generation %d to %d); the bytes read are discarded rather than served under the old digest",
				errUtils.ErrArtifactPublicationUnavailable, observed.Name, observed.Version, observed.Generation, current.Generation)
		}
		return nil
	}
}

// verifyingReader verifies size and digest as the content is read. At EOF it compares the digest
// and then runs the generation recheck, so a clean EOF means the bytes were intact and still
// belonged to the pinned reference.
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
	defer perf.Track(nil, "local.verifyingReader.Read")()

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
		v.failure = fmt.Errorf("%w: %w", errUtils.ErrArtifactIntegrityFailed, err)
		return n, v.failure
	}
	return n, err
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
	defer perf.Track(nil, "local.verifyingReader.Close")()

	return v.source.Close()
}

// openFile opens a payload or sidecar file, mapping a missing file to an integrity failure: the
// completed reference promised it exists.
func (t *tree) openFile(rel, what string) (io.ReadCloser, fs.FileInfo, error) {
	file, err := t.Open(native(rel))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s %q is missing or unreadable: %w", errUtils.ErrArtifactIntegrityFailed, what, rel, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: %s %q: %w", errUtils.ErrArtifactIntegrityFailed, what, rel, err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("%w: %s %q is not a regular file", errUtils.ErrArtifactIntegrityFailed, what, rel)
	}
	return file, info, nil
}

// verifyBytes compares a stored sidecar with the expected bytes.
func (t *tree) verifyBytes(rel, what string, want []byte) error {
	got, err := t.readBounded(rel, artifact.MaxRecordSize)
	if err != nil {
		return fmt.Errorf("%w: %s %q is missing or unreadable: %w", errUtils.ErrArtifactIntegrityFailed, what, rel, err)
	}
	if string(got) != string(want) {
		return fmt.Errorf("%w: %s %q does not match the stored Record", errUtils.ErrArtifactIntegrityFailed, what, rel)
	}
	return nil
}

// documentDescriptor returns the Record's descriptor for a role.
func documentDescriptor(record *artifact.Record, role artifact.DocumentRole) (*artifact.Document, bool) {
	for index := range record.Documents {
		if record.Documents[index].Role == role {
			return &record.Documents[index], true
		}
	}
	return nil, false
}

// blobDescriptor converts a Record document to the OCI-style descriptor used in streams.
func blobDescriptor(doc *artifact.Document) ocispec.Descriptor {
	return ocispec.Descriptor{MediaType: doc.MediaType, Digest: doc.Digest, Size: doc.Size}
}
