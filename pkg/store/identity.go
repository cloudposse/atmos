package store

import (
	"context"
	"time"
)

// AWSAuthConfig holds the AWS-specific authentication configuration resolved from an identity.
// This mirrors the relevant fields from schema.AWSAuthContext without importing pkg/schema
// to avoid circular dependencies (pkg/schema imports pkg/store).
type AWSAuthConfig struct {
	CredentialsFile string
	ConfigFile      string
	Profile         string
	Region          string
	EndpointURL     string
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
	AccessToken     string //nolint:gosec // Intentional credential field resolved from Atmos identity context.
	TokenExpiry     time.Time
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

// SecretsAuthContext carries an identity-resolving AuthContextResolver and the effective default
// identity name to non-store secret backends (e.g. cloud-KMS SOPS providers) that live outside the
// store registry but need the same identity->credentials resolution. It is populated by the same
// code paths that inject the store auth resolver (the `atmos secret` command and terraform), so
// SOPS providers can authenticate KMS calls via an Atmos identity instead of ambient credentials.
type SecretsAuthContext struct {
	// Resolver authenticates an identity name and returns cloud-specific credentials.
	Resolver AuthContextResolver
	// DefaultIdentity is the effective identity (from --identity/ATMOS_IDENTITY or the stack/component
	// default) used when a provider does not name its own identity.
	DefaultIdentity string
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
