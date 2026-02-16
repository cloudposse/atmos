package okta

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// problematicOktaEnvVars lists environment variables that should be cleared by Atmos auth
// to avoid authentication conflicts when using Okta SDK or Terraform Okta provider.
//
// These variables can interfere with Atmos's Okta authentication flow.
// By clearing these variables before authentication, we ensure Atmos uses only its managed
// credentials.
var problematicOktaEnvVars = []string{
	// Okta API authentication.
	"OKTA_API_TOKEN",
	"OKTA_API_KEY",

	// OAuth client credentials.
	"OKTA_CLIENT_ID",
	"OKTA_CLIENT_SECRET",

	// Service account / private key authentication.
	"OKTA_PRIVATE_KEY",
	"OKTA_PRIVATE_KEY_ID",

	// OAuth scopes (controlled by provider config).
	"OKTA_SCOPES",

	// Access tokens (we set this explicitly).
	"OKTA_OAUTH2_ACCESS_TOKEN",
}

// PrepareEnvironmentConfig holds configuration for Okta environment preparation.
type PrepareEnvironmentConfig struct {
	// Environ is the current environment variables.
	Environ map[string]string

	// OrgURL is the Okta organization URL (e.g., https://company.okta.com).
	OrgURL string

	// AccessToken is the OAuth 2.0 access token for API access.
	AccessToken string

	// ConfigDir is the directory containing Okta token files (optional).
	ConfigDir string
}

// PrepareEnvironment configures environment variables for Okta SDK when using Atmos auth.
//
// This function:
//  1. Clears direct Okta credential env vars to prevent conflicts with Atmos-managed credentials
//  2. Sets OKTA_ORG_URL and OKTA_BASE_URL for SDK compatibility
//  3. Sets OKTA_OAUTH2_ACCESS_TOKEN for Terraform Okta provider
//  4. Optionally sets OKTA_CONFIG_DIR if a config directory is provided
//
// Note: Other cloud provider credentials (AWS, Azure, GCP) are NOT cleared to support multi-cloud
// scenarios such as using S3 backend for Terraform state while authenticating to Okta.
//
// Returns a NEW map with modifications - does not mutate the input.
func PrepareEnvironment(cfg PrepareEnvironmentConfig) map[string]string {
	defer perf.Track(nil, "pkg/auth/cloud/okta.PrepareEnvironment")()

	log.Debug("Preparing Okta environment for Atmos-managed credentials",
		"org_url", cfg.OrgURL,
		"has_access_token", cfg.AccessToken != "",
		"config_dir", cfg.ConfigDir,
	)

	// Create a copy to avoid mutating the input.
	result := make(map[string]string, len(cfg.Environ)+10)
	for k, v := range cfg.Environ {
		result[k] = v
	}

	// Clear problematic Okta credential environment variables.
	// These would override Atmos-managed credentials.
	// Note: We do NOT clear AWS/Azure/GCP credentials to support multi-cloud scenarios.
	for _, key := range problematicOktaEnvVars {
		if _, exists := result[key]; exists {
			log.Debug("Clearing Okta credential environment variable", "key", key)
			delete(result, key)
		}
	}

	// Set Okta organization URL for SDKs and Terraform providers.
	// OKTA_ORG_URL is the primary variable used by the Okta Terraform provider.
	// OKTA_BASE_URL is used by some older integrations and SDKs.
	if cfg.OrgURL != "" {
		result["OKTA_ORG_URL"] = cfg.OrgURL
		result["OKTA_BASE_URL"] = cfg.OrgURL
		log.Debug("Set Okta organization URL",
			"OKTA_ORG_URL", cfg.OrgURL,
			"OKTA_BASE_URL", cfg.OrgURL,
		)
	}

	// Set OAuth 2.0 access token for Terraform Okta provider.
	// This enables the provider to authenticate using the OAuth token obtained via device code.
	if cfg.AccessToken != "" {
		result["OKTA_OAUTH2_ACCESS_TOKEN"] = cfg.AccessToken
		log.Debug("Set Okta OAuth2 access token")
	}

	// Optionally set config directory for token files.
	// This allows Okta CLI or custom tools to find Atmos-managed tokens.
	if cfg.ConfigDir != "" {
		result["OKTA_CONFIG_DIR"] = cfg.ConfigDir
		log.Debug("Set Okta config directory", "OKTA_CONFIG_DIR", cfg.ConfigDir)
	}

	log.Debug("Okta environment prepared",
		"has_org_url", cfg.OrgURL != "",
		"has_access_token", cfg.AccessToken != "",
		"cleared_vars", len(problematicOktaEnvVars),
	)

	return result
}
