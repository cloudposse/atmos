//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Format of an error that names a digest and keeps the cause.
const errFormatDigestCause = "%w: %s: %w"

// fetchBounded reads one small blob with the declared size and digest verified; a larger or
// tampered blob never reaches the caller. Oversize content is reported as too large, everything
// else that fails verification as an integrity failure.
func fetchBounded(ctx context.Context, st store, desc ocispec.Descriptor, limit int64) ([]byte, error) {
	if desc.Size > limit || desc.Size < 0 {
		return nil, fmt.Errorf("%w: %s declares %d bytes; the limit is %d", errUtils.ErrArtifactTooLarge, desc.Digest, desc.Size, limit)
	}
	raw, err := atmosoci.FetchBytes(ctx, st, desc, limit)
	switch {
	case err == nil:
		return raw, nil
	case errors.Is(err, errUtils.ErrOCIIntegrity):
		return nil, fmt.Errorf(errFormatDigestCause, errUtils.ErrArtifactIntegrityFailed, desc.Digest, err)
	case isNotFound(err):
		return nil, fmt.Errorf("%w: %s is missing: %w", errUtils.ErrArtifactIntegrityFailed, desc.Digest, err)
	default:
		return nil, fmt.Errorf(errFormatDigestCause, errUtils.ErrArtifactDownloadFailed, desc.Digest, err)
	}
}

// lazyReader opens a blob on first read and verifies size and digest as it streams: a clean EOF
// means the bytes matched their descriptor. Nothing is fetched for a stream nobody reads.
type lazyReader struct {
	ctx    context.Context
	st     store
	desc   ocispec.Descriptor
	once   sync.Once
	source io.ReadCloser
	hasher hash.Hash
	read   int64
	err    error
	done   bool
}

func newLazyReader(ctx context.Context, st store, desc ocispec.Descriptor) *lazyReader {
	return &lazyReader{ctx: ctx, st: st, desc: desc}
}

// Read implements io.Reader.
func (l *lazyReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "oci.lazyReader.Read")()

	l.once.Do(l.open)
	if l.err != nil {
		return 0, l.err
	}
	if err := l.ctx.Err(); err != nil {
		l.err = err
		return 0, err
	}
	n, err := l.source.Read(p)
	l.read += int64(n)
	_, _ = l.hasher.Write(p[:n])
	switch {
	case l.read > l.desc.Size:
		l.err = fmt.Errorf("%w: %s is larger than its declared %d bytes", errUtils.ErrArtifactIntegrityFailed, l.desc.Digest, l.desc.Size)
		return n, l.err
	case errors.Is(err, io.EOF):
		l.done = true
		if finishErr := l.finish(); finishErr != nil {
			l.err = finishErr
			return n, finishErr
		}
		return n, io.EOF
	case err != nil:
		l.err = fmt.Errorf(errFormatDigestCause, errUtils.ErrArtifactDownloadFailed, l.desc.Digest, err)
		return n, l.err
	}
	return n, nil
}

func (l *lazyReader) open() {
	l.hasher = l.desc.Digest.Algorithm().Hash()
	source, err := l.st.Fetch(l.ctx, l.desc)
	if err != nil {
		if isNotFound(err) {
			l.err = fmt.Errorf("%w: %s is missing: %w", errUtils.ErrArtifactIntegrityFailed, l.desc.Digest, err)
		} else {
			l.err = fmt.Errorf(errFormatDigestCause, errUtils.ErrArtifactDownloadFailed, l.desc.Digest, err)
		}
		return
	}
	l.source = source
}

func (l *lazyReader) finish() error {
	if l.read != l.desc.Size {
		return fmt.Errorf("%w: %s is %d bytes; its descriptor declares %d", errUtils.ErrArtifactIntegrityFailed, l.desc.Digest, l.read, l.desc.Size)
	}
	if got := digest.NewDigest(l.desc.Digest.Algorithm(), l.hasher); got != l.desc.Digest {
		return fmt.Errorf("%w: content digest %s does not match its descriptor %s", errUtils.ErrArtifactIntegrityFailed, got, l.desc.Digest)
	}
	return nil
}

// Close implements io.Closer.
func (l *lazyReader) Close() error {
	defer perf.Track(nil, "oci.lazyReader.Close")()

	l.once.Do(func() { l.err = io.ErrClosedPipe })
	if l.source == nil {
		return nil
	}
	return l.source.Close()
}

// graphSource exposes a foreign OCI graph for transfer. Every node read is verified against its
// descriptor, and the original root bytes are never rewritten.
type graphSource struct {
	st   store
	root ocispec.Descriptor
	// only, when set, narrows the traversal below an index to one selected child.
	only *ocispec.Descriptor
}

var _ artifact.GraphSource = (*graphSource)(nil)

// Root implements [artifact.GraphSource].
func (g *graphSource) Root() ocispec.Descriptor {
	defer perf.Track(nil, "oci.graphSource.Root")()

	return g.root
}

// Fetch implements [artifact.GraphSource].
func (g *graphSource) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	defer perf.Track(nil, "oci.graphSource.Fetch")()

	return newLazyReader(ctx, g.st, desc), nil
}

// Successors implements [artifact.GraphSource]: the config, layers, and children a manifest or
// index references. Other blobs have none.
func (g *graphSource) Successors(ctx context.Context, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.graphSource.Successors")()

	if !isManifestMediaType(desc.MediaType) && !isIndexMediaType(desc.MediaType) {
		return nil, nil
	}
	raw, err := fetchBounded(ctx, g.st, desc, maxManifestSize)
	if err != nil {
		return nil, err
	}
	successors, err := successorsOf(desc, raw)
	if err != nil {
		return nil, err
	}
	if g.only == nil || desc.Digest != g.root.Digest || !isIndexMediaType(desc.MediaType) {
		return successors, nil
	}
	for _, child := range successors {
		if child.Digest == g.only.Digest {
			return []ocispec.Descriptor{child}, nil
		}
	}
	return nil, fmt.Errorf("%w: the selected platform is not a child of %s", errUtils.ErrArtifactNotFound, desc.Digest)
}

// successorsOf lists the descriptors a manifest or index references.
func successorsOf(desc ocispec.Descriptor, raw []byte) ([]ocispec.Descriptor, error) {
	loaded, err := classify(desc, raw)
	if err != nil {
		return nil, err
	}
	if loaded.isIndex() {
		return append([]ocispec.Descriptor(nil), loaded.index.Manifests...), nil
	}
	successors := []ocispec.Descriptor{loaded.manifest.Config}
	return append(successors, loaded.manifest.Layers...), nil
}

// bytesReader returns a reader over data.
func bytesReader(data []byte) io.Reader { return bytes.NewReader(data) }
