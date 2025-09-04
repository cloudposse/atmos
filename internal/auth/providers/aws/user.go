package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/internal/auth/authstore"
	"github.com/cloudposse/atmos/pkg/schema"
)

// userProvider implements AWS user (static credentials) authentication
type userProvider struct {
	name   string
	config *schema.Provider
	region string
}

// NewUserProvider creates a new AWS user provider
func NewUserProvider(name string, providerConfig *schema.Provider) (*userProvider, error) {
	if providerConfig.Kind != "aws/user" {
		return nil, fmt.Errorf("invalid provider kind for user provider: %s", providerConfig.Kind)
	}

	region := providerConfig.Region
	if region == "" {
		region = "us-east-1" // Default region
	}

	return &userProvider{
		name:   name,
		config: providerConfig,
		region: region,
	}, nil
}

// Kind returns the provider kind
func (p *userProvider) Kind() string {
	return "aws/user"
}

// Authenticate performs AWS user authentication (returns static credentials)
func (p *userProvider) Authenticate(ctx context.Context) (*schema.Credentials, error) {
	var accessKeyID, secretAccessKey, sessionToken string
	
	// Try to get credentials from spec first (takes precedence)
	if specAccessKeyID, ok := p.config.Spec["access_key_id"].(string); ok && specAccessKeyID != "" {
		accessKeyID = specAccessKeyID
	}
	
	if specSecretAccessKey, ok := p.config.Spec["secret_access_key"].(string); ok && specSecretAccessKey != "" {
		secretAccessKey = specSecretAccessKey
	}
	
	// Session token is optional from spec
	sessionToken, _ = p.config.Spec["session_token"].(string)
	
	// If credentials not in spec, try to retrieve from keyring
	if accessKeyID == "" || secretAccessKey == "" {
		// Get identity name from provider name (assumes format provider/identity)
		// This will be passed from the auth manager when we know the identity
		// For now, we'll construct the alias based on provider name
		alias := fmt.Sprintf("%s/default", p.name) // Fallback alias
		
		store := authstore.NewKeyringAuthStore()
		type userSecret struct {
			AccessKeyID     string    `json:"access_key_id"`
			SecretAccessKey string    `json:"secret_access_key"`
			MfaArn          string    `json:"mfa_arn,omitempty"`
			LastUpdated     time.Time `json:"last_updated"`
		}
		
		var secret userSecret
		err := store.GetAny(alias, &secret)
		if err == nil {
			if accessKeyID == "" {
				accessKeyID = secret.AccessKeyID
			}
			if secretAccessKey == "" {
				secretAccessKey = secret.SecretAccessKey
			}
		}
	}
	
	// Validate that we have required credentials
	if accessKeyID == "" {
		return nil, fmt.Errorf("access_key_id is required in provider spec or keyring")
	}
	
	if secretAccessKey == "" {
		return nil, fmt.Errorf("secret_access_key is required in provider spec or keyring")
	}

	// User credentials don't typically expire, but we set a far future date
	expiration := time.Now().Add(24 * time.Hour).Format(time.RFC3339)

	return &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			SessionToken:    sessionToken,
			Region:          p.region,
			Expiration:      expiration,
		},
	}, nil
}

// Validate validates the provider configuration
func (p *userProvider) Validate() error {
	// For AWS user providers, credentials can come from either:
	// 1. Spec configuration (takes precedence)
	// 2. Keyring storage (configured via atmos auth user configure)
	// At validation time, we only require that the provider is properly configured
	// Actual credential validation happens during authentication
	return nil
}

// Environment returns environment variables for this provider
func (p *userProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	env["AWS_REGION"] = p.region
	return env, nil
}
