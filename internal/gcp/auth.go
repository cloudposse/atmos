package gcp

import (
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AuthOptions contains configuration for Google Cloud authentication.
type AuthOptions struct {
	// Credentials can be either:
	// - JSON content (if starts with "{")
	// - File path to service account JSON file
	// - Empty string to use Application Default Credentials (ADC)
	Credentials string

	// AccessToken is an OAuth2 access token to use directly. This is preferred
	// by Atmos GCP WIF/service-account identities because their generated ADC
	// files do not contain refresh tokens.
	AccessToken string //nolint:gosec // Intentional credential field passed directly to Google client options.
	TokenExpiry time.Time

	// Endpoint overrides the default Google API endpoint. This is primarily used
	// by local emulators and integration tests.
	Endpoint string

	// EndpointInsecure allows plaintext gRPC for local endpoints.
	EndpointInsecure bool

	// WithoutAuthentication disables Google authentication for local endpoints
	// that do not validate credentials.
	WithoutAuthentication bool

	// TODO: Add support for service account impersonation
	// ImpersonateServiceAccount string
}

// GetClientOptions returns Google Cloud client options based on the provided authentication configuration.
// This function provides unified authentication handling across all Google Cloud services in Atmos.
//
// Authentication precedence:
// 1. Explicit credentials (JSON content or file path)
// 2. GOOGLE_OAUTH_ACCESS_TOKEN environment variable (static access token)
// 3. Application Default Credentials (ADC) which automatically handles:
//   - GOOGLE_APPLICATION_CREDENTIALS environment variable
//   - Compute Engine metadata service
//   - Cloud Shell credentials
//   - gcloud user credentials (from `gcloud auth application-default login`)
//   - Workload Identity (in GKE)
func GetClientOptions(opts AuthOptions) []option.ClientOption {
	var clientOpts []option.ClientOption

	if opts.Endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(normalizeEndpoint(opts.Endpoint)))
	}
	if opts.EndpointInsecure {
		clientOpts = append(clientOpts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}
	if opts.WithoutAuthentication {
		clientOpts = append(clientOpts, option.WithoutAuthentication())
		return clientOpts
	}

	if opts.Credentials != "" {
		// Determine if credentials are JSON content or file path
		if strings.HasPrefix(strings.TrimSpace(opts.Credentials), "{") {
			// JSON content
			clientOpts = append(clientOpts, option.WithCredentialsJSON([]byte(opts.Credentials)))
		} else {
			// File path
			clientOpts = append(clientOpts, option.WithCredentialsFile(opts.Credentials))
		}
		return clientOpts
	}

	if opts.AccessToken != "" {
		token := &oauth2.Token{
			AccessToken: opts.AccessToken,
			Expiry:      opts.TokenExpiry,
		}
		tokenSource := oauth2.StaticTokenSource(token)
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
		return clientOpts
	}

	// Check for GOOGLE_OAUTH_ACCESS_TOKEN environment variable.
	// This is set by Atmos GCP auth when using service account impersonation.
	// The Google Cloud SDK doesn't automatically use this env var, so we need
	// to handle it explicitly by creating a static token source.
	if accessToken := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN"); accessToken != "" {
		token := &oauth2.Token{AccessToken: accessToken}
		tokenSource := oauth2.StaticTokenSource(token)
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
		return clientOpts
	}

	// If no explicit credentials or access token, Google Cloud client libraries
	// will automatically use ADC (Application Default Credentials).
	return clientOpts
}

// normalizeEndpoint trims surrounding whitespace and strips any http:// or
// https:// scheme prefix so the result is suitable for option.WithEndpoint.
func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	return endpoint
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
