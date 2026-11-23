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

// acrCredentialsForTenant swaps the identity's credentials for credentials issued by
// spec.registry.tenant_id. Overridable in tests.
var acrCredentialsForTenant = azureCloud.CredentialsForACRTenant

// acrDockerConfigFactory creates a Docker config manager. Overridable in tests.
var acrDockerConfigFactory = docker.NewConfigManager

// ACRIntegration implements the azure/acr integration type.
type ACRIntegration struct {
	name     string
	identity string
	registry *schema.Registry
	// realm is the credential isolation realm, whose MSAL cache holds the cached
	// sign-in used to mint a token in spec.registry.tenant_id.
	realm string
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

	if registry.TenantID != "" {
		if err := azureCloud.ValidateACRTenantID(registry.TenantID); err != nil {
			return nil, fmt.Errorf("%w: integration '%s': %w", errUtils.ErrIntegrationFailed, config.Name, err)
		}
	}

	return &ACRIntegration{
		name:     config.Name,
		identity: identity,
		registry: registry,
		realm:    config.Realm,
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

	registryURL := a.loginServer(creds)

	log.Debug("Logging in to ACR registry", "registry", registryURL)

	result, err := a.ExchangeCredentials(ctx, creds)
	if err != nil {
		return err
	}

	// Write credentials to Docker config.
	if err := dockerConfig.WriteAuth(registryURL, result.Username, result.Password); err != nil {
		notice, managed := docker.HelperManagedNotice(err)
		if !managed {
			return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDockerConfigWrite, err)
		}
		// config.json routes the registry to a helper that gets ACR credentials
		// itself and cannot store them, so there is nothing to write.
		ui.Info("ACR login: " + notice)
		return nil
	}

	// Log success with actual expiration time, when known.
	if result.Expiration == nil {
		ui.Success(fmt.Sprintf("ACR login: %s", registryURL))
	} else {
		expiresIn := time.Until(*result.Expiration).Round(time.Minute)
		ui.Success(fmt.Sprintf("ACR login: %s (expires in %s)", registryURL, expiresIn))
	}
	log.Debug("ACR login successful", "registry", registryURL, "expires_at", result.Expiration)

	return nil
}

// ExchangeCredentials mints credentials for this configured ACR registry without writing Docker state.
func (a *ACRIntegration) ExchangeCredentials(ctx context.Context, creds types.ICredentials) (*types.OCICredentials, error) {
	defer perf.Track(nil, "azure.ACRIntegration.ExchangeCredentials")()

	registryURL := a.loginServer(creds)

	// spec.registry.tenant_id: exchange a token issued by the registry's tenant.
	if a.registry.TenantID != "" {
		tenantCreds, err := acrCredentialsForTenant(ctx, creds, a.registry.TenantID, a.realm)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrACRAuthFailed, registryURL, err)
		}
		creds = tenantCreds
	}

	result, err := acrGetAuthToken(ctx, creds, registryURL)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get ACR token for %s: %w", errUtils.ErrACRAuthFailed, registryURL, err)
	}
	if result == nil {
		return nil, errUtils.ErrIdentityCredentialsNone
	}
	credentials := &types.OCICredentials{Registry: registryURL, Username: result.Username, Password: result.Password}
	if !result.ExpiresAt.IsZero() {
		credentials.Expiration = &result.ExpiresAt
	}
	return credentials, nil
}

// Cleanup removes ACR Docker config entries for this integration's registry.
// Logout runs without credentials, so the provider's cloud is not known here: the
// registry's login server is removed for every Azure cloud. RemoveAuth leaves
// registries without an auths entry alone, so only entries that exist are touched.
func (a *ACRIntegration) Cleanup(_ context.Context) error {
	defer perf.Track(nil, "azure.ACRIntegration.Cleanup")()

	registryURLs := azureCloud.RegistryURLsForAllClouds(a.registry.Name)

	dockerConfig, err := acrDockerConfigFactory()
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDockerConfigWrite, err)
	}

	if err := dockerConfig.RemoveAuth(registryURLs...); err != nil {
		return fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrDockerConfigWrite, err)
	}

	log.Debug("ACR cleanup: removed Docker auth", "registries", registryURLs)

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

// loginServer returns the registry's login server in the Azure cloud of the
// identity's provider (spec.cloud_environment, carried on the credentials as
// CloudEnvironment). Without Azure credentials or a known cloud it is the public cloud.
func (a *ACRIntegration) loginServer(creds types.ICredentials) string {
	var cloudEnv *azureCloud.CloudEnvironment
	if azureCreds, ok := creds.(*types.AzureCredentials); ok && azureCreds != nil {
		cloudEnv = azureCloud.GetCloudEnvironment(azureCreds.CloudEnvironment)
	}
	return azureCloud.BuildRegistryURLForCloud(a.registry.Name, cloudEnv)
}

// GetIdentity returns the identity name this integration uses.
func (a *ACRIntegration) GetIdentity() string {
	return a.identity
}

// GetRegistry returns the configured registry.
func (a *ACRIntegration) GetRegistry() *schema.Registry {
	return a.registry
}
