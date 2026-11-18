package discovery

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SourceCheck identifies one discovery source to a [Policy].
type SourceCheck struct {
	// Alias is the configured repository alias.
	Alias string

	// Info describes the repository.
	Info *service.RepositoryInfo

	// Kind is "collection", "named", or "catalog".
	Kind string

	// Reference is the source reference: the repository location, or the catalog reference.
	Reference string

	// Cached reports that the read is served from a snapshot rather than the source.
	Cached bool
}

// Policy decides whether a discovery source may be used now. It is evaluated on every read, so a
// policy that changed since a snapshot was written applies to the cached data too: a snapshot is
// never an authorization.
type Policy interface {
	// AuthorizeSource runs before a snapshot is served from cache and before a source is read.
	AuthorizeSource(ctx context.Context, check *SourceCheck) error

	// AuthorizeEntries runs on the entries about to be returned, cached or fresh.
	AuthorizeEntries(ctx context.Context, check *SourceCheck, entries []artifact.ListEntry) error
}

// LocationPolicy is the default policy. A repository must be readable, and every entry that came
// from a catalog must be bound to the destination repository path: a catalog is discoverable data
// and can never direct the repository's identity to another registry or path.
type LocationPolicy struct{}

var _ Policy = LocationPolicy{}

// AuthorizeSource implements [Policy].
func (LocationPolicy) AuthorizeSource(_ context.Context, check *SourceCheck) error {
	defer perf.Track(nil, "discovery.LocationPolicy.AuthorizeSource")()

	if !check.Info.Capabilities.Read {
		return artifact.NewUnsupportedCapability(check.Alias, "list", "read", "the repository is write-only")
	}
	return nil
}

// AuthorizeEntries implements [Policy].
func (LocationPolicy) AuthorizeEntries(_ context.Context, check *SourceCheck, entries []artifact.ListEntry) error {
	defer perf.Track(nil, "discovery.LocationPolicy.AuthorizeEntries")()

	if check.Kind != kindCatalog {
		return nil
	}
	location := strings.ToLower(check.Info.Discovery.Location)
	for index := range entries {
		entry := &entries[index]
		if want := location + "/" + entry.Name; entry.Pinned.Repository != want {
			return fmt.Errorf("%w: catalog entry %s:%s is bound to %q, not to the repository path %q", errUtils.ErrArtifactCatalogInvalid, entry.Name, entry.Version, entry.Pinned.Repository, want)
		}
	}
	return nil
}
