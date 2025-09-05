package cloud

import (
	"context"

	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// CloudProvider is an alias for the types.CloudProvider interface
type CloudProvider = types.CloudProvider

// CloudProviderFactory creates cloud provider instances based on provider kind
type CloudProviderFactory interface {
	// GetCloudProvider returns the appropriate cloud provider for the given provider kind
	// For example: "aws/sso" -> AWS cloud provider, "azure/ad" -> Azure cloud provider
	GetCloudProvider(providerKind string) (CloudProvider, error)

	// RegisterCloudProvider allows registration of new cloud providers
	RegisterCloudProvider(name string, provider CloudProvider) error

	// ListCloudProviders returns all registered cloud provider names
	ListCloudProviders() []string
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
