package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/cache"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// Discovery implements [service.Enumerator] over the shared discovery cache and optional catalogs.
type Discovery struct {
	scope     func() (string, error)
	directory string
	ttl       time.Duration
	now       func() time.Time
	offline   func(info *service.RepositoryInfo) bool
	policy    Policy
	maxSize   int64

	once  sync.Once
	store *cache.Store
	err   error
}

var _ service.Enumerator = (*Discovery)(nil)

// New returns a discovery cache. The scope function is called once, on first use, so building the
// service performs no file or network access; compose supplies [ScopeOf] bound to the configuration.
func New(scope func() (string, error), opts ...Option) *Discovery {
	defer perf.Track(nil, "discovery.New")()

	d := &Discovery{
		scope: scope, directory: xdg.LookupXDGCacheDir("artifacts/discovery"), ttl: defaultTTL, now: time.Now,
		offline: func(*service.RepositoryInfo) bool { return false }, policy: LocationPolicy{}, maxSize: cache.DefaultMaxSize,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(d)
		}
	}
	return d
}

// open builds the store once.
func (d *Discovery) open() (*cache.Store, error) {
	d.once.Do(func() {
		scope, err := d.scope()
		if err != nil {
			d.err = err
			return
		}
		d.store = cache.New(d.directory, scope, cache.WithTTL(d.ttl), cache.WithClock(d.now), cache.WithMaxSize(d.maxSize))
	})
	return d.store, d.err
}

// Enumerate implements [service.Enumerator].
func (d *Discovery) Enumerate(ctx context.Context, request *service.EnumerateRequest) (*service.Enumeration, error) {
	defer perf.Track(nil, "discovery.Discovery.Enumerate")()

	offline := d.offline(request.Info)
	if request.Refresh && offline {
		return nil, offlineError(request.Alias, "a refresh needs the network")
	}
	source := d.sourceOf(request)
	if !request.Info.Discovery.Remote() {
		return d.live(ctx, request, source)
	}
	store, err := d.open()
	if err != nil {
		return nil, err
	}
	loaded, cached, err := cache.Load(ctx, store, d.spec(request, source), cache.Mode{Refresh: request.Refresh, Offline: offline})
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, fmt.Errorf("%w: repository %q returned no listing", errUtils.ErrArtifactDiscoveryIncomplete, request.Alias)
	}
	if err := d.policy.AuthorizeEntries(ctx, source, loaded.Entries); err != nil {
		return nil, err
	}
	return d.result(source, loaded, cached), nil
}

// sourceOf selects what is enumerated: one name's versions, the configured catalog, or the
// collection.
func (d *Discovery) sourceOf(request *service.EnumerateRequest) *SourceCheck {
	info := request.Info
	check := &SourceCheck{Alias: request.Alias, Info: info, Kind: kindCollection, Reference: info.Discovery.Location}
	switch {
	case request.Name != "":
		check.Kind, check.Reference = kindNamed, info.Discovery.Location+"|"+request.Name
	case info.Discovery.Catalog != "":
		check.Kind, check.Reference = kindCatalog, info.Discovery.Catalog
	}
	check.Reference += "#" + fingerprint(info)
	return check
}

// fingerprintLength is how many hexadecimal characters of the settings hash name a source.
const fingerprintLength = 16

// fingerprint hashes the repository settings that change what a source contains (options and the
// bound identity name, never a credential), so reconfiguring a repository starts a new snapshot.
func fingerprint(info *service.RepositoryInfo) string {
	raw, _ := json.Marshal([]any{info.Discovery.Kind, info.Discovery.Options, info.Discovery.Identity, info.Discovery.Catalog})
	return cache.Hash(raw)[:fingerprintLength]
}

// live lists a repository that is cheap to read directly: no snapshot, no cache.
func (d *Discovery) live(ctx context.Context, request *service.EnumerateRequest, source *SourceCheck) (*service.Enumeration, error) {
	if err := d.policy.AuthorizeSource(ctx, source); err != nil {
		return nil, err
	}
	entries, err := request.Live(ctx, nil)
	if err != nil {
		return nil, err
	}
	sortEntries(entries)
	if err := d.policy.AuthorizeEntries(ctx, source, entries); err != nil {
		return nil, err
	}
	return &service.Enumeration{Entries: entries, Source: sourceLive}, nil
}

// Source names reported on a listing.
const (
	sourceLive    = "live"
	sourceCache   = "cache"
	sourceCatalog = "catalog"
)

// result describes where the entries came from.
func (d *Discovery) result(source *SourceCheck, loaded *snapshot, cached bool) *service.Enumeration {
	enumeration := &service.Enumeration{Entries: loaded.Entries, Source: sourceLive}
	if cached {
		enumeration.Source, enumeration.FetchedAt = sourceCache, loaded.FetchedAt
	}
	if source.Kind == kindCatalog {
		enumeration.Source, enumeration.FetchedAt = sourceCatalog, loaded.FetchedAt
		enumeration.Diagnostics = append(enumeration.Diagnostics, artifact.Diagnostic{
			Severity: artifact.SeverityInfo, Code: "catalog",
			Message: fmt.Sprintf("listed from the catalog %s (%s); artifacts published since the catalog was last updated are not included", source.Info.Discovery.Catalog, loaded.Digest),
		})
	}
	return enumeration
}

// spec describes the source to the shared cache.
func (d *Discovery) spec(request *service.EnumerateRequest, source *SourceCheck) *cache.Source[snapshot, *snapshot] {
	return &cache.Source[snapshot, *snapshot]{
		Kind: source.Kind, Reference: source.Reference, Version: snapshotVersion, Decode: decodeSnapshot,
		Fetch: func(ctx context.Context, previous *snapshot) (*snapshot, error) {
			if source.Kind == kindCatalog {
				return d.fetchCatalog(ctx, request, previous)
			}
			return d.fetchRepository(ctx, request, previous)
		},
		Authorize: func(ctx context.Context, cached bool) error {
			check := *source
			check.Cached = cached
			return d.policy.AuthorizeSource(ctx, &check)
		},
		ErrOffline:  offlineError(request.Alias, "there is no valid discovery snapshot for it"),
		ErrTooLarge: fmt.Errorf("%w: repository %q", errUtils.ErrArtifactDiscoveryTooLarge, request.Alias),
	}
}

// fetchRepository lists the repository completely. A failure at any page returns the error and
// caches nothing, so the previous snapshot stays authoritative and a disappeared reference is
// removed only by a refresh that completed.
func (d *Discovery) fetchRepository(ctx context.Context, request *service.EnumerateRequest, previous *snapshot) (*snapshot, error) {
	entries, err := request.Live(ctx, knownByDigest(previous))
	if err != nil {
		return nil, err
	}
	sortEntries(entries)
	return &snapshot{Entries: entries}, nil
}

// fetchCatalog reads the repository's configured catalog. When the catalog manifest digest equals
// the previous snapshot's, the previous entries are reused without reading the catalog payload. A
// configured catalog that cannot be read is an error, never an empty listing.
func (d *Discovery) fetchCatalog(ctx context.Context, request *service.EnumerateRequest, previous *snapshot) (*snapshot, error) {
	location := request.Info.Discovery.Location
	ref, err := catalog.RefOf(location, request.Info.Discovery.Catalog)
	if err != nil {
		return nil, err
	}
	repo, err := request.Open(ctx)
	if err != nil {
		return nil, err
	}
	pinned, err := catalog.Head(ctx, repo, ref)
	if err != nil {
		return nil, fmt.Errorf("the catalog %s of repository %q is not readable: %w", request.Info.Discovery.Catalog, request.Alias, err)
	}
	if previous != nil && previous.Digest != "" && digest.Digest(previous.Digest) == pinned.ResolvedDigest {
		return &snapshot{Digest: previous.Digest, Entries: previous.Entries}, nil
	}
	loaded, err := catalog.ReadPinned(ctx, repo, &pinned)
	if err != nil {
		return nil, err
	}
	if err := catalog.Validate(loaded.File, location); err != nil {
		return nil, err
	}
	entries := catalog.ListEntries(loaded.File, location, request.Alias)
	sortEntries(entries)
	return &snapshot{Digest: loaded.Digest.String(), Entries: entries}, nil
}

// offlineError builds the offline discovery error with its remedy.
func offlineError(alias, reason string) error {
	return errUtils.Build(errUtils.ErrArtifactDiscoveryOffline).
		WithExplanationf("Repository %q cannot be listed offline: %s.", alias, reason).
		WithHint("Run `atmos artifacts list` online first to create a discovery snapshot, or disable offline mode.").
		Err()
}
