package azure

import (
	"fmt"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// CloudEnvironment defines the endpoints for a specific Azure cloud (public, government, China).
type CloudEnvironment struct {
	// Name is the canonical name of the cloud environment.
	Name string
	// LoginEndpoint is the Azure AD / Entra ID authority host (e.g., "login.microsoftonline.com").
	LoginEndpoint string
	// ManagementScope is the Azure Resource Manager API scope.
	ManagementScope string
	// GraphAPIScope is the Microsoft Graph API scope.
	GraphAPIScope string
	// KeyVaultScope is the Azure KeyVault API scope.
	KeyVaultScope string
	// BlobStorageSuffix is the blob storage URL suffix (e.g., "blob.core.windows.net").
	BlobStorageSuffix string
	// PortalURL is the Azure Portal base URL.
	PortalURL string
	// AzureProfileEnvName is the environment name used in azureProfile.json (e.g., "AzureCloud").
	AzureProfileEnvName string
}

// Well-known Azure cloud environments.
var cloudEnvironments = map[string]*CloudEnvironment{
	"public": {
		Name:                "public",
		LoginEndpoint:       "login.microsoftonline.com",
		ManagementScope:     "https://management.azure.com/.default",
		GraphAPIScope:       "https://graph.microsoft.com/.default",
		KeyVaultScope:       "https://vault.azure.net/.default",
		BlobStorageSuffix:   "blob.core.windows.net",
		PortalURL:           "https://portal.azure.com/",
		AzureProfileEnvName: "AzureCloud",
	},
	"usgovernment": {
		Name:                "usgovernment",
		LoginEndpoint:       "login.microsoftonline.us",
		ManagementScope:     "https://management.usgovcloudapi.net/.default",
		GraphAPIScope:       "https://graph.microsoft.us/.default",
		KeyVaultScope:       "https://vault.usgovcloudapi.net/.default",
		BlobStorageSuffix:   "blob.core.usgovcloudapi.net",
		PortalURL:           "https://portal.azure.us/",
		AzureProfileEnvName: "AzureUSGovernment",
	},
	"china": {
		Name:                "china",
		LoginEndpoint:       "login.chinacloudapi.cn",
		ManagementScope:     "https://management.chinacloudapi.cn/.default",
		GraphAPIScope:       "https://microsoftgraph.chinacloudapi.cn/.default",
		KeyVaultScope:       "https://vault.azure.cn/.default",
		BlobStorageSuffix:   "blob.core.chinacloudapi.cn",
		PortalURL:           "https://portal.azure.cn/",
		AzureProfileEnvName: "AzureChinaCloud",
	},
}

// PublicCloud is the default Azure public cloud environment.
var PublicCloud = cloudEnvironments["public"]

// GetCloudEnvironment returns the endpoint set for the given cloud name.
// Returns the "public" environment if name is empty or unknown.
func GetCloudEnvironment(name string) *CloudEnvironment {
	if env, ok := cloudEnvironments[name]; ok {
		return env
	}
	return PublicCloud
}

// KnownCloudEnvironments returns the names of all known cloud environments.
func KnownCloudEnvironments() []string {
	names := make([]string, 0, len(cloudEnvironments))
	for name := range cloudEnvironments {
		names = append(names, name)
	}
	return names
}

// ValidateCloudEnvironment validates that a cloud environment name is known.
// Empty string is valid (defaults to "public"). Unknown non-empty values return an error.
func ValidateCloudEnvironment(name string) error {
	if name == "" {
		return nil // Empty defaults to public.
	}
	if _, ok := cloudEnvironments[name]; ok {
		return nil
	}
	known := KnownCloudEnvironments()
	sort.Strings(known)
	return fmt.Errorf("%w: unknown cloud_environment %q; valid values are: %s", errUtils.ErrInvalidAuthConfig, name, strings.Join(known, ", "))
}
