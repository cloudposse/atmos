package azure

import (
	"context"
	"fmt"
	"net/url"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// AzurePortalURL is the Azure Portal base URL.
	AzurePortalURL = "https://portal.azure.com/"

	// AzureDefaultSessionDuration is the default session duration (Azure tokens are typically valid for 1 hour).
	AzureDefaultSessionDuration = 1 * time.Hour
)

// destinationPattern holds the URL pattern for a destination alias.
type destinationPattern struct {
	path                 string // URL path after base URL
	requiresSubscription bool   // Whether this destination requires subscription_id
}

// azurePortalDestinations maps destination aliases to their URL patterns.
var azurePortalDestinations = map[string]destinationPattern{
	"resourcegroups":     {path: "/blade/HubsExtension/BrowseResourceGroups"},
	"rg":                 {path: "/blade/HubsExtension/BrowseResourceGroups"},
	"vm":                 {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Compute%2FVirtualMachines"},
	"virtualmachines":    {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Compute%2FVirtualMachines"},
	"storage":            {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Storage%2FStorageAccounts"},
	"storageaccounts":    {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Storage%2FStorageAccounts"},
	"network":            {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Network%2FVirtualNetworks"},
	"vnet":               {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Network%2FVirtualNetworks"},
	"virtualnetworks":    {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Network%2FVirtualNetworks"},
	"cosmosdb":           {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.DocumentDb%2FDatabaseAccounts"},
	"cosmos":             {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.DocumentDb%2FDatabaseAccounts"},
	"sql":                {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Sql%2FServers"},
	"sqldatabases":       {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Sql%2FServers"},
	"keyvault":           {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.KeyVault%2FVaults"},
	"kv":                 {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.KeyVault%2FVaults"},
	"monitor":            {path: "/blade/Microsoft_Azure_Monitoring/AzureMonitoringBrowseBlade"},
	"monitoring":         {path: "/blade/Microsoft_Azure_Monitoring/AzureMonitoringBrowseBlade"},
	"aks":                {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.ContainerService%2FManagedClusters"},
	"kubernetes":         {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.ContainerService%2FManagedClusters"},
	"functions":          {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Web%2FSites/kind/functionapp"},
	"functionapps":       {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Web%2FSites/kind/functionapp"},
	"appservice":         {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Web%2FSites"},
	"webapps":            {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Web%2FSites"},
	"containers":         {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.ContainerInstance%2FContainerGroups"},
	"containerinstances": {path: "/blade/HubsExtension/BrowseResource/resourceType/Microsoft.ContainerInstance%2FContainerGroups"},
	"subscription":       {path: "/resource/subscriptions/%s/overview", requiresSubscription: true},
	"sub":                {path: "/resource/subscriptions/%s/overview", requiresSubscription: true},
}

// ConsoleURLGenerator generates Azure Portal URLs with authentication context.
type ConsoleURLGenerator struct{}

// NewConsoleURLGenerator creates a new ConsoleURLGenerator.
func NewConsoleURLGenerator() *ConsoleURLGenerator {
	defer perf.Track(nil, "azure.NewConsoleURLGenerator")()
	return &ConsoleURLGenerator{}
}

// SupportsConsoleAccess returns true (Azure Console URL generator supports console access).
func (g *ConsoleURLGenerator) SupportsConsoleAccess() bool {
	return true
}

// GetConsoleURL generates an Azure Portal sign-in URL with authentication context.
//
// Azure Portal URLs support deep linking with tenant context:
//   - Base portal: https://portal.azure.com/
//   - Tenant-specific: https://portal.azure.com/#@{tenant}
//   - Resource-specific: https://portal.azure.com/#@{tenant}/resource/subscriptions/{sub}/...
//
// Unlike AWS federation (which requires a signin token), Azure Portal authentication
// uses browser-based OAuth with the same credentials used to access Azure APIs.
// The Portal will automatically pick up the user's authenticated session.
//
// References:
//   - https://docs.microsoft.com/en-us/azure/azure-portal/azure-portal-dashboards-create-programmatically
//   - https://learn.microsoft.com/en-us/azure/azure-portal/azure-portal-add-to-favorites
func (g *ConsoleURLGenerator) GetConsoleURL(ctx context.Context, creds types.ICredentials, options types.ConsoleURLOptions) (string, time.Duration, error) {
	defer perf.Track(nil, "azure.ConsoleURLGenerator.GetConsoleURL")()

	// Validate and extract Azure credentials.
	azureCreds, err := validateAzureCredentials(creds)
	if err != nil {
		return "", 0, err
	}

	// Determine session duration (informational - Azure controls actual session lifetime).
	duration := determineSessionDuration(options.SessionDuration)

	// Resolve destination.
	destination, err := resolveDestinationWithDefault(options.Destination, azureCreds)
	if err != nil {
		return "", 0, err
	}

	log.Debug("Generated Azure Portal URL", "destination", destination, "duration", duration)

	return destination, duration, nil
}

// validateAzureCredentials validates and extracts Azure credentials from the interface.
func validateAzureCredentials(creds types.ICredentials) (*types.AzureCredentials, error) {
	// Check if credentials are nil.
	if creds == nil {
		return nil, fmt.Errorf("%w: credentials cannot be nil", errUtils.ErrInvalidAuthConfig)
	}

	azureCreds, ok := creds.(*types.AzureCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: expected Azure credentials, got %T", errUtils.ErrInvalidAuthConfig, creds)
	}

	if azureCreds.AccessToken == "" {
		return nil, fmt.Errorf("%w: access token required for console access", errUtils.ErrInvalidAuthConfig)
	}

	if azureCreds.TenantID == "" {
		return nil, fmt.Errorf("%w: tenant ID required for console access", errUtils.ErrInvalidAuthConfig)
	}

	return azureCreds, nil
}

// determineSessionDuration determines the session duration (informational for Azure).
// Azure Portal session lifetime is controlled by Azure AD token expiration, not by URL parameters.
func determineSessionDuration(requested time.Duration) time.Duration {
	duration := requested
	if duration == 0 {
		duration = AzureDefaultSessionDuration
	}
	return duration
}

// resolveDestinationWithDefault resolves the destination and applies default if empty.
func resolveDestinationWithDefault(dest string, azureCreds *types.AzureCredentials) (string, error) {
	destination, err := ResolveDestination(dest, azureCreds)
	if err != nil {
		return "", fmt.Errorf("failed to resolve destination: %w", err)
	}
	if destination == "" {
		// Default to tenant-specific portal home.
		destination = fmt.Sprintf("%s#@%s", AzurePortalURL, azureCreds.TenantID)
	}
	return destination, nil
}

// ResolveDestination resolves destination aliases to full Azure Portal URLs.
//
// Supports the following destination formats:
//   - Empty string or "home" → Tenant home page
//   - "subscription" → Subscription overview
//   - "resourcegroups" or "rg" → Resource groups blade
//   - "vm" or "virtualmachines" → Virtual Machines blade
//   - "storage" or "storageaccounts" → Storage Accounts blade
//   - "network" or "vnet" → Virtual Networks blade
//   - "cosmosdb" → Cosmos DB blade
//   - "sql" → SQL Databases blade
//   - "keyvault" → Key Vaults blade
//   - "monitor" → Azure Monitor blade
//   - Full URL starting with https:// → Pass through unchanged
//
// All resolved URLs include tenant context for proper navigation.
func ResolveDestination(dest string, azureCreds *types.AzureCredentials) (string, error) {
	// Validate credentials.
	if err := validateDestinationCredentials(azureCreds); err != nil {
		return "", err
	}

	if dest == "" || dest == "home" {
		// Tenant home page.
		return fmt.Sprintf("%s#@%s", AzurePortalURL, azureCreds.TenantID), nil
	}

	// If already a full URL, pass through unchanged.
	if parsedURL, err := url.Parse(dest); err == nil && parsedURL.Scheme != "" {
		return dest, nil
	}

	// Build base URL with tenant context.
	baseURL := fmt.Sprintf("%s#@%s", AzurePortalURL, azureCreds.TenantID)

	// Look up destination pattern.
	pattern, found := azurePortalDestinations[dest]
	if !found {
		return "", fmt.Errorf("%w: unsupported destination alias: %s", errUtils.ErrInvalidAuthConfig, dest)
	}

	// Check if subscription is required.
	if pattern.requiresSubscription && azureCreds.SubscriptionID == "" {
		return "", fmt.Errorf("%w: subscription_id required for '%s' destination", errUtils.ErrInvalidAuthConfig, dest)
	}

	// Build final URL.
	if pattern.requiresSubscription {
		return baseURL + fmt.Sprintf(pattern.path, azureCreds.SubscriptionID), nil
	}
	return baseURL + pattern.path, nil
}

// validateDestinationCredentials validates that credentials have required fields for destination resolution.
func validateDestinationCredentials(azureCreds *types.AzureCredentials) error {
	if azureCreds == nil {
		return fmt.Errorf("%w: Azure credentials are required to resolve console destination", errUtils.ErrInvalidAuthConfig)
	}

	if azureCreds.TenantID == "" {
		return fmt.Errorf("%w: tenant ID required to resolve console destination", errUtils.ErrInvalidAuthConfig)
	}

	return nil
}
