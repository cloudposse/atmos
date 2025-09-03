package aws

import (
	"context"
	"fmt"
	"time"

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
	// For user providers, credentials are typically stored in keyring
	// This is a placeholder - actual implementation would retrieve from secure storage
	
	// Get credentials from spec or environment
	accessKeyID, ok := p.config.Spec["access_key_id"].(string)
	if !ok || accessKeyID == "" {
		return nil, fmt.Errorf("access_key_id is required in provider spec")
	}

	secretAccessKey, ok := p.config.Spec["secret_access_key"].(string)
	if !ok || secretAccessKey == "" {
		return nil, fmt.Errorf("secret_access_key is required in provider spec")
	}

	// Session token is optional
	sessionToken, _ := p.config.Spec["session_token"].(string)

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
	if p.config.Spec == nil {
		return fmt.Errorf("spec is required")
	}
	
	accessKeyID, ok := p.config.Spec["access_key_id"].(string)
	if !ok || accessKeyID == "" {
		return fmt.Errorf("access_key_id is required in spec")
	}
	
	secretAccessKey, ok := p.config.Spec["secret_access_key"].(string)
	if !ok || secretAccessKey == "" {
		return fmt.Errorf("secret_access_key is required in spec")
	}
	
	return nil
}

// Environment returns environment variables for this provider
func (p *userProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	env["AWS_REGION"] = p.region
	return env, nil
}
