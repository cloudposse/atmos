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
// Fields mirror schema.AzureAuthContext; realm-scoped paths are embedded in CredentialsFile.
type AzureAuthConfig struct {
	CredentialsFile string
	SubscriptionID  string
	TenantID        string
	UseOIDC         bool
	ClientID        string
	TokenFilePath   string
}

// GCPAuthConfig holds the GCP-specific authentication configuration resolved from an identity.
// Fields mirror schema.GCPAuthContext; realm-scoped paths are embedded in CredentialsFile.
type GCPAuthConfig struct {
	CredentialsFile string
	ProjectID       string
}

// AuthContextResolver resolves an identity name to a cloud-specific auth configuration.
// Implemented outside this package (in pkg/store/authbridge) to avoid circular deps.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_identity.go -package=store
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
