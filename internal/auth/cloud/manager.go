package cloud

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/schema"
)

// DefaultCloudProviderManager implements CloudProviderManager
type DefaultCloudProviderManager struct {
	factory CloudProviderFactory
}

// NewCloudProviderManager creates a new cloud provider manager
func NewCloudProviderManager() CloudProviderManager {
	return &DefaultCloudProviderManager{
		factory: NewCloudProviderFactory(),
	}
}

// NewCloudProviderManagerWithFactory creates a new cloud provider manager with a custom factory
func NewCloudProviderManagerWithFactory(factory CloudProviderFactory) CloudProviderManager {
	return &DefaultCloudProviderManager{
		factory: factory,
	}
}

// SetupEnvironment sets up the cloud environment for the given auth provider
func (m *DefaultCloudProviderManager) SetupEnvironment(ctx context.Context, providerKind, providerName, identityName string, credentials *schema.Credentials) error {
	cloudProvider, err := m.factory.GetCloudProvider(providerKind)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider for %s: %w", providerKind, err)
	}

	// Validate credentials before setup
	if err := cloudProvider.ValidateCredentials(ctx, credentials); err != nil {
		return fmt.Errorf("credential validation failed for %s: %w", cloudProvider.GetName(), err)
	}

	// Setup cloud-specific environment
	if err := cloudProvider.SetupEnvironment(ctx, providerName, identityName, credentials); err != nil {
		return fmt.Errorf("failed to setup environment for %s: %w", cloudProvider.GetName(), err)
	}

	return nil
}

// Cleanup cleans up cloud resources for the given auth provider
func (m *DefaultCloudProviderManager) Cleanup(ctx context.Context, providerKind, providerName, identityName string) error {
	cloudProvider, err := m.factory.GetCloudProvider(providerKind)
	if err != nil {
		return fmt.Errorf("failed to get cloud provider for %s: %w", providerKind, err)
	}

	if err := cloudProvider.Cleanup(ctx, providerName, identityName); err != nil {
		return fmt.Errorf("failed to cleanup environment for %s: %w", cloudProvider.GetName(), err)
	}

	return nil
}

// GetEnvironmentVariables gets environment variables for the given auth provider
func (m *DefaultCloudProviderManager) GetEnvironmentVariables(providerKind, providerName, identityName string) (map[string]string, error) {
	cloudProvider, err := m.factory.GetCloudProvider(providerKind)
	if err != nil {
		return nil, fmt.Errorf("failed to get cloud provider for %s: %w", providerKind, err)
	}

	envVars, err := cloudProvider.GetEnvironmentVariables(providerName, identityName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment variables: %w", err)
	}
	return envVars, nil
}
