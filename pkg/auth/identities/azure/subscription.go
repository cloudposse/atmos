package azure

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// subscriptionIdentity implements Azure subscription-based identity.
// This identity uses Azure credentials to access a specific subscription and resource scope.
type subscriptionIdentity struct {
	name           string
	config         *schema.Identity
	subscriptionID string
	resourceGroup  string
	location       string
}

// NewSubscriptionIdentity creates a new Azure subscription identity.
func NewSubscriptionIdentity(name string, config *schema.Identity) (*subscriptionIdentity, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is required", errUtils.ErrInvalidIdentityConfig)
	}
	if config.Kind != "azure/subscription" {
		return nil, fmt.Errorf("%w: invalid identity kind for Azure subscription identity: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	// Extract subscription-specific config from Principal.
	subscriptionID := ""
	resourceGroup := ""
	location := ""

	if config.Principal != nil {
		if sid, ok := config.Principal["subscription_id"].(string); ok {
			subscriptionID = sid
		}
		if rg, ok := config.Principal["resource_group"].(string); ok {
			resourceGroup = rg
		}
		if loc, ok := config.Principal["location"].(string); ok {
			location = loc
		}
	}

	// Subscription ID is required.
	if subscriptionID == "" {
		return nil, fmt.Errorf("%w: subscription_id is required in principal for Azure subscription identity", errUtils.ErrInvalidIdentityConfig)
	}

	return &subscriptionIdentity{
		name:           name,
		config:         config,
		subscriptionID: subscriptionID,
		resourceGroup:  resourceGroup,
		location:       location,
	}, nil
}

// Kind returns the identity kind.
func (i *subscriptionIdentity) Kind() string {
	return "azure/subscription"
}

// GetProviderName returns the provider name for this identity.
func (i *subscriptionIdentity) GetProviderName() (string, error) {
	if i.config.Via == nil || i.config.Via.Provider == "" {
		return "", fmt.Errorf("%w: Azure subscription identity requires via.provider", errUtils.ErrInvalidIdentityConfig)
	}
	return i.config.Via.Provider, nil
}

// Authenticate performs authentication using the provided base credentials.
// For Azure subscription identity, we use the provider credentials directly.
func (i *subscriptionIdentity) Authenticate(ctx context.Context, baseCreds authTypes.ICredentials) (authTypes.ICredentials, error) {
	defer perf.Track(nil, "azure.subscriptionIdentity.Authenticate")()

	log.Debug("Authenticating Azure subscription identity",
		azureCloud.LogFieldIdentity, i.name,
		azureCloud.LogFieldSubscription, i.subscriptionID,
	)

	// Verify base credentials are Azure credentials.
	azureCreds, ok := baseCreds.(*authTypes.AzureCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: Azure subscription identity requires Azure credentials from provider", errUtils.ErrAuthenticationFailed)
	}

	// Create new credentials with subscription-specific configuration.
	// Override subscription ID if different from provider.
	creds := &authTypes.AzureCredentials{
		AccessToken:        azureCreds.AccessToken,
		TokenType:          azureCreds.TokenType,
		Expiration:         azureCreds.Expiration,
		TenantID:           azureCreds.TenantID,
		SubscriptionID:     i.subscriptionID,              // Use identity's subscription.
		Location:           i.location,                    // Use identity's location if specified.
		GraphAPIToken:      azureCreds.GraphAPIToken,      // Preserve Graph API token from provider.
		GraphAPIExpiration: azureCreds.GraphAPIExpiration, // Preserve Graph API token expiration.
		KeyVaultToken:      azureCreds.KeyVaultToken,      // Preserve KeyVault API token from provider.
		KeyVaultExpiration: azureCreds.KeyVaultExpiration, // Preserve KeyVault token expiration.
	}

	// If location not specified in identity, use provider's location.
	if creds.Location == "" {
		creds.Location = azureCreds.Location
	}

	log.Debug("Successfully authenticated Azure subscription identity",
		azureCloud.LogFieldIdentity, i.name,
		azureCloud.LogFieldSubscription, i.subscriptionID,
	)

	return creds, nil
}

// Validate validates the identity configuration.
func (i *subscriptionIdentity) Validate() error {
	if i.subscriptionID == "" {
		return fmt.Errorf("%w: subscription_id is required", errUtils.ErrInvalidIdentityConfig)
	}
	if i.config.Via == nil || i.config.Via.Provider == "" {
		return fmt.Errorf("%w: via.provider is required", errUtils.ErrInvalidIdentityConfig)
	}
	return nil
}

// Environment returns environment variables for this identity.
func (i *subscriptionIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)
	if i.subscriptionID != "" {
		env["AZURE_SUBSCRIPTION_ID"] = i.subscriptionID
		env["ARM_SUBSCRIPTION_ID"] = i.subscriptionID
	}
	if i.location != "" {
		env["AZURE_LOCATION"] = i.location
		env["ARM_LOCATION"] = i.location
	}
	if i.resourceGroup != "" {
		env["AZURE_RESOURCE_GROUP"] = i.resourceGroup
	}
	return env, nil
}

// PrepareEnvironment prepares environment variables for external processes.
func (i *subscriptionIdentity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	// Add identity-specific environment variables.
	result := make(map[string]string)
	for k, v := range environ {
		result[k] = v
	}

	if i.subscriptionID != "" {
		result["AZURE_SUBSCRIPTION_ID"] = i.subscriptionID
		result["ARM_SUBSCRIPTION_ID"] = i.subscriptionID
	}
	if i.location != "" {
		result["AZURE_LOCATION"] = i.location
		result["ARM_LOCATION"] = i.location
	}
	if i.resourceGroup != "" {
		result["AZURE_RESOURCE_GROUP"] = i.resourceGroup
	}

	return result, nil
}

// PostAuthenticate is called after successful authentication with the final credentials.
func (i *subscriptionIdentity) PostAuthenticate(ctx context.Context, params *authTypes.PostAuthenticateParams) error {
	defer perf.Track(nil, "azure.subscriptionIdentity.PostAuthenticate")()

	log.Debug("Post-authenticate for Azure subscription identity",
		azureCloud.LogFieldIdentity, i.name,
		azureCloud.LogFieldSubscription, i.subscriptionID,
	)

	// Setup Azure files (credentials.json).
	if err := azureCloud.SetupFiles(params.ProviderName, params.IdentityName, params.Credentials, ""); err != nil {
		return fmt.Errorf("failed to setup Azure files: %w", err)
	}

	// Update Azure CLI files (MSAL cache and azureProfile.json) for Terraform provider compatibility.
	// This ensures azuread and azapi providers can authenticate using Azure CLI credentials.
	azureCreds, ok := params.Credentials.(*authTypes.AzureCredentials)
	if ok {
		if err := azureCloud.UpdateAzureCLIFiles(params.Credentials, azureCreds.TenantID, i.subscriptionID); err != nil {
			log.Debug("Failed to update Azure CLI files", "error", err)
			// Non-fatal - continue with normal flow.
		}
	}

	// Populate Azure auth context.
	setupParams := &azureCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: params.ProviderName,
		IdentityName: params.IdentityName,
		Credentials:  params.Credentials,
		BasePath:     "", // Use default path.
	}
	if err := azureCloud.SetAuthContext(setupParams); err != nil {
		return fmt.Errorf("failed to set Azure auth context: %w", err)
	}

	// Set environment variables in stack info.
	if err := azureCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo); err != nil {
		return fmt.Errorf("failed to set Azure environment variables: %w", err)
	}

	log.Debug("Post-authenticate complete for Azure subscription identity",
		azureCloud.LogFieldIdentity, i.name,
		azureCloud.LogFieldSubscription, i.subscriptionID,
	)

	return nil
}

// Logout removes identity-specific credential storage.
func (i *subscriptionIdentity) Logout(ctx context.Context) error {
	log.Debug("Logout Azure subscription identity", azureCloud.LogFieldIdentity, i.name)
	// Credentials are managed by keyring, files cleaned up by provider.
	return nil
}

// CredentialsExist checks if credentials exist for this identity.
func (i *subscriptionIdentity) CredentialsExist() (bool, error) {
	// Check if Azure credentials file exists.
	providerName, err := i.GetProviderName()
	if err != nil {
		return false, err
	}

	fileManager, err := azureCloud.NewAzureFileManager("")
	if err != nil {
		return false, err
	}

	return fileManager.CredentialsExist(providerName), nil
}

// Paths returns credential files/directories used by this identity.
func (i *subscriptionIdentity) Paths() ([]authTypes.Path, error) {
	// Get provider name.
	providerName, err := i.GetProviderName()
	if err != nil {
		return nil, err
	}

	// Create file manager to get provider-namespaced paths.
	fileManager, err := azureCloud.NewAzureFileManager("")
	if err != nil {
		return nil, err
	}

	return []authTypes.Path{
		{
			Location: fileManager.GetCredentialsPath(providerName),
			Type:     authTypes.PathTypeFile,
			Required: true,
			Purpose:  fmt.Sprintf("Azure credentials file for identity %s", i.name),
			Metadata: map[string]string{
				"read_only": "true",
			},
		},
	}, nil
}

// LoadCredentials loads credentials from identity-managed storage.
func (i *subscriptionIdentity) LoadCredentials(ctx context.Context) (authTypes.ICredentials, error) {
	// Load from Azure credentials file.
	providerName, err := i.GetProviderName()
	if err != nil {
		return nil, err
	}

	fileManager, err := azureCloud.NewAzureFileManager("")
	if err != nil {
		return nil, err
	}

	creds, err := fileManager.LoadCredentials(providerName)
	if err != nil {
		return nil, err
	}

	// Override subscription ID with identity's configuration.
	if i.subscriptionID != "" {
		creds.SubscriptionID = i.subscriptionID
	}
	if i.location != "" {
		creds.Location = i.location
	}

	return creds, nil
}
