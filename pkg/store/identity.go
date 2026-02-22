package store

import "context"

// AWSAuthConfig holds the AWS-specific authentication configuration resolved from an identity.
// This mirrors the relevant fields from schema.AWSAuthContext without importing pkg/schema
// to avoid circular dependencies (pkg/schema imports pkg/store).
type AWSAuthConfig struct {
	CredentialsFile string
	ConfigFile      string
	Profile         string
	Region          string
}

// AzureAuthConfig holds the Azure-specific authentication configuration resolved from an identity.
type AzureAuthConfig struct {
	TenantID string
	UseOIDC  bool
	ClientID string
}

// GCPAuthConfig holds the GCP-specific authentication configuration resolved from an identity.
type GCPAuthConfig struct {
	CredentialsFile string
}

// AuthContextResolver resolves an identity name to a cloud-specific auth configuration.
// Implemented outside this package (in pkg/store/authbridge) to avoid circular deps.
type AuthContextResolver interface {
	// ResolveAWSAuthContext authenticates the named identity and returns AWS credentials.
	ResolveAWSAuthContext(ctx context.Context, identityName string) (*AWSAuthConfig, error)

	// ResolveAzureAuthContext authenticates the named identity and returns Azure credentials.
	ResolveAzureAuthContext(ctx context.Context, identityName string) (*AzureAuthConfig, error)

	// ResolveGCPAuthContext authenticates the named identity and returns GCP credentials.
	ResolveGCPAuthContext(ctx context.Context, identityName string) (*GCPAuthConfig, error)
}

// IdentityAwareStore is implemented by stores that support identity-based authentication.
// Stores that implement this interface can authenticate using Atmos auth identities
// instead of the default credential chain.
type IdentityAwareStore interface {
	Store
	// SetAuthContext injects the resolver and identity name so the store can
	// lazily resolve credentials on first Get/Set call.
	SetAuthContext(resolver AuthContextResolver, identityName string)
}
