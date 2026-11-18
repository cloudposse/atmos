package service

import (
	"context"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DiscoveryInfo is what discovery needs to know about a configured repository, without touching
// the network or resolving credentials. It never contains credential values.
type DiscoveryInfo struct {
	// Kind is the repository kind, for example "oci/registry".
	Kind string

	// Location is the credential-free collection root: "registry/prefix" for a registry, "s3://bucket/prefix"
	// for S3, and the directory for local and layout repositories.
	Location string

	// Catalog is the optional catalog reference ("oci://registry/path:tag"); empty when none is configured.
	Catalog string

	// Identity is the name of the Atmos identity bound to the repository; never a credential.
	Identity string

	// Options are the effective repository options. They hold no credentials.
	Options map[string]any
}

// Remote reports whether listing the repository crosses the network, which is what makes a
// discovery snapshot worthwhile. Local directories, archives, and layouts are always listed live.
func (d *DiscoveryInfo) Remote() bool {
	defer perf.Track(nil, "service.DiscoveryInfo.Remote")()

	return d.Kind == artifact.RepositoryKindS3 || d.Kind == artifact.RepositoryKindOCIRegistry
}

// Enumerator supplies the entries of one repository for List and Search. The service falls back to
// reading the repository directly when no enumerator is configured. An enumerator owns caching:
// when a fresh snapshot answers the request it must not call EnumerateRequest.Live or open the
// repository.
type Enumerator interface {
	// Enumerate returns the unfiltered entries of the repository (or of one name). The service
	// applies label and text filters afterward, so a snapshot holds the whole collection.
	Enumerate(ctx context.Context, request *EnumerateRequest) (*Enumeration, error)
}

// EnumerateRequest selects what an [Enumerator] lists.
type EnumerateRequest struct {
	// Alias is the configured repository alias.
	Alias string

	// Info is the repository description.
	Info *RepositoryInfo

	// Name restricts the listing to the versions of one exact name; empty enumerates the collection.
	Name string

	// Refresh bypasses a fresh snapshot.
	Refresh bool

	// Open opens the repository, resolving credentials. It is lazy so a cache hit never authenticates.
	Open func(ctx context.Context) (artifact.Repository, error)

	// Live lists the repository directly and consumes its pagination. The known entries, keyed by
	// resolved digest, let the repository reuse metadata of unchanged references.
	Live func(ctx context.Context, known map[digest.Digest]artifact.ListEntry) ([]artifact.ListEntry, error)
}

// Enumeration is the result of [Enumerator.Enumerate].
type Enumeration struct {
	// Entries are the unfiltered entries.
	Entries []artifact.ListEntry

	// Source is "live", "cache", or "catalog".
	Source string

	// FetchedAt is when a cached or catalog snapshot was fetched; zero for a live listing.
	FetchedAt time.Time

	// Diagnostics are non-fatal notes, for example that the entries come from a catalog.
	Diagnostics []artifact.Diagnostic
}

// WithEnumerator routes List and Search through an enumerator (the discovery cache and catalogs).
func WithEnumerator(enumerator Enumerator) Option {
	defer perf.Track(nil, "service.WithEnumerator")()

	return func(s *Service) { s.enumerator = enumerator }
}

// canEnumerateCollection reports whether a collection listing is possible: natively, or through a
// configured catalog read by the enumerator.
func (s *Service) canEnumerateCollection(info *RepositoryInfo) bool {
	return info.Capabilities.EnumerateCollection || s.enumerator != nil && info.Discovery.Catalog != ""
}
