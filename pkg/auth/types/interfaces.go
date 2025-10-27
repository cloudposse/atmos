package types

//go:generate go run go.uber.org/mock/mockgen@v0.5.0 -source=$GOFILE -destination=mock_interfaces_test.go -package=$GOPACKAGE

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
)

// PathType indicates what kind of filesystem entity the path represents.
type PathType string

const (
	// PathTypeFile indicates a single file (e.g., ~/.aws/credentials).
	PathTypeFile PathType = "file"
	// PathTypeDirectory indicates a directory (e.g., ~/.azure/).
	PathTypeDirectory PathType = "directory"
)

// Path represents a credential file or directory used by the provider/identity.
type Path struct {
	// Location is the filesystem path (may contain ~ for home directory).
	Location string `json:"location"`

	// Type indicates if this is a file or directory.
	Type PathType `json:"type"`

	// Required indicates if path must exist for provider to function.
	// If false, missing paths are optional (provider works without them).
	Required bool `json:"required"`

	// Purpose describes what this path is used for (helps with debugging/logging).
	// Examples: "AWS credentials file", "Azure config directory", "GCP service account key"
	Purpose string `json:"purpose"`

	// Metadata holds optional provider-specific information.
	// Consumers can use this for advanced features without breaking interface.
	// Examples:
	//   - "selinux_label": "system_u:object_r:container_file_t:s0" (future SELinux support)
	//   - "read_only": "true" (hint that path should be read-only)
	//   - "mount_target": "/workspace/.aws" (suggested container path)
	Metadata map[string]string `json:"metadata,omitempty"`
}

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

	// Paths returns credential files/directories used by this provider.
	// Returns empty slice if provider doesn't use filesystem credentials (e.g., GitHub tokens).
	// Consumers decide how to use these paths (mount, copy, delete, etc.).
	Paths() ([]Path, error)

	// Logout removes provider-specific credential storage (files, cache, etc.).
	// Returns error only if cleanup fails for critical resources.
	// Best-effort: continue cleanup even if individual steps fail.
	Logout(ctx context.Context) error

	// GetFilesDisplayPath returns the display path for credential files.
	// Returns the configured path if set, otherwise a default path.
	// For display purposes only (may use ~ for home directory).
	GetFilesDisplayPath() string
}

// PostAuthenticateParams contains parameters for PostAuthenticate method.
type PostAuthenticateParams struct {
	AuthContext  *schema.AuthContext
	StackInfo    *schema.ConfigAndStacksInfo
	ProviderName string
	IdentityName string
	Credentials  ICredentials
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

	// Paths returns credential files/directories used by this identity.
	// Returns empty slice if identity doesn't use filesystem credentials.
	// Paths are in addition to provider paths (identities can add more files).
	Paths() ([]Path, error)

	// PostAuthenticate is called after successful authentication with the final credentials.
	// It receives both authContext (to populate runtime credentials) and stackInfo (to read
	// stack-level auth configuration overrides and write environment variables).
	PostAuthenticate(ctx context.Context, params *PostAuthenticateParams) error

	// Logout removes identity-specific credential storage.
	// Best-effort: continue cleanup even if individual steps fail.
	Logout(ctx context.Context) error
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

	// GetFilesDisplayPath returns the display path for AWS files for a provider.
	// Returns the configured path if set, otherwise default ~/.aws/atmos.
	GetFilesDisplayPath(providerName string) string

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

	// Logout removes credentials for the specified identity and its authentication chain.
	// Best-effort: continues cleanup even if individual steps fail.
	Logout(ctx context.Context, identityName string) error

	// LogoutProvider removes all credentials for the specified provider.
	// Best-effort: continues cleanup even if individual steps fail.
	LogoutProvider(ctx context.Context, providerName string) error

	// LogoutAll removes all cached credentials for all identities.
	// Best-effort: continues cleanup even if individual steps fail.
	LogoutAll(ctx context.Context) error
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

	// Validate validates credentials by making an API call to the provider.
	// Returns expiration time if available, error if credentials are invalid.
	// Returns ErrNotImplemented if validation is not supported for this credential type.
	Validate(ctx context.Context) (*time.Time, error)
}

// ConsoleAccessProvider is an optional interface that providers can implement
// to support web console/browser-based login.
type ConsoleAccessProvider interface {
	// GetConsoleURL generates a web console sign-in URL using the provided credentials.
	// Returns the sign-in URL, the duration for which the URL remains valid, and any error encountered.
	GetConsoleURL(ctx context.Context, creds ICredentials, options ConsoleURLOptions) (url string, duration time.Duration, err error)

	// SupportsConsoleAccess returns true if this provider supports web console access.
	SupportsConsoleAccess() bool
}

// ConsoleURLOptions provides configuration for console URL generation.
type ConsoleURLOptions struct {
	// Destination is the specific console page to navigate to (optional).
	// For AWS: "https://console.aws.amazon.com/s3" or similar.
	// For Azure: "https://portal.azure.com/#blade/...".
	// For GCP: "https://console.cloud.google.com/...".
	Destination string

	// SessionDuration is the requested duration for the console session (how long you stay logged in).
	// Providers may have maximum limits (e.g., AWS: 12 hours).
	// Note: AWS signin tokens themselves have a fixed 15-minute expiration (time to click the link).
	SessionDuration time.Duration

	// Issuer is an optional identifier shown in the console URL (used by AWS).
	Issuer string

	// OpenInBrowser if true, automatically opens the URL in the default browser.
	OpenInBrowser bool
}
