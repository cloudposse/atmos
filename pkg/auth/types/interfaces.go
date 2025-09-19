package types

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Provider defines the interface that all authentication providers must implement.
type Provider interface {
	// Kind returns the provider kind (e.g., "aws/iam-identity-center").
	Kind() string
	// Name returns the provider name as defined in configuration.
	Name() string
	// PreAuthenticate allows the provider to inspect the authentication chain prior to authentication
	// so that it can set up any provider-specific preferences based on downstream identities (e.g.,
	// preferred role ARN for SAML based on the next identity in the chain).
	// Implementations should be side-effect free beyond local provider state.
	// Providers can access the current chain via manager.GetChain().
	PreAuthenticate(manager AuthManager) error
	// Authenticate performs provider-specific authentication and returns credentials.
	Authenticate(ctx context.Context) (ICredentials, error)

	// Validate validates the provider configuration.
	Validate() error

	// Environment returns environment variables that should be set for this provider.
	Environment() (map[string]string, error)
}

// Identity defines the interface that all authentication identities must implement.
type Identity interface {
	// Kind returns the identity kind (e.g., "aws/permission-set").
	Kind() string

	// GetProviderName returns the provider name for this identity.
	// AWS user identities return "aws-user", others return their via.provider.
	GetProviderName() (string, error)

	// Authenticate performs authentication using the provided base credentials.
	Authenticate(ctx context.Context, baseCreds ICredentials) (ICredentials, error)

	// Validate validates the identity configuration.
	Validate() error

	// Environment returns environment variables that should be set for this identity.
	Environment() (map[string]string, error)

	// PostAuthenticate is called after successful authentication with the final credentials.
	// Implementations can use the manager to perform provider-specific file setup or other side effects.
	PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds ICredentials) error
}

// AuthManager manages the overall authentication process.
type AuthManager interface {
	// Authenticate performs authentication for the specified identity.
	Authenticate(ctx context.Context, identityName string) (*WhoamiInfo, error)

	// Whoami returns information about the specified identity's credentials.
	Whoami(ctx context.Context, identityName string) (*WhoamiInfo, error)

	// Validate validates the entire auth configuration.
	Validate() error

	// GetDefaultIdentity returns the name of the default identity, if any.
	GetDefaultIdentity() (string, error)

	// ListIdentities returns all available identity names.
	ListIdentities() []string

	// GetProviderForIdentity returns the root provider name for the given identity.
	// Recursively resolves through identity chains to find the root provider.
	GetProviderForIdentity(identityName string) string

	// GetProviderKindForIdentity returns the provider kind for the given identity.
	GetProviderKindForIdentity(identityName string) (string, error)

	// GetChain returns the most recently constructed authentication chain
	// in the format: [providerName, identity1, identity2, ..., targetIdentity].
	GetChain() []string

	// GetStackInfo returns the current stack info pointer associated with this manager.
	GetStackInfo() *schema.ConfigAndStacksInfo

	// ListProviders returns all available provider names.
	ListProviders() []string

	// GetIdentities returns all available identity configurations.
	GetIdentities() map[string]schema.Identity

	// GetProviders returns all available provider configurations.
	GetProviders() map[string]schema.Provider
}

// CredentialStore defines the interface for storing and retrieving credentials.
type CredentialStore interface {
	// Store stores credentials for the given alias.
	Store(alias string, creds ICredentials) error

	// Retrieve retrieves credentials for the given alias.
	Retrieve(alias string) (ICredentials, error)

	// Delete deletes credentials for the given alias.
	Delete(alias string) error

	// List returns all stored credential aliases.
	List() ([]string, error)

	// IsExpired checks if credentials for the given alias are expired.
	IsExpired(alias string) (bool, error)
}

// Validator defines the interface for validating auth configurations.
type Validator interface {
	// ValidateAuthConfig validates the entire auth configuration.
	ValidateAuthConfig(config *schema.AuthConfig) error

	// ValidateProvider validates a provider configuration.
	ValidateProvider(name string, provider *schema.Provider) error

	// ValidateIdentity validates an identity configuration.
	ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error

	// ValidateChains validates identity chains for cycles and invalid references.
	ValidateChains(identities map[string]*schema.Identity, providers map[string]*schema.Provider) error
}

type ICredentials interface {
	IsExpired() bool

	GetExpiration() (*time.Time, error)

	BuildWhoamiInfo(info *WhoamiInfo)
}
