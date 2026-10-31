//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"fmt"
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Repository is an OCI-backed [artifact.Repository]. One implementation serves both oci/registry
// and oci/layout; the backend decides where content lives and how versions are named.
//
// A Repository is safe for concurrent use. It keeps two small in-memory tables, both derived
// only from verified content: a bounded cache of manifests and configs addressed by name and
// digest, and the roots it has resolved, which locate the manifest a signature subject is
// attached to.
type Repository struct {
	alias    string
	be       backend
	settings settings
	delegate string
	lookup   artifact.RepositoryLookup

	mu     sync.Mutex
	cache  map[string][]byte
	order  []string
	roots  map[digest.Digest][]located
	pinned map[string]ocispec.Descriptor
}

var (
	_ artifact.Repository     = (*Repository)(nil)
	_ artifact.Copier         = (*Repository)(nil)
	_ artifact.EvidenceReader = (*Repository)(nil)
	_ artifact.EvidenceWriter = (*Repository)(nil)
)

// located is a root the repository resolved: the artifact name it was found under and its
// descriptor. Signature subjects carry only a digest, so this is how Sign and Verify find the
// manifest an attachment belongs to.
type located struct {
	name string
	desc ocispec.Descriptor
}

func newRepositoryCore(alias string, be backend, applied settings, delegate string, lookup artifact.RepositoryLookup) *Repository {
	return &Repository{
		alias: alias, be: be, settings: applied, delegate: delegate, lookup: lookup,
		cache: map[string][]byte{}, roots: map[digest.Digest][]located{}, pinned: map[string]ocispec.Descriptor{},
	}
}

// Capabilities implements [artifact.Repository]. It needs no network access. Repository-scope
// locks are not advertised here: an OCI tag is not a distributed lease, so that scope exists only
// through a configured lock delegate, which the service adds when it describes the repository.
func (r *Repository) Capabilities() artifact.Capabilities {
	defer perf.Track(nil, "oci.Repository.Capabilities")()

	return staticCapabilities(r.be.kind(), r.settings.verifier, r.be.enumeratesCollection())
}

// staticCapabilities computes the capabilities of a kind without I/O. Evidence formats come from
// the verifier: a repository that cannot verify a format does not claim to store it.
func staticCapabilities(kind string, verifier artifact.Verifier, enumerates bool) artifact.Capabilities {
	caps := artifact.Capabilities{
		Read:                true,
		Write:               true,
		EnumerateNamed:      true,
		EnumerateCollection: enumerates,
		Delete:              artifact.DeleteManifest,
		ConditionalUpdate:   false,
		Packagings:          []artifact.Packaging{artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip},
		PreservesModes:      true,
		LockScopes:          []artifact.LockScope{artifact.LockScopeHost},
	}
	if kind == artifact.RepositoryKindOCILayout {
		caps.Delete = artifact.DeleteReference
	}
	if verifier != nil {
		caps.SubjectKinds = []artifact.SubjectKind{artifact.SubjectRecord, artifact.SubjectOCIRoot}
		caps.EvidenceFormats = append([]artifact.EvidenceFormat(nil), verifier.Formats()...)
	}
	return caps
}

// cacheKey scopes a cached object to the artifact name it was read under, so content proven to
// exist in one repository path is never assumed to exist in another.
func cacheKey(name string, desc ocispec.Descriptor) string {
	return name + "@" + desc.Digest.String()
}

// cached returns a cached verified blob.
func (r *Repository) cached(name string, desc ocispec.Descriptor) ([]byte, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	raw, ok := r.cache[cacheKey(name, desc)]
	return raw, ok
}

// remember stores a verified blob, evicting the oldest entry beyond the bound.
func (r *Repository) remember(name string, desc ocispec.Descriptor, raw []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := cacheKey(name, desc)
	if _, present := r.cache[key]; !present {
		r.order = append(r.order, key)
		if len(r.order) > manifestCacheEntries {
			delete(r.cache, r.order[0])
			r.order = r.order[1:]
		}
	}
	r.cache[key] = raw
}

// track records a resolved root so signature subjects can be located later.
func (r *Repository) track(name string, loaded *root) {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := []digest.Digest{loaded.desc.Digest}
	if loaded.class == classAtmos {
		keys = append(keys, loaded.manifest.Config.Digest)
	}
	for _, key := range keys {
		known := r.roots[key]
		duplicate := false
		for _, item := range known {
			duplicate = duplicate || (item.name == name && item.desc.Digest == loaded.desc.Digest)
		}
		if !duplicate {
			r.roots[key] = append(known, located{name: name, desc: loaded.desc})
		}
	}
	r.pinned[cacheKey(name, loaded.desc)] = loaded.desc
}

// descriptorFor returns the descriptor remembered for a resolved digest of a name.
func (r *Repository) descriptorFor(name string, dgst digest.Digest) (ocispec.Descriptor, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	desc, ok := r.pinned[name+"@"+dgst.String()]
	return desc, ok
}

// locations returns every root known for a subject digest.
func (r *Repository) locations(dgst digest.Digest) []located {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]located(nil), r.roots[dgst]...)
}

// loadCachedRoot loads a root through the verified cache.
func (r *Repository) loadCachedRoot(ctx context.Context, st store, name string, desc ocispec.Descriptor) (*root, error) {
	if raw, ok := r.cached(name, desc); ok {
		return classify(desc, raw)
	}
	loaded, err := loadRoot(ctx, st, desc)
	if err != nil {
		return nil, err
	}
	r.remember(name, desc, loaded.raw)
	r.track(name, loaded)
	return loaded, nil
}

// readRecord fetches the Record config blob of an Atmos package with a bounded, verified read.
func (r *Repository) readRecord(ctx context.Context, st store, name string, loaded *root) (*artifact.Record, error) {
	config := loaded.manifest.Config
	if raw, ok := r.cached(name, config); ok {
		return artifact.DecodeRecord(raw)
	}
	raw, err := fetchBounded(ctx, st, config, artifact.MaxRecordSize)
	if err != nil {
		return nil, fmt.Errorf("read the Record of %s: %w", loaded.desc.Digest, err)
	}
	record, err := artifact.DecodeRecord(raw)
	if err != nil {
		return nil, err
	}
	r.remember(name, config, raw)
	return record, nil
}

// requireSubjectOf checks a Record against the reference that led to it: the Record is bound to
// its own name and version, so a tag or repository path serving another Record is tampering.
func requireSubjectOf(record *artifact.Record, ref artifact.Ref) error {
	if record.Name != ref.Name {
		return fmt.Errorf("%w: %s holds the Record of %q", errUtils.ErrArtifactIntegrityFailed, ref, record.Name)
	}
	if ref.Version != "" && record.Version != ref.Version {
		return fmt.Errorf("%w: %s holds the Record of version %q", errUtils.ErrArtifactIntegrityFailed, ref, record.Version)
	}
	return nil
}
