package factory

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	awsIdentities "github.com/cloudposse/atmos/pkg/auth/identities/aws"
	azureIdentities "github.com/cloudposse/atmos/pkg/auth/identities/azure"
	awsProviders "github.com/cloudposse/atmos/pkg/auth/providers/aws"
	azureProviders "github.com/cloudposse/atmos/pkg/auth/providers/azure"
	githubProviders "github.com/cloudposse/atmos/pkg/auth/providers/github"
	mockProviders "github.com/cloudposse/atmos/pkg/auth/providers/mock"
	mockawsProviders "github.com/cloudposse/atmos/pkg/auth/providers/mock/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProviderConstructor creates a provider from name and spec.
type ProviderConstructor func(name string, spec map[string]any) (types.Provider, error)

// IdentityConstructor creates an identity from name and principal.
type IdentityConstructor func(name string, principal map[string]any) (types.Identity, error)

// Factory manages provider and identity registration.
type Factory struct {
	providers  map[string]ProviderConstructor
	identities map[string]IdentityConstructor
}

// defaultFactory is the package-level factory with all constructors registered (e.g. GCP).
var defaultFactory *Factory

func init() {
	defaultFactory = NewFactory()
}

// NewFactory creates a new auth factory with all providers and identities registered.
func NewFactory() *Factory {
	defer perf.Track(nil, "factory.NewFactory")()

	f := &Factory{
		providers:  make(map[string]ProviderConstructor),
		identities: make(map[string]IdentityConstructor),
	}
	RegisterGCPProviders(f)
	RegisterGCPIdentities(f)
	return f
}

// RegisterProvider registers a provider constructor for a kind.
func (f *Factory) RegisterProvider(kind string, constructor ProviderConstructor) {
	f.providers[kind] = constructor
}

// RegisterIdentity registers an identity constructor for a kind.
func (f *Factory) RegisterIdentity(kind string, constructor IdentityConstructor) {
	f.identities[kind] = constructor
}

// HasProvider checks if a provider kind is registered.
func (f *Factory) HasProvider(kind string) bool {
	_, ok := f.providers[kind]
	return ok
}

// HasIdentity checks if an identity kind is registered.
func (f *Factory) HasIdentity(kind string) bool {
	_, ok := f.identities[kind]
	return ok
}

// CreateProvider creates a provider instance from kind, name, and spec.
func (f *Factory) CreateProvider(kind, name string, spec map[string]any) (types.Provider, error) {
	constructor, ok := f.providers[kind]
	if !ok {
		return nil, fmt.Errorf("%w: unknown provider kind: %s", errUtils.ErrInvalidProviderConfig, kind)
	}
	return constructor(name, spec)
}

// CreateIdentity creates an identity instance from kind, name, and principal.
func (f *Factory) CreateIdentity(kind, name string, principal map[string]any) (types.Identity, error) {
	constructor, ok := f.identities[kind]
	if !ok {
		return nil, fmt.Errorf("%w: unknown identity kind: %s", errUtils.ErrInvalidIdentityConfig, kind)
	}
	return constructor(name, principal)
}

// NewProvider creates a new provider instance based on the provider configuration.
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
	defer perf.Track(nil, "factory.NewProvider")()
	if config == nil {
		return nil, fmt.Errorf("%w: provider config is nil", errUtils.ErrInvalidAuthConfig)
	}
	if defaultFactory != nil && defaultFactory.HasProvider(config.Kind) {
		return defaultFactory.CreateProvider(config.Kind, name, config.Spec)
	}
	switch config.Kind {
	case "aws/iam-identity-center":
		return awsProviders.NewSSOProvider(name, config)
	case "aws/saml":
		return awsProviders.NewSAMLProvider(name, config)
	case "azure/cli":
		return azureProviders.NewCLIProvider(name, config)
	case "azure/device-code":
		return azureProviders.NewDeviceCodeProvider(name, config)
	case "azure/oidc":
		return azureProviders.NewOIDCProvider(name, config)
	case "github/oidc":
		return githubProviders.NewOIDCProvider(name, config)
	case "mock":
		return mockProviders.NewProvider(name, config), nil
	case "mock/aws":
		return mockawsProviders.NewProvider(name, config), nil
	default:
		return nil, fmt.Errorf("%w: unsupported provider kind: %s", errUtils.ErrInvalidProviderKind, config.Kind)
	}
}

// ConfigSetter is an optional interface for identities that need access to the full config.
// Used by GCP identities to access Via.Provider for whoami reporting.
type ConfigSetter interface {
	SetConfig(config *schema.Identity)
}

// NewIdentity creates a new identity instance based on the identity configuration.
func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
	defer perf.Track(nil, "factory.NewIdentity")()
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidAuthConfig)
	}
	if defaultFactory != nil && defaultFactory.HasIdentity(config.Kind) {
		identity, err := defaultFactory.CreateIdentity(config.Kind, name, config.Principal)
		if err != nil {
			return nil, err
		}
		// Set config if identity supports it (for Via.Provider resolution in whoami).
		if setter, ok := identity.(ConfigSetter); ok {
			setter.SetConfig(config)
		}
		return identity, nil
	}
	switch config.Kind {
	case "aws/permission-set":
		return awsIdentities.NewPermissionSetIdentity(name, config)
	case "aws/assume-role":
		return awsIdentities.NewAssumeRoleIdentity(name, config)
	case "aws/assume-root":
		return awsIdentities.NewAssumeRootIdentity(name, config)
	case "aws/user":
		return awsIdentities.NewUserIdentity(name, config)
	case "azure/subscription":
		return azureIdentities.NewSubscriptionIdentity(name, config)
	case "mock":
		return mockProviders.NewIdentity(name, config), nil
	case "mock/aws":
		return mockawsProviders.NewIdentity(name, config), nil
	default:
		return nil, fmt.Errorf("%w: unsupported identity kind: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}
}
