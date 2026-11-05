package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultConcurrency bounds the metadata reads of one build or sync.
const DefaultConcurrency = artifact.DefaultConcurrency

// Repositories resolves configured repository aliases. [service.ConfigRepositories] and
// [service.StaticRepositories] implement it. Describe never authenticates; Open may.
type Repositories interface {
	// Describe returns the repository description without authenticating.
	Describe(alias string) (service.RepositoryInfo, error)

	// Open constructs the repository, resolving credentials as needed.
	Open(ctx context.Context, alias string) (artifact.Repository, error)
}

// Service builds and maintains the optional catalog of configured repositories. It reads artifact
// metadata only: it never downloads a payload, and it never publishes, signs, or deletes an artifact.
type Service struct {
	repos       Repositories
	clock       func() time.Time
	newID       func() string
	owner       string
	concurrency int
}

// Option customizes a [Service].
type Option func(*Service)

// WithClock injects the clock used for generation timestamps and Records.
func WithClock(clock func() time.Time) Option {
	defer perf.Track(nil, "catalog.WithClock")()

	return func(s *Service) { s.clock = clock }
}

// WithIDGenerator injects the operation identifier generator.
func WithIDGenerator(newID func() string) Option {
	defer perf.Track(nil, "catalog.WithIDGenerator")()

	return func(s *Service) { s.newID = newID }
}

// WithOwner sets the lock owner recorded in claims.
func WithOwner(owner string) Option {
	defer perf.Track(nil, "catalog.WithOwner")()

	return func(s *Service) { s.owner = owner }
}

// WithConcurrency bounds concurrent metadata reads.
func WithConcurrency(n int) Option {
	defer perf.Track(nil, "catalog.WithConcurrency")()

	return func(s *Service) { s.concurrency = n }
}

// NewService returns a catalog service over the configured repositories.
func NewService(repos Repositories, opts ...Option) *Service {
	defer perf.Track(nil, "catalog.NewService")()

	s := &Service{repos: repos, clock: time.Now, newID: uuid.NewString, concurrency: DefaultConcurrency}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	if s.concurrency < 1 {
		s.concurrency = DefaultConcurrency
	}
	if s.owner == "" {
		host, _ := os.Hostname()
		s.owner = fmt.Sprintf("%s/%d", host, os.Getpid())
	}
	return s
}

// target is a repository with its catalog configuration resolved.
type target struct {
	alias string
	info  service.RepositoryInfo
	ref   artifact.Ref
	uri   string
}

// describe resolves the catalog configuration of an alias. It touches neither the network nor any
// credential, so a mutating command fails on missing configuration before any write.
func (s *Service) describe(alias string, requireCatalog bool) (*target, error) {
	if alias == "" {
		return nil, fmt.Errorf("%w: a repository is required", errUtils.ErrArtifactConfigInvalid)
	}
	info, err := s.repos.Describe(alias)
	if err != nil {
		return nil, err
	}
	found := &target{alias: alias, info: info, uri: info.Discovery.Catalog}
	if info.Discovery.Catalog == "" {
		if requireCatalog {
			return nil, fmt.Errorf("%w: repository %q has no catalog; set `catalog` on its entry in artifacts.repositories", errUtils.ErrArtifactCatalogNotConfigured, alias)
		}
		return found, nil
	}
	if !info.Capabilities.Read {
		return nil, artifact.NewUnsupportedCapability(alias, "catalog", "read", "the repository is write-only")
	}
	found.ref, err = RefOf(info.Discovery.Location, info.Discovery.Catalog)
	if err != nil {
		return nil, err
	}
	return found, nil
}

// requireWritable additionally requires the repository to accept writes.
func (t *target) requireWritable() error {
	if !t.info.Capabilities.Write {
		return artifact.NewUnsupportedCapability(t.alias, "catalog", "write", "the repository is read-only")
	}
	return nil
}

// latest reads the current catalog; a missing catalog yields nil.
func latest(ctx context.Context, repo artifact.Repository, ref artifact.Ref) (*Loaded, error) {
	loaded, err := Read(ctx, repo, ref)
	if err != nil {
		if errors.Is(err, errUtils.ErrArtifactNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return loaded, nil
}
