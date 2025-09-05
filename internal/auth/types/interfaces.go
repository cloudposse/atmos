package types

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

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

	// Authenticate performs authentication using the provided base credentials
	Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error)

	// Validate validates the identity configuration
	Validate() error

	// Environment returns environment variables that should be set for this identity
	Environment() (map[string]string, error)

	// Merge merges this identity configuration with component-level overrides
	Merge(component *schema.Identity) Identity
}

// AuthManager manages the overall authentication process
type AuthManager interface {
	// Authenticate performs authentication for the specified identity
	Authenticate(ctx context.Context, identityName string) (*schema.WhoamiInfo, error)

	// Whoami returns information about the specified identity's credentials
	Whoami(ctx context.Context, identityName string) (*schema.WhoamiInfo, error)

	// Validate validates the entire auth configuration
	Validate() error

	// SetupAWSFiles writes AWS credentials and config files for the specified identity
	SetupAWSFiles(ctx context.Context, providerName, identityName string, creds *schema.Credentials) error

	// GetDefaultIdentity returns the name of the default identity, if any
	GetDefaultIdentity() (string, error)

	// ListIdentities returns all available identity names
	ListIdentities() []string

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

// ConfigMerger defines the interface for merging auth configurations
type ConfigMerger interface {
	// MergeAuthConfig merges component auth config with global auth config
	MergeAuthConfig(global *schema.AuthConfig, component *schema.ComponentAuthConfig) (*schema.AuthConfig, error)

	// MergeIdentity merges component identity config with global identity config
	MergeIdentity(global *schema.Identity, component *schema.Identity) *schema.Identity

	// MergeProvider merges component provider config with global provider config
	MergeProvider(global *schema.Provider, component *schema.Provider) *schema.Provider
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
