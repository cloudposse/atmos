package types

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// CloudProvider defines the interface for cloud-specific operations
type CloudProvider interface {
	// GetName returns the name of the cloud provider (e.g., "aws", "azure", "gcp")
	GetName() string

	// SetupEnvironment sets up cloud-specific environment variables and files
	SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *schema.Credentials) error

	// GetEnvironmentVariables returns cloud-specific environment variables for tools like Terraform
	GetEnvironmentVariables(providerName, identityName string) (map[string]string, error)

	// Cleanup removes temporary files and resources created by this provider
	Cleanup(ctx context.Context, providerName, identityName string) error

	// ValidateCredentials validates that the provided credentials are valid for this cloud provider
	ValidateCredentials(ctx context.Context, credentials *schema.Credentials) error

	// GetCredentialFilePaths returns the paths to credential files managed by this provider
	GetCredentialFilePaths(providerName string) map[string]string
}

// CloudProviderFactory manages cloud provider instances
type CloudProviderFactory interface {
	// GetCloudProvider returns the appropriate cloud provider for the given provider kind
	GetCloudProvider(providerKind string) (CloudProvider, error)

	// RegisterCloudProvider registers a new cloud provider
	RegisterCloudProvider(name string, provider CloudProvider)
}

// CloudProviderManager provides high-level cloud provider operations
type CloudProviderManager interface {
	// SetupEnvironment sets up cloud environment for the given provider kind and identity
	SetupEnvironment(ctx context.Context, providerKind, providerName, identityName string, credentials *schema.Credentials) error

	// GetEnvironmentVariables returns environment variables for the given provider kind and identity
	GetEnvironmentVariables(providerKind, providerName, identityName string) (map[string]string, error)

	// Cleanup removes temporary resources for the given provider kind and identity
	Cleanup(ctx context.Context, providerKind, providerName, identityName string) error
}

// Provider defines the interface that all authentication providers must implement
type Provider interface {
	// Kind returns the provider kind (e.g., "aws/iam-identity-center")
	Kind() string

	// Authenticate performs the authentication process and returns credentials
	Authenticate(ctx context.Context) (*schema.Credentials, error)

	// Validate validates the provider configuration
	Validate() error

	// Environment returns environment variables that should be set for this provider
	Environment() (map[string]string, error)
}

// Identity defines the interface that all authentication identities must implement
type Identity interface {
	// Kind returns the identity kind (e.g., "aws/permission-set")
	Kind() string

	// GetProviderName returns the provider name for this identity
	// AWS user identities return "aws-user", others return their via.provider
	GetProviderName() (string, error)

	// Authenticate performs authentication using the provided base credentials
	Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error)

	// Validate validates the identity configuration
	Validate() error

	// Environment returns environment variables that should be set for this identity
	Environment() (map[string]string, error)
}

// PostAuthHook defines an optional interface that identities can implement
// to perform actions after successful authentication
type PostAuthHook interface {
	// PostAuthenticate is called after successful authentication with the final credentials
	PostAuthenticate(ctx context.Context, providerName, identityName string, creds *schema.Credentials) error
}

// AuthManager manages the overall authentication process
type AuthManager interface {
	// Authenticate performs authentication for the specified identity
	Authenticate(ctx context.Context, identityName string) (*schema.WhoamiInfo, error)

	// Whoami returns information about the specified identity's credentials
	Whoami(ctx context.Context, identityName string) (*schema.WhoamiInfo, error)

	// Validate validates the entire auth configuration
	Validate() error

	// GetDefaultIdentity returns the name of the default identity, if any
	GetDefaultIdentity() (string, error)

	// ListIdentities returns all available identity names
	ListIdentities() []string

	// GetProviderForIdentity returns the root provider name for the given identity
	// Recursively resolves through identity chains to find the root provider
	GetProviderForIdentity(identityName string) string

	// GetProviderKindForIdentity returns the provider kind for the given identity
	GetProviderKindForIdentity(identityName string) (string, error)

	// ListProviders returns all available provider names
	ListProviders() []string
}

// CredentialStore defines the interface for storing and retrieving credentials
type CredentialStore interface {
	// Store stores credentials for the given alias
	Store(alias string, creds *schema.Credentials) error

	// Retrieve retrieves credentials for the given alias
	Retrieve(alias string) (*schema.Credentials, error)

	// Delete deletes credentials for the given alias
	Delete(alias string) error

	// List returns all stored credential aliases
	List() ([]string, error)

	// IsExpired checks if credentials for the given alias are expired
	IsExpired(alias string) (bool, error)
}

// AWSFileManager manages AWS credentials and config files
type AWSFileManager interface {
	// WriteCredentials writes AWS credentials to the provider-specific file with identity profile
	WriteCredentials(providerName, identityName string, creds *schema.AWSCredentials) error

	// WriteConfig writes AWS config to the provider-specific file with identity profile
	WriteConfig(providerName, identityName, region, outputFormat string) error

	// GetCredentialsPath returns the path to the credentials file for the provider
	GetCredentialsPath(providerName string) string

	// GetConfigPath returns the path to the config file for the provider
	GetConfigPath(providerName string) string

	// SetEnvironmentVariables sets the AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE environment variables
	SetEnvironmentVariables(providerName string) error

	// GetEnvironmentVariables returns the AWS file environment variables as EnvironmentVariable slice
	GetEnvironmentVariables(providerName, identityName string) []schema.EnvironmentVariable

	// Cleanup removes AWS files for the provider
	Cleanup(providerName string) error
}

// Validator defines the interface for validating auth configurations
type Validator interface {
	// ValidateAuthConfig validates the entire auth configuration
	ValidateAuthConfig(config *schema.AuthConfig) error

	// ValidateProvider validates a provider configuration
	ValidateProvider(name string, provider *schema.Provider) error

	// ValidateIdentity validates an identity configuration
	ValidateIdentity(name string, identity *schema.Identity, providers map[string]*schema.Provider) error

	// ValidateChains validates identity chains for cycles and invalid references
	ValidateChains(identities map[string]*schema.Identity, providers map[string]*schema.Provider) error
}
