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

// PrepareEnvironmentConfig holds configuration for Azure environment preparation.
type PrepareEnvironmentConfig struct {
	Environ        map[string]string // Current environment variables
	SubscriptionID string            // Azure subscription ID
	TenantID       string            // Azure tenant ID
	Location       string            // Azure location/region (optional)
	// OIDC-specific configuration for Terraform ARM_USE_OIDC support.
	UseOIDC       bool   // Use OIDC instead of CLI authentication
	ClientID      string // Azure AD application (client) ID
	TokenFilePath string // Path to OIDC token file (optional)
}

// PrepareEnvironment configures environment variables for Azure SDK when using Atmos auth.
//
// This function:
//  1. Clears direct Azure credential env vars to prevent conflicts with Atmos-managed credentials
//  2. Sets AZURE_SUBSCRIPTION_ID, AZURE_TENANT_ID, AZURE_LOCATION
//  3. Sets ARM_* variables for Terraform provider compatibility
//  4. Sets ARM_USE_CLI=true (for CLI/device-code auth) or ARM_USE_OIDC=true (for OIDC auth)
//
// For OIDC authentication (service principal with federated credentials), it sets:
//   - ARM_USE_OIDC=true
//   - ARM_CLIENT_ID
//   - AZURE_FEDERATED_TOKEN_FILE (if token file path is provided)
//
// For CLI/device-code authentication, it sets ARM_USE_CLI=true which tells Terraform
// to use the MSAL cache populated by Atmos.
//
// Note: Other cloud provider credentials (AWS, GCP) are NOT cleared to support multi-cloud
// scenarios such as using S3 backend for Terraform state while deploying to Azure.
//
// Returns a NEW map with modifications - does not mutate the input.
func PrepareEnvironment(cfg PrepareEnvironmentConfig) map[string]string {
	defer perf.Track(nil, "pkg/auth/cloud/azure.PrepareEnvironment")()

	log.Debug("Preparing Azure environment for Atmos-managed credentials",
		"subscription", cfg.SubscriptionID,
		"tenant", cfg.TenantID,
		"location", cfg.Location,
		"useOIDC", cfg.UseOIDC,
	)

	// Create a copy to avoid mutating the input.
	result := make(map[string]string, len(cfg.Environ)+10)
	for k, v := range cfg.Environ {
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
	if cfg.SubscriptionID != "" {
		result["AZURE_SUBSCRIPTION_ID"] = cfg.SubscriptionID
		result["ARM_SUBSCRIPTION_ID"] = cfg.SubscriptionID
	}

	if cfg.TenantID != "" {
		result["AZURE_TENANT_ID"] = cfg.TenantID
		result["ARM_TENANT_ID"] = cfg.TenantID
	}

	if cfg.Location != "" {
		result["AZURE_LOCATION"] = cfg.Location
		result["ARM_LOCATION"] = cfg.Location
	}

	// Set authentication method based on provider type.
	if cfg.UseOIDC {
		// OIDC authentication (service principal with federated credentials).
		// This is used for GitHub Actions, Azure DevOps, and other CI/CD systems.
		result["ARM_USE_OIDC"] = "true"
		log.Debug("Set ARM_USE_OIDC=true for OIDC authentication")

		// Set client ID for OIDC.
		if cfg.ClientID != "" {
			result["ARM_CLIENT_ID"] = cfg.ClientID
			result["AZURE_CLIENT_ID"] = cfg.ClientID
		}

		// Set token file path if available.
		if cfg.TokenFilePath != "" {
			result["AZURE_FEDERATED_TOKEN_FILE"] = cfg.TokenFilePath
			result["ARM_OIDC_TOKEN_FILE_PATH"] = cfg.TokenFilePath
		}

		log.Debug("Azure OIDC auth active - Terraform will use federated credentials",
			"subscription", cfg.SubscriptionID,
			"tenant", cfg.TenantID,
			"client_id", cfg.ClientID,
		)
	} else {
		// CLI/device-code authentication.
		// This matches how 'az login' works - it updates the MSAL cache and Azure profile,
		// then the providers automatically detect and use those credentials.
		result["ARM_USE_CLI"] = "true"
		log.Debug("Set ARM_USE_CLI=true for Azure CLI authentication",
			"note", "Providers will use MSAL cache populated by Atmos")

		log.Debug("Azure CLI auth active - Terraform will use MSAL cache credentials",
			"subscription", cfg.SubscriptionID,
			"tenant", cfg.TenantID,
		)
	}

	return result
}
