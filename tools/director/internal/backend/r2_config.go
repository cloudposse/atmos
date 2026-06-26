package backend

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	// ErrMissingR2Credentials is returned when required Cloudflare R2 credentials are not set.
	ErrMissingR2Credentials = errors.New("missing Cloudflare R2 credentials")

	// ErrR2Validation is returned when R2 backend validation fails.
	ErrR2Validation = errors.New("R2 backend validation failed")
)

// R2Config holds the configuration for Cloudflare R2 backend.
type R2Config struct {
	AccountID   string
	AccessKeyID string
	SecretKey   string
	BucketName  string
	Prefix      string
	BaseURL     string
}

// LoadR2Config loads R2 configuration from environment variables and backend config.
// It validates that all required credentials are present and returns a friendly error if not.
func LoadR2Config(backendConfig map[string]interface{}) (*R2Config, error) {
	config := &R2Config{}

	// Load credentials from environment variables.
	config.AccountID = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	config.AccessKeyID = os.Getenv("CLOUDFLARE_R2_ACCESS_KEY_ID")
	config.SecretKey = os.Getenv("CLOUDFLARE_R2_SECRET_ACCESS_KEY")

	// Validate required credentials.
	var missingVars []string
	if config.AccountID == "" {
		missingVars = append(missingVars, "CLOUDFLARE_ACCOUNT_ID")
	}
	if config.AccessKeyID == "" {
		missingVars = append(missingVars, "CLOUDFLARE_R2_ACCESS_KEY_ID")
	}
	if config.SecretKey == "" {
		missingVars = append(missingVars, "CLOUDFLARE_R2_SECRET_ACCESS_KEY")
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("%w\n\nRequired environment variables:\n  - %s\n\nSet these in your environment or .env file",
			ErrMissingR2Credentials,
			strings.Join(missingVars, "\n  - "))
	}

	// Load backend configuration from defaults.yaml.
	if backendConfig != nil {
		if r2Config, ok := backendConfig["r2"].(map[string]interface{}); ok {
			if bucketName, ok := r2Config["bucket_name"].(string); ok {
				config.BucketName = bucketName
			}
			if prefix, ok := r2Config["prefix"].(string); ok {
				config.Prefix = prefix
			}
			if baseURL, ok := r2Config["base_url"].(string); ok {
				config.BaseURL = baseURL
			}
		}
	}

	// Allow environment variable override for bucket name.
	if envBucket := os.Getenv("CLOUDFLARE_BUCKET_NAME"); envBucket != "" {
		config.BucketName = envBucket
	}

	// Allow environment variable override for base URL.
	if envBaseURL := os.Getenv("CLOUDFLARE_R2_BASE_URL"); envBaseURL != "" {
		config.BaseURL = envBaseURL
	}

	// Validate bucket name is set.
	if config.BucketName == "" {
		return nil, fmt.Errorf("%w: bucket_name not configured in defaults.yaml or CLOUDFLARE_BUCKET_NAME environment variable",
			ErrR2Validation)
	}

	return config, nil
}

// GetEndpoint returns the S3-compatible endpoint URL for Cloudflare R2.
func (c *R2Config) GetEndpoint() string {
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", c.AccountID)
}
