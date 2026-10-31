package artifact

import (
	"context"
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// IdentityResolver lazily resolves credentials for a named Atmos auth identity. Backends call it
// only when an operation needs credentials; constructing a repository, validating configuration,
// describing configuration, and dry runs never call it. The concrete credential type is owned by
// the backend that asked for it.
type IdentityResolver func(ctx context.Context, identity string) (any, error)

// RepositoryLookup returns another configured repository by alias, used for delegated locking.
type RepositoryLookup func(alias string) (Repository, error)

// RepositoryOptions are the construction options a factory receives.
type RepositoryOptions struct {
	// ResolveIdentity lazily resolves the repository's identity. It may be nil.
	ResolveIdentity IdentityResolver

	// LookupRepository finds another configured repository (for example a lock delegate). It may be nil.
	LookupRepository RepositoryLookup
}

// RepositoryOption configures construction of a repository.
type RepositoryOption func(*RepositoryOptions)

// WithIdentityResolver supplies the lazy identity resolver.
func WithIdentityResolver(resolver IdentityResolver) RepositoryOption {
	defer perf.Track(nil, "artifact.WithIdentityResolver")()

	return func(o *RepositoryOptions) { o.ResolveIdentity = resolver }
}

// WithRepositoryLookup supplies the lookup used to find delegated repositories.
func WithRepositoryLookup(lookup RepositoryLookup) RepositoryOption {
	defer perf.Track(nil, "artifact.WithRepositoryLookup")()

	return func(o *RepositoryOptions) { o.LookupRepository = lookup }
}

// ApplyRepositoryOptions folds construction options into a RepositoryOptions value for factories.
func ApplyRepositoryOptions(opts ...RepositoryOption) RepositoryOptions {
	defer perf.Track(nil, "artifact.ApplyRepositoryOptions")()

	var applied RepositoryOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&applied)
		}
	}
	return applied
}

// RepositoryFactory constructs a repository. It validates nothing the registry has not already
// validated, must not touch the network, resolve credentials, or create directories, and returns
// a repository whose Capabilities() is available immediately.
type RepositoryFactory func(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, opts ...RepositoryOption) (Repository, error)

type registeredKind struct {
	descriptor KindDescriptor
	factory    RepositoryFactory
}

// RepositoryRegistry maps repository kinds to descriptors and factories. There is no central
// backend switch: backend packages register their own kind from a composition root. It is safe
// for concurrent use.
type RepositoryRegistry struct {
	mu    sync.RWMutex
	kinds map[string]registeredKind
}

// NewRepositoryRegistry returns an empty registry.
func NewRepositoryRegistry() *RepositoryRegistry {
	defer perf.Track(nil, "artifact.NewRepositoryRegistry")()

	return &RepositoryRegistry{kinds: make(map[string]registeredKind)}
}

// Register adds a kind. It rejects an empty kind, a nil factory, and a kind that is already registered.
func (r *RepositoryRegistry) Register(desc *KindDescriptor, factory RepositoryFactory) error {
	defer perf.Track(nil, "artifact.RepositoryRegistry.Register")()

	if desc == nil || desc.Kind == "" {
		return fmt.Errorf("%w: repository kind must not be empty", errUtils.ErrArtifactConfigInvalid)
	}
	if err := ValidateKind(desc.Kind); err != nil {
		return fmt.Errorf("%w: repository kind %q: %w", errUtils.ErrArtifactConfigInvalid, desc.Kind, err)
	}
	if factory == nil {
		return fmt.Errorf("%w: factory for repository kind %q must not be nil", errUtils.ErrArtifactConfigInvalid, desc.Kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.kinds[desc.Kind]; exists {
		return fmt.Errorf("%w: repository kind %q is already registered", errUtils.ErrArtifactConflict, desc.Kind)
	}
	r.kinds[desc.Kind] = registeredKind{descriptor: *desc, factory: factory}
	return nil
}

// Lookup returns the descriptor of a kind: the registered descriptor when a backend registered
// the kind, otherwise the built-in V1 descriptor. The boolean is false for an unknown kind.
func (r *RepositoryRegistry) Lookup(kind string) (*KindDescriptor, bool) {
	defer perf.Track(nil, "artifact.RepositoryRegistry.Lookup")()

	r.mu.RLock()
	entry, ok := r.kinds[kind]
	r.mu.RUnlock()
	if ok {
		desc := entry.descriptor
		return &desc, true
	}
	return BuiltinKind(kind)
}

// IsRegistered reports whether a backend registered a factory for the kind.
func (r *RepositoryRegistry) IsRegistered(kind string) bool {
	defer perf.Track(nil, "artifact.RepositoryRegistry.IsRegistered")()

	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.kinds[kind]
	return ok
}

// Kinds returns the sorted names of every known kind: the registered ones plus the built-in V1 set.
func (r *RepositoryRegistry) Kinds() []string {
	defer perf.Track(nil, "artifact.RepositoryRegistry.Kinds")()

	seen := make(map[string]bool)
	for _, kind := range BuiltinKinds() {
		seen[kind] = true
	}
	r.mu.RLock()
	for kind := range r.kinds {
		seen[kind] = true
	}
	r.mu.RUnlock()
	names := make([]string, 0, len(seen))
	for kind := range seen {
		names = append(names, kind)
	}
	sort.Strings(names)
	return names
}

// New validates the repository configuration against its kind and constructs the repository
// through the registered factory. An unknown kind is a configuration error; a known V1 kind
// whose backend is not registered in this build is an unsupported-capability error.
func (r *RepositoryRegistry) New(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, opts ...RepositoryOption) (Repository, error) {
	defer perf.Track(nil, "artifact.RepositoryRegistry.New")()

	if cfg == nil {
		return nil, fmt.Errorf("%w: repository %q has no configuration", errUtils.ErrArtifactConfigInvalid, alias)
	}
	desc, known := r.Lookup(cfg.Kind)
	if !known {
		return nil, fmt.Errorf("%w: repository %q has unknown kind %q (known kinds: %v)", errUtils.ErrArtifactConfigInvalid, alias, cfg.Kind, r.Kinds())
	}
	r.mu.RLock()
	entry, registered := r.kinds[cfg.Kind]
	r.mu.RUnlock()
	if !registered {
		return nil, NewUnsupportedCapability(alias, "construct", "kind/"+cfg.Kind, "no backend is registered for this repository kind in this build")
	}
	if err := ValidateRepositoryConfig(desc, cfg); err != nil {
		return nil, fmt.Errorf("repository %q: %w", alias, err)
	}
	repo, err := entry.factory(ctx, alias, cfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("repository %q: %w", alias, err)
	}
	return repo, nil
}

// Reset removes every registered kind. It exists for tests that register fake backends.
func (r *RepositoryRegistry) Reset() {
	defer perf.Track(nil, "artifact.RepositoryRegistry.Reset")()

	r.mu.Lock()
	defer r.mu.Unlock()
	r.kinds = make(map[string]registeredKind)
}

// defaultRepositoryRegistry is the process-wide registry that backend packages register into.
var defaultRepositoryRegistry = NewRepositoryRegistry()

// DefaultRepositoryRegistry returns the process-wide registry.
func DefaultRepositoryRegistry() *RepositoryRegistry {
	defer perf.Track(nil, "artifact.DefaultRepositoryRegistry")()

	return defaultRepositoryRegistry
}

// RegisterRepositoryKind registers a backend kind in the process-wide registry. Backend packages
// call it from a composition root. Duplicate registration is an error.
func RegisterRepositoryKind(desc *KindDescriptor, factory RepositoryFactory) error {
	defer perf.Track(nil, "artifact.RegisterRepositoryKind")()

	return defaultRepositoryRegistry.Register(desc, factory)
}

// MustRegisterRepositoryKind is RegisterRepositoryKind for init functions: it panics on error.
func MustRegisterRepositoryKind(desc *KindDescriptor, factory RepositoryFactory) {
	defer perf.Track(nil, "artifact.MustRegisterRepositoryKind")()

	if err := RegisterRepositoryKind(desc, factory); err != nil {
		panic(err)
	}
}

// LookupRepositoryKind looks up a kind in the process-wide registry.
func LookupRepositoryKind(kind string) (*KindDescriptor, bool) {
	defer perf.Track(nil, "artifact.LookupRepositoryKind")()

	return defaultRepositoryRegistry.Lookup(kind)
}

// RepositoryKinds lists the known kinds of the process-wide registry.
func RepositoryKinds() []string {
	defer perf.Track(nil, "artifact.RepositoryKinds")()

	return defaultRepositoryRegistry.Kinds()
}

// NewRepository constructs a repository from the process-wide registry.
func NewRepository(ctx context.Context, alias string, cfg *schema.ArtifactRepositoryConfig, opts ...RepositoryOption) (Repository, error) {
	defer perf.Track(nil, "artifact.NewRepository")()

	return defaultRepositoryRegistry.New(ctx, alias, cfg, opts...)
}

// ResetRepositoryKindsForTesting clears the process-wide registry. Tests that register fake
// backends call it from t.Cleanup so registrations do not leak between tests.
func ResetRepositoryKindsForTesting() {
	defer perf.Track(nil, "artifact.ResetRepositoryKindsForTesting")()

	defaultRepositoryRegistry.Reset()
}
