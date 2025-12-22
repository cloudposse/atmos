package backend

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	// ErrMissingStreamCredentials is returned when required Cloudflare Stream credentials are not set.
	ErrMissingStreamCredentials = errors.New("missing Cloudflare Stream credentials")

	// ErrStreamValidation is returned when Stream backend validation fails.
	ErrStreamValidation = errors.New("Stream backend validation failed")
)

// StreamConfig holds the configuration for Cloudflare Stream backend.
type StreamConfig struct {
	AccountID         string
	APIToken          string
	CustomerSubdomain string // Optional: extracted from API response if not provided
}

// LoadStreamConfig loads Stream configuration from environment variables and backend config.
// It validates that all required credentials are present and returns a friendly error if not.
func LoadStreamConfig(backendConfig map[string]interface{}) (*StreamConfig, error) {
	config := &StreamConfig{}

	// Load credentials from environment variables.
	config.AccountID = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	config.APIToken = os.Getenv("CLOUDFLARE_STREAM_API_TOKEN")

	// Validate required credentials.
	var missingVars []string
	if config.AccountID == "" {
		missingVars = append(missingVars, "CLOUDFLARE_ACCOUNT_ID")
	}
	if config.APIToken == "" {
		missingVars = append(missingVars, "CLOUDFLARE_STREAM_API_TOKEN")
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("%w\n\nRequired environment variables:\n  - %s\n\nSet these in your environment or .env file",
			ErrMissingStreamCredentials,
			strings.Join(missingVars, "\n  - "))
	}

	// Load optional customer subdomain from config.
	if backendConfig != nil {
		if streamConfig, ok := backendConfig["stream"].(map[string]interface{}); ok {
			if subdomain, ok := streamConfig["customer_subdomain"].(string); ok {
				config.CustomerSubdomain = subdomain
			}
		}
	}

	// Allow environment variable override for customer subdomain.
	if envSubdomain := os.Getenv("CLOUDFLARE_CUSTOMER_SUBDOMAIN"); envSubdomain != "" {
		config.CustomerSubdomain = envSubdomain
	}

	return config, nil
}
