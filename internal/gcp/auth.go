package gcp

import (
	"strings"

	"google.golang.org/api/option"
)

// AuthOptions contains configuration for Google Cloud authentication.
type AuthOptions struct {
	// Credentials can be either:
	// - JSON content (if starts with "{")
	// - File path to service account JSON file
	// - Empty string to use Application Default Credentials (ADC)
	Credentials string

	// TODO: Add support for service account impersonation
	// ImpersonateServiceAccount string
}

// GetClientOptions returns Google Cloud client options based on the provided authentication configuration.
// This function provides unified authentication handling across all Google Cloud services in Atmos.
//
// Authentication precedence:
// 1. Explicit credentials (JSON content or file path)
// 2. Application Default Credentials (ADC) which automatically handles:
//   - GOOGLE_APPLICATION_CREDENTIALS environment variable
//   - Compute Engine metadata service
//   - Cloud Shell credentials
//   - gcloud user credentials (from `gcloud auth application-default login`)
//   - Workload Identity (in GKE)
func GetClientOptions(opts AuthOptions) []option.ClientOption {
	var clientOpts []option.ClientOption

	if opts.Credentials != "" {
		// Determine if credentials are JSON content or file path
		if strings.HasPrefix(strings.TrimSpace(opts.Credentials), "{") {
			// JSON content
			clientOpts = append(clientOpts, option.WithCredentialsJSON([]byte(opts.Credentials)))
		} else {
			// File path
			clientOpts = append(clientOpts, option.WithCredentialsFile(opts.Credentials))
		}
	}
	// If no explicit credentials, Google Cloud client libraries will automatically use ADC

	return clientOpts
}

// GetCredentialsFromBackend extracts credentials from a Terraform backend configuration.
// This is used by the GCS Terraform backend.
func GetCredentialsFromBackend(backend map[string]any) string {
	if credentials, ok := backend["credentials"].(string); ok {
		return credentials
	}
	return ""
}

// GetCredentialsFromStore extracts credentials from a store configuration.
// This is used by the Google Secret Manager store.
func GetCredentialsFromStore(credentials *string) string {
	if credentials != nil {
		return *credentials
	}
	return ""
}
