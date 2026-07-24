package azure

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	"github.com/cloudposse/atmos/pkg/auth/integrations"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func init() {
	integrations.Register(integrations.KindAzureACR, NewACRIntegration)
}

// acrGetAuthToken retrieves an ACR authorization token. Overridable in tests.
var acrGetAuthToken = azureCloud.GetAuthorizationToken

// acrDockerConfigFactory creates a Docker config manager. Overridable in tests.
var acrDockerConfigFactory = docker.NewConfigManager

// ACRIntegration implements the azure/acr integration type.
type ACRIntegration struct {
	name     string
	identity string
	registry *schema.Registry
}

// NewACRIntegration creates an ACR integration from config.
func NewACRIntegration(config *integrations.IntegrationConfig) (integrations.Integration, error) {
	defer perf.Track(nil, "azure.NewACRIntegration")()

	if config == nil || config.Config == nil {
		return nil, fmt.Errorf("%w: integration config is nil", errUtils.ErrIntegrationNotFound)
	}

	// Extract identity from via.identity.
	identity := ""
	if config.Config.Via != nil {
		identity = config.Config.Via.Identity
	}

	// Extract registry from spec.registry - required for azure/acr integrations.
	var registry *schema.Registry
	if config.Config.Spec != nil && config.Config.Spec.Registry != nil {
		registry = config.Config.Spec.Registry
	}

	if registry == nil {
		return nil, fmt.Errorf("%w: integration '%s' has no registry configured (spec.registry is required for azure/acr)", errUtils.ErrIntegrationFailed, config.Name)
	}

	if registry.Name == "" {
		return nil, fmt.Errorf("%w: integration '%s' has no registry name configured", errUtils.ErrIntegrationFailed, config.Name)
	}

	return &ACRIntegration{
		name:     config.Name,
		identity: identity,
		registry: registry,
	}, nil
}

// Kind returns "azure/acr".
func (a *ACRIntegration) Kind() string {
	return integrations.KindAzureACR
}

// Execute performs ACR login for the configured registry.
func (a *ACRIntegration) Execute(ctx context.Context, creds types.ICredentials) error {
	defer perf.Track(nil, "azure.ACRIntegration.Execute")()

	// Create Docker config manager.
	dockerConfig, err := acrDockerConfigFactory()
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrIntegrationFailed, err)
	}

	registryURL := azureCloud.BuildRegistryURL(a.registry.Name)

	log.Debug("Logging in to ACR registry", "registry", registryURL)

	// Get authorization token from ACR.
	result, err := acrGetAuthToken(ctx, creds, registryURL)
	if err != nil {
		return fmt.Errorf("%w: failed to get ACR token for %s: %w", errUtils.ErrACRAuthFailed, registryURL, err)
	}

	// Write credentials to Docker config.
	if err := dockerConfig.WriteAuth(registryURL, result.Username, result.Password); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDockerConfigWrite, err)
	}

	// Log success with actual expiration time, when known.
	if result.ExpiresAt.IsZero() {
		ui.Success(fmt.Sprintf("ACR login: %s", registryURL))
	} else {
		expiresIn := time.Until(result.ExpiresAt).Round(time.Minute)
		ui.Success(fmt.Sprintf("ACR login: %s (expires in %s)", registryURL, expiresIn))
	}
	log.Debug("ACR login successful", "registry", registryURL, "expires_at", result.ExpiresAt)

	return nil
}

// Cleanup removes ACR Docker config entries for this integration's registry.
func (a *ACRIntegration) Cleanup(_ context.Context) error {
	defer perf.Track(nil, "azure.ACRIntegration.Cleanup")()

	registryURL := azureCloud.BuildRegistryURL(a.registry.Name)

	dockerConfig, err := acrDockerConfigFactory()
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDockerConfigWrite, err)
	}

	if err := dockerConfig.RemoveAuth(registryURL); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDockerConfigWrite, err)
	}

	log.Debug("ACR cleanup: removed Docker auth", "registry", registryURL)

	return nil
}

// Environment returns environment variables contributed by this ACR integration.
func (a *ACRIntegration) Environment() (map[string]string, error) {
	defer perf.Track(nil, "azure.ACRIntegration.Environment")()

	dockerConfig, err := acrDockerConfigFactory()
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDockerConfigWrite, err)
	}

	return map[string]string{
		"DOCKER_CONFIG": dockerConfig.GetConfigDir(),
	}, nil
}

// GetIdentity returns the identity name this integration uses.
func (a *ACRIntegration) GetIdentity() string {
	return a.identity
}

// GetRegistry returns the configured registry.
func (a *ACRIntegration) GetRegistry() *schema.Registry {
	return a.registry
}
