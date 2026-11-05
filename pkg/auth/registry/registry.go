package registry

import (
	"errors"
	"fmt"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProviderConstructor creates a provider from name and spec.
type ProviderConstructor func(name string, spec map[string]any) (types.Provider, error)

// IdentityConstructor creates an identity from name and principal.
type IdentityConstructor func(name string, principal map[string]any) (types.Identity, error)

// ProviderConfigConstructor receives the entire provider configuration.
type ProviderConfigConstructor func(name string, config *schema.Provider) (types.Provider, error)

// IdentityConfigConstructor receives the entire identity configuration.
type IdentityConfigConstructor func(name string, config *schema.Identity) (types.Identity, error)

// Registry manages provider and identity registration. Constructors run outside the lock.
type Registry struct {
	mu         sync.RWMutex
	providers  map[string]ProviderConfigConstructor
	identities map[string]IdentityConfigConstructor
}

// defaultRegistry dispatches built-in and plugin constructors through the same registry.
var defaultRegistry = NewRegistry()

// NewRegistry creates a registry containing every built-in provider and identity.
func NewRegistry() *Registry {
	defer perf.Track(nil, "registry.NewRegistry")()

	f := &Registry{providers: builtinProviders(), identities: builtinIdentities()}
	// These are static, disjoint registrations. A collision is a programming error.
	if err := errors.Join(RegisterGCPProviders(f), RegisterGCPIdentities(f), RegisterEmulatorIdentities(f)); err != nil {
		panic(err)
	}
	return f
}

// RegisterProvider registers a spec-based constructor without replacing an existing kind.
func (f *Registry) RegisterProvider(kind string, constructor ProviderConstructor) error {
	if constructor == nil {
		return errUtils.ErrInvalidAuthRegistration
	}
	return f.RegisterProviderConfig(kind, func(name string, config *schema.Provider) (types.Provider, error) {
		return constructor(name, config.Spec)
	})
}

// RegisterIdentity registers a principal-based constructor without replacing an existing kind.
func (f *Registry) RegisterIdentity(kind string, constructor IdentityConstructor) error {
	if constructor == nil {
		return errUtils.ErrInvalidAuthRegistration
	}
	return f.RegisterIdentityConfig(kind, func(name string, config *schema.Identity) (types.Identity, error) {
		return constructor(name, config.Principal)
	})
}

// RegisterProviderConfig registers a constructor with access to the entire configuration.
func (f *Registry) RegisterProviderConfig(kind string, constructor ProviderConfigConstructor) error {
	if kind == "" || constructor == nil {
		return errUtils.ErrInvalidAuthRegistration
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.providers[kind]; exists {
		return fmt.Errorf("%w: provider %s", errUtils.ErrDuplicateAuthRegistration, kind)
	}
	if f.providers == nil {
		f.providers = make(map[string]ProviderConfigConstructor)
	}
	f.providers[kind] = constructor
	return nil
}

// RegisterIdentityConfig registers a constructor with access to the entire configuration.
func (f *Registry) RegisterIdentityConfig(kind string, constructor IdentityConfigConstructor) error {
	if kind == "" || constructor == nil {
		return errUtils.ErrInvalidAuthRegistration
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.identities[kind]; exists {
		return fmt.Errorf("%w: identity %s", errUtils.ErrDuplicateAuthRegistration, kind)
	}
	if f.identities == nil {
		f.identities = make(map[string]IdentityConfigConstructor)
	}
	f.identities[kind] = constructor
	return nil
}

// RegisterProvider registers a provider in the process-wide auth registry.
func RegisterProvider(kind string, constructor ProviderConfigConstructor) error {
	return defaultRegistry.RegisterProviderConfig(kind, constructor)
}

// RegisterIdentity registers an identity in the process-wide auth registry.
func RegisterIdentity(kind string, constructor IdentityConfigConstructor) error {
	return defaultRegistry.RegisterIdentityConfig(kind, constructor)
}

// HasProvider checks if a provider kind is registered.
func (f *Registry) HasProvider(kind string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.providers[kind]
	return ok
}

// HasIdentity checks if an identity kind is registered.
func (f *Registry) HasIdentity(kind string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.identities[kind]
	return ok
}

// CreateProvider creates a provider using its spec.
func (f *Registry) CreateProvider(kind, name string, spec map[string]any) (types.Provider, error) {
	return f.NewProvider(name, &schema.Provider{Kind: kind, Spec: spec})
}

// CreateIdentity creates an identity using its principal.
func (f *Registry) CreateIdentity(kind, name string, principal map[string]any) (types.Identity, error) {
	return f.NewIdentity(name, &schema.Identity{Kind: kind, Principal: principal})
}

// NewProvider creates a provider without discarding legacy configuration fields.
func (f *Registry) NewProvider(name string, config *schema.Provider) (types.Provider, error) {
	defer perf.Track(nil, "registry.NewProvider")()

	if config == nil {
		return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrInvalidAuthConfig)
	}
	f.mu.RLock()
	constructor, ok := f.providers[config.Kind]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}
	return constructor(name, config)
}

// ConfigSetter is an optional interface for identities that need access to the full config.
// Used by GCP identities to access Via.Provider for whoami reporting.
type ConfigSetter interface{ SetConfig(config *schema.Identity) }

// NewIdentity creates an identity without discarding its chain, session, or credentials.
func (f *Registry) NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "registry.NewIdentity")()

	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidAuthConfig)
	}
	if config.Kind == "" {
		return nil, fmt.Errorf("%w: identity is not configured", errUtils.ErrInvalidIdentityKind)
	}
	f.mu.RLock()
	constructor, ok := f.identities[config.Kind]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}
	identity, err := constructor(name, config)
	if err != nil {
		return nil, err
	}
	// Set config if identity supports it (for Via.Provider resolution in whoami).
	if setter, ok := identity.(ConfigSetter); ok {
		setter.SetConfig(config)
	}
	return identity, nil
}

// NewProvider creates a provider through the process-wide registry.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
	return defaultRegistry.NewProvider(name, config)
}

// NewIdentity creates an identity through the process-wide registry.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
	return defaultRegistry.NewIdentity(name, config)
}
