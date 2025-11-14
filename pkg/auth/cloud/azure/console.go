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
	// Guard against nil credentials.
	if azureCreds == nil {
		return "", fmt.Errorf("%w: Azure credentials are required to resolve console destination", errUtils.ErrInvalidAuthConfig)
	}

	// Guard against missing tenant ID.
	if azureCreds.TenantID == "" {
		return "", fmt.Errorf("%w: tenant ID required to resolve console destination", errUtils.ErrInvalidAuthConfig)
	}

	if dest == "" || dest == "home" {
		// Tenant home page.
		return fmt.Sprintf("%s#@%s", AzurePortalURL, azureCreds.TenantID), nil
	}

	// If already a full URL, pass through unchanged.
	if parsedURL, err := url.Parse(dest); err == nil && parsedURL.Scheme != "" {
		return dest, nil
	}

	// Resolve destination aliases.
	tenantID := azureCreds.TenantID
	subscriptionID := azureCreds.SubscriptionID

	// Build base URL with tenant context.
	baseURL := fmt.Sprintf("%s#@%s", AzurePortalURL, tenantID)

	switch dest {
	case "subscription", "sub":
		if subscriptionID == "" {
			return "", fmt.Errorf("%w: subscription_id required for 'subscription' destination", errUtils.ErrInvalidAuthConfig)
		}
		return fmt.Sprintf("%s/resource/subscriptions/%s/overview", baseURL, subscriptionID), nil

	case "resourcegroups", "rg":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResourceGroups", baseURL), nil

	case "vm", "virtualmachines":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Compute%%2FVirtualMachines", baseURL), nil

	case "storage", "storageaccounts":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Storage%%2FStorageAccounts", baseURL), nil

	case "network", "vnet", "virtualnetworks":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Network%%2FVirtualNetworks", baseURL), nil

	case "cosmosdb", "cosmos":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.DocumentDb%%2FDatabaseAccounts", baseURL), nil

	case "sql", "sqldatabases":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Sql%%2FServers", baseURL), nil

	case "keyvault", "kv":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.KeyVault%%2FVaults", baseURL), nil

	case "monitor", "monitoring":
		return fmt.Sprintf("%s/blade/Microsoft_Azure_Monitoring/AzureMonitoringBrowseBlade", baseURL), nil

	case "aks", "kubernetes":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.ContainerService%%2FManagedClusters", baseURL), nil

	case "functions", "functionapps":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Web%%2FSites/kind/functionapp", baseURL), nil

	case "appservice", "webapps":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.Web%%2FSites", baseURL), nil

	case "containers", "containerinstances":
		return fmt.Sprintf("%s/blade/HubsExtension/BrowseResource/resourceType/Microsoft.ContainerInstance%%2FContainerGroups", baseURL), nil

	default:
		return "", fmt.Errorf("%w: unsupported destination alias: %s", errUtils.ErrInvalidAuthConfig, dest)
	}
}
