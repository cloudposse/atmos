package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/auth/authstore"
	"github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// assumeRoleIdentity implements AWS assume role identity
type assumeRoleIdentity struct {
	name   string
	config *schema.Identity
}

// NewAssumeRoleIdentity creates a new AWS assume role identity
func NewAssumeRoleIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config.Kind != "aws/assume-role" {
		return nil, fmt.Errorf("invalid identity kind for assume role: %s", config.Kind)
	}

	return &assumeRoleIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind
func (i *assumeRoleIdentity) Kind() string {
	return "aws/assume-role"
}

// assumeRoleCache represents cached assume role credentials
type assumeRoleCache struct {
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	SessionToken    string    `json:"session_token"`
	Region          string    `json:"region"`
	Expiration      time.Time `json:"expiration"`
	LastUpdated     time.Time `json:"last_updated"`
}

// Authenticate performs authentication using assume role
func (i *assumeRoleIdentity) Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error) {
	// Check cache first
	if creds := i.checkCache(); creds != nil {
		log.Debug("Using cached assume role credentials", "identity", i.name, "accessKeyId", creds.AWS.AccessKeyID[:10]+"...")
		return creds, nil
	}

	if baseCreds == nil || baseCreds.AWS == nil {
		return nil, fmt.Errorf("base AWS credentials are required")
	}

	log.Debug("Assume role authentication with base credentials", "identity", i.name, "baseAccessKeyId", baseCreds.AWS.AccessKeyID[:10]+"...", "hasSecretKey", baseCreds.AWS.SecretAccessKey != "", "hasSessionToken", baseCreds.AWS.SessionToken != "")

	// Get role ARN from spec
	roleArn, ok := i.config.Spec["assume_role"].(string)
	if !ok || roleArn == "" {
		return nil, fmt.Errorf("assume_role is required in spec")
	}

	// Create AWS config using base credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(baseCreds.AWS.Region),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     baseCreds.AWS.AccessKeyID,
				SecretAccessKey: baseCreds.AWS.SecretAccessKey,
				SessionToken:    baseCreds.AWS.SessionToken,
			}, nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	// Assume the role
	sessionName := fmt.Sprintf("atmos-%s-%d", i.name, time.Now().Unix())
	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String(sessionName),
	}

	// Add external ID if specified
	if externalID, ok := i.config.Spec["external_id"].(string); ok && externalID != "" {
		assumeRoleInput.ExternalId = aws.String(externalID)
	}

	// Add duration if specified
	if durationStr, ok := i.config.Spec["duration"].(string); ok && durationStr != "" {
		if duration, err := time.ParseDuration(durationStr); err == nil {
			assumeRoleInput.DurationSeconds = aws.Int32(int32(duration.Seconds()))
		}
	}

	result, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to assume role: %w", err)
	}

	// Convert to our credential format
	expiration := ""
	expirationTime := time.Time{}
	if result.Credentials.Expiration != nil {
		expirationTime = *result.Credentials.Expiration
		expiration = expirationTime.Format(time.RFC3339)
	}

	creds := &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
			SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
			SessionToken:    aws.ToString(result.Credentials.SessionToken),
			Region:          baseCreds.AWS.Region,
			Expiration:      expiration,
		},
	}

	// Cache the credentials
	i.cacheCredentials(creds, expirationTime)

	return creds, nil
}

// Validate validates the identity configuration
func (i *assumeRoleIdentity) Validate() error {
	if i.config.Spec == nil {
		return fmt.Errorf("spec is required")
	}

	roleArn, ok := i.config.Spec["assume_role"].(string)
	if !ok || roleArn == "" {
		return fmt.Errorf("assume_role is required in spec")
	}

	return nil
}

// Environment returns environment variables for this identity
func (i *assumeRoleIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Add environment variables from identity config
	for _, envVar := range i.config.Environment {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// Merge merges this identity configuration with component-level overrides
func (i *assumeRoleIdentity) Merge(component *schema.Identity) types.Identity {
	merged := &assumeRoleIdentity{
		name: i.name,
		config: &schema.Identity{
			Kind:        i.config.Kind,
			Default:     component.Default, // Component can override default
			Via:         i.config.Via,
			Spec:        make(map[string]interface{}),
			Alias:       i.config.Alias,
			Environment: i.config.Environment,
		},
	}

	// Merge spec
	for k, v := range i.config.Spec {
		merged.config.Spec[k] = v
	}
	for k, v := range component.Spec {
		merged.config.Spec[k] = v // Component overrides
	}

	// Merge environment variables
	merged.config.Environment = append(merged.config.Environment, component.Environment...)

	// Override alias if provided
	if component.Alias != "" {
		merged.config.Alias = component.Alias
	}

	return merged
}

// checkCache checks for valid cached assume role credentials
func (i *assumeRoleIdentity) checkCache() *schema.Credentials {
	store := authstore.NewKeyringAuthStore()
	cacheKey := fmt.Sprintf(credentials.KeyringService, i.Kind(), i.getProviderName(), i.name)

	var cache assumeRoleCache
	if err := store.GetAny(cacheKey, &cache); err != nil {
		log.Debug("No cache or error reading cache", "identity", i.name, "error", err)
		return nil // No cache or error reading cache
	}
	log.Debug("Found cache", "identity", i.name)

	// Check if cache is expired (with 5 minute buffer)
	if time.Now().Add(5 * time.Minute).After(cache.Expiration) {
		// Cache expired, remove it
		store.Delete(cacheKey)
		return nil
	}

	return &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     cache.AccessKeyID,
			SecretAccessKey: cache.SecretAccessKey,
			SessionToken:    cache.SessionToken,
			Region:          cache.Region,
			Expiration:      cache.Expiration.Format(time.RFC3339),
		},
	}
}

// cacheCredentials stores assume role credentials in keyring
func (i *assumeRoleIdentity) cacheCredentials(creds *schema.Credentials, expiration time.Time) {
	cacheKey := fmt.Sprintf(credentials.KeyringService, i.Kind(), i.getProviderName(), i.name)
	store := authstore.NewKeyringAuthStore()

	cache := assumeRoleCache{
		AccessKeyID:     creds.AWS.AccessKeyID,
		SecretAccessKey: creds.AWS.SecretAccessKey,
		SessionToken:    creds.AWS.SessionToken,
		Region:          creds.AWS.Region,
		Expiration:      expiration,
		LastUpdated:     time.Now(),
	}

	log.Debug("Caching assume role credentials", "key", cacheKey)
	if err := store.SetAny(cacheKey, cache); err != nil {
		// Don't fail authentication if caching fails, just log
		log.Warn("Failed to cache assume role credentials", "error", err)
	}
}

// getProviderName extracts the provider name from the identity configuration
func (i *assumeRoleIdentity) getProviderName() string {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider
	}
	if i.config.Via != nil && i.config.Via.Identity != "" {
		// This assume role identity chains through another identity
		// For caching purposes, we'll use the chained identity name
		return i.config.Via.Identity
	}
	return "unknown"
}
