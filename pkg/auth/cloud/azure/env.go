package azure

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// problematicAzureEnvVars lists environment variables that should be cleared by Atmos auth
// to avoid authentication conflicts when using Azure SDK.
//
// These variables can interfere with Atmos's Azure authentication flow.
// By clearing these variables before authentication, we ensure Atmos uses only its managed
// credentials.
var problematicAzureEnvVars = []string{
	// Authentication credentials.
	"AZURE_CLIENT_ID",
	"AZURE_CLIENT_SECRET",
	"AZURE_CLIENT_CERTIFICATE_PATH",
	"AZURE_USERNAME",
	"AZURE_PASSWORD",
	"AZURE_FEDERATED_TOKEN_FILE",

	// Subscription/tenant configuration.
	"AZURE_SUBSCRIPTION_ID",
	"AZURE_TENANT_ID",

	// Location configuration.
	"AZURE_LOCATION",
	"AZURE_REGION",

	// Config file paths.
	"AZURE_CONFIG_DIR",

	// Terraform/ARM specific vars.
	"ARM_CLIENT_ID",
	"ARM_CLIENT_SECRET",
	"ARM_CLIENT_CERTIFICATE_PATH",
	"ARM_SUBSCRIPTION_ID",
	"ARM_TENANT_ID",
	"ARM_USE_MSI",
	"ARM_USE_CLI",
	"ARM_USE_OIDC",
}

// PrepareEnvironment configures environment variables for Azure SDK when using Atmos auth.
//
// This function:
//  1. Clears direct Azure credential env vars to prevent conflicts with Atmos-managed credentials
//  2. Sets AZURE_SUBSCRIPTION_ID, AZURE_TENANT_ID, AZURE_LOCATION
//  3. Sets ARM_* variables for Terraform provider compatibility
//  4. Sets ARM_USE_CLI=true to enable Azure CLI authentication
//
// This matches how 'az login' works - Atmos updates the MSAL cache and Azure profile,
// then Terraform providers automatically detect and use those credentials via ARM_USE_CLI.
//
// Note: Other cloud provider credentials (AWS, GCP) are NOT cleared to support multi-cloud
// scenarios such as using S3 backend for Terraform state while deploying to Azure.
//
// Returns a NEW map with modifications - does not mutate the input.
//
// Parameters:
//   - environ: Current environment variables (map[string]string)
//   - subscriptionID: Azure subscription ID
//   - tenantID: Azure tenant ID
//   - location: Azure location/region
//   - credentialsFile: Path to Azure credentials file (unused, kept for compatibility)
//   - accessToken: Azure access token (unused, kept for compatibility)
func PrepareEnvironment(environ map[string]string, subscriptionID, tenantID, location, credentialsFile, accessToken string) map[string]string {
	defer perf.Track(nil, "pkg/auth/cloud/azure.PrepareEnvironment")()

	log.Debug("Preparing Azure environment for Atmos-managed credentials",
		"subscription", subscriptionID,
		"tenant", tenantID,
		"location", location,
		"credentials_file", credentialsFile,
		"has_access_token", accessToken != "",
	)

	// Create a copy to avoid mutating the input.
	result := make(map[string]string, len(environ)+10)
	for k, v := range environ {
		result[k] = v
	}

	// Clear problematic Azure credential environment variables.
	// These would override Atmos-managed credentials.
	// Note: We do NOT clear AWS/GCP credentials to support multi-cloud scenarios.
	for _, key := range problematicAzureEnvVars {
		if _, exists := result[key]; exists {
			log.Debug("Clearing Azure credential environment variable", "key", key)
			delete(result, key)
		}
	}

	// Set Azure subscription and tenant for Terraform providers.
	// These are required for azurerm, azuread, and azapi providers to work correctly.
	if subscriptionID != "" {
		result["AZURE_SUBSCRIPTION_ID"] = subscriptionID
		result["ARM_SUBSCRIPTION_ID"] = subscriptionID
	}

	if tenantID != "" {
		result["AZURE_TENANT_ID"] = tenantID
		result["ARM_TENANT_ID"] = tenantID
	}

	if location != "" {
		result["AZURE_LOCATION"] = location
		result["ARM_LOCATION"] = location
	}

	// Always use Azure CLI authentication for Terraform providers.
	// This matches how 'az login' works - it updates the MSAL cache and Azure profile,
	// then the providers automatically detect and use those credentials.
	// This approach works for all three providers: azurerm, azapi, and azuread.
	result["ARM_USE_CLI"] = "true"
	log.Debug("Set ARM_USE_CLI=true for Azure CLI authentication",
		"note", "Providers will use MSAL cache populated by Atmos")

	log.Debug("Azure auth active - Terraform will use Azure CLI credentials from MSAL cache",
		"subscription", subscriptionID,
		"tenant", tenantID,
	)

	return result
}
