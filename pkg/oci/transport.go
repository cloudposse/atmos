//nolint:gocritic // Descriptor-by-value parameters mirror the ORAS content contracts so stores stay interchangeable.
package oci

import (
	"context"
	"io"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_transport.go -package oci github.com/cloudposse/atmos/pkg/oci Store,RepositoryStore,Transport

// Repository identifies one OCI repository independently of any client library.
// Registry is the host (with optional port) and Path the repository name.
type Repository struct {
	Registry string
	Path     string
}

// ParseRepository validates "registry/path" using the same rules as the legacy client,
// so loopback registries still use plain HTTP and Docker Hub short names are normalized.
func ParseRepository(value string) (Repository, error) {
	defer perf.Track(nil, "oci.ParseRepository")()

	parsed, err := name.NewRepository(value)
	if err != nil {
		return Repository{}, wrapRegistry(err)
	}
	return repositoryOf(parsed), nil
}

func repositoryOf(repository name.Repository) Repository {
	return Repository{Registry: repository.RegistryStr(), Path: repository.RepositoryStr()}
}

// String returns "registry/path".
//
//nolint:lintroller // Trivial accessor.
func (r Repository) String() string { return r.Registry + "/" + r.Path }

// legacy converts back to the type used for credential resolution.
func (r Repository) legacy() (name.Repository, error) {
	parsed, err := name.NewRepository(r.String())
	if err != nil {
		return name.Repository{}, wrapRegistry(err)
	}
	return parsed, nil
}

// Fetcher streams content by descriptor.
type Fetcher interface {
	// Fetch streams the content identified by the descriptor.
	Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error)
}

// Store is a content-addressed OCI graph store. Its method set deliberately mirrors
// the ORAS target contract so any implementation (registry, layout, in-memory,
// legacy adapter) can participate in graph copy. Only OCI image-spec descriptors
// and go-digest values appear in the signature.
type Store interface {
	// Fetch streams the content identified by the descriptor.
	Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error)
	// Exists reports whether the content is already present.
	Exists(ctx context.Context, target ocispec.Descriptor) (bool, error)
	// Push stores content that must match the expected descriptor's size and digest.
	Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error
	// Resolve resolves a tag or digest reference to a descriptor.
	Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error)
	// Tag associates a reference with an existing manifest descriptor.
	Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error
}

// GraphStore additionally discovers the manifests that point at a descriptor, which
// is how referrers and signatures are found for extended copies.
type GraphStore interface {
	Store
	// Predecessors returns manifests whose subject or children include the node.
	Predecessors(ctx context.Context, node ocispec.Descriptor) ([]ocispec.Descriptor, error)
}

// ReferencePusher pushes a manifest directly under a tag or digest reference.
type ReferencePusher interface {
	PushReference(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error
}

// RepositoryStore is a registry repository opened with already-resolved credentials.
type RepositoryStore interface {
	GraphStore
	ReferencePusher
	// Tags lists every tag without interpreting it as a version.
	Tags(ctx context.Context) ([]string, error)
	// Delete removes a manifest by its full descriptor. It never deletes tags or shared blobs.
	Delete(ctx context.Context, target ocispec.Descriptor) error
	// Referrers lists attachments of the subject, optionally filtered by artifact type.
	// Registries without the OCI 1.1 API use the referrers tag-schema fallback.
	Referrers(ctx context.Context, subject ocispec.Descriptor, artifactType string) ([]ocispec.Descriptor, error)
}

// Transport opens repositories. Credentials are resolved when a repository is opened,
// so a failed identity fails before any network request and never downgrades to anonymous.
type Transport interface {
	Repository(ctx context.Context, repository Repository) (RepositoryStore, error)
}

// FetchBytes reads and verifies a small blob. The size limit is checked before and while
// reading; both the declared size and the digest must match, so oversized or tampered
// content never reaches callers.
func FetchBytes(ctx context.Context, store Fetcher, target ocispec.Descriptor, limit int64) ([]byte, error) {
	defer perf.Track(nil, "oci.FetchBytes")()

	if limit <= 0 || target.Size < 0 || target.Size > limit {
		return nil, errUtils.ErrOCIIntegrity
	}
	reader, err := store.Fetch(ctx, target)
	if err != nil {
		return nil, wrapRegistry(err)
	}
	defer reader.Close()
	return readVerified(reader, target, limit)
}

func readVerified(reader io.Reader, target ocispec.Descriptor, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, wrapIntegrity(err)
	}
	if int64(len(raw)) != target.Size || int64(len(raw)) > limit {
		return nil, errUtils.ErrOCIIntegrity
	}
	if err := target.Digest.Validate(); err != nil {
		return nil, wrapIntegrity(err)
	}
	verifier := target.Digest.Verifier()
	if _, err := verifier.Write(raw); err != nil || !verifier.Verified() {
		return nil, errUtils.ErrOCIIntegrity
	}
	return raw, nil
}

// Descriptor builds a descriptor for raw bytes.
func Descriptor(mediaType string, raw []byte) ocispec.Descriptor {
	defer perf.Track(nil, "oci.Descriptor")()

	return ocispec.Descriptor{MediaType: mediaType, Digest: digest.FromBytes(raw), Size: int64(len(raw))}
}
