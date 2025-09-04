package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/huh"

	atmosCredentials "github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/environment"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// userIdentity implements AWS user identity (passthrough)
type userIdentity struct {
	name   string
	config *schema.Identity
}

// NewUserIdentity creates a new AWS user identity
func NewUserIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config.Kind != "aws/user" {
		return nil, fmt.Errorf("invalid identity kind for user: %s", config.Kind)
	}

	return &userIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind
func (i *userIdentity) Kind() string {
	return "aws/user"
}

// Authenticate performs authentication by retrieving credentials from keyring
func (i *userIdentity) Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error) {
	// For AWS User identities, retrieve credentials from credential store
	credStore := atmosCredentials.NewCredentialStore()
	
	creds, err := credStore.Retrieve(i.name)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve AWS User credentials for %q: %w", i.name, err)
	}
	
	// Get region from identity spec
	region := "us-east-1" // default
	if r, ok := i.config.Spec["region"].(string); ok && r != "" {
		region = r
	}
	
	// Set region in credentials if not already set
	if creds.AWS.Region == "" {
		creds.AWS.Region = region
	}
	
	// If MFA ARN is configured, get session token with MFA
	if creds.AWS.MfaArn != "" {
		secret := userSecret{
			AccessKeyID:     creds.AWS.AccessKeyID,
			SecretAccessKey: creds.AWS.SecretAccessKey,
			MfaArn:          creds.AWS.MfaArn,
		}
		return i.authenticateWithMFA(ctx, secret, region)
	}
	
	// Write credentials to AWS files using "aws-user" as mock provider
	if err := i.writeAWSFiles(creds, region); err != nil {
		return nil, fmt.Errorf("failed to write AWS files: %w", err)
	}
	
	return creds, nil
}

// writeAWSFiles writes credentials to AWS config files using "aws-user" as mock provider
func (i *userIdentity) writeAWSFiles(creds *schema.Credentials, region string) error {
	awsFileManager := environment.NewAWSFileManager()
	
	// Write credentials to ~/.aws/atmos/aws-user/credentials
	if err := awsFileManager.WriteCredentials("aws-user", i.name, creds.AWS); err != nil {
		return fmt.Errorf("failed to write AWS credentials: %w", err)
	}
	
	// Write config to ~/.aws/atmos/aws-user/config
	if err := awsFileManager.WriteConfig("aws-user", i.name, region); err != nil {
		return fmt.Errorf("failed to write AWS config: %w", err)
	}
	
	return nil
}

// authenticateWithMFA handles MFA authentication for AWS User identities
func (i *userIdentity) authenticateWithMFA(ctx context.Context, secret userSecret, region string) (*schema.Credentials, error) {
	// Create AWS config with base credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			secret.AccessKeyID,
			secret.SecretAccessKey,
			"", // no session token for base credentials
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	// Prompt for MFA token
	var mfaToken string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter MFA Token").
				Description(fmt.Sprintf("MFA Device: %s", secret.MfaArn)).
				Value(&mfaToken).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("MFA token is required")
					}
					if len(s) != 6 {
						return fmt.Errorf("MFA token must be 6 digits")
					}
					return nil
				}),
		),
	)

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("failed to get MFA token: %w", err)
	}

	// Get session token with MFA
	input := &sts.GetSessionTokenInput{
		SerialNumber: aws.String(secret.MfaArn),
		TokenCode:    aws.String(mfaToken),
		DurationSeconds: aws.Int32(3600), // 1 hour session
	}

	result, err := stsClient.GetSessionToken(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get session token with MFA: %w", err)
	}

	// Convert to standard credentials format
	creds := &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     *result.Credentials.AccessKeyId,
			SecretAccessKey: *result.Credentials.SecretAccessKey,
			SessionToken:    *result.Credentials.SessionToken,
			Region:          region,
			Expiration:      result.Credentials.Expiration.Format(time.RFC3339),
		},
	}

	// Write credentials to AWS files using "aws-user" as mock provider
	if err := i.writeAWSFiles(creds, region); err != nil {
		return nil, fmt.Errorf("failed to write AWS files: %w", err)
	}

	// Store credentials in credential store for caching
	credStore := atmosCredentials.NewCredentialStore()
	if err := credStore.Store(i.name, creds); err != nil {
		return nil, fmt.Errorf("failed to store credentials: %w", err)
	}

	return creds, nil
}

// userSecret defines the structure used by auth user configure command
type userSecret struct {
	AccessKeyID     string    `json:"access_key_id"`
	SecretAccessKey string    `json:"secret_access_key"`
	MfaArn          string    `json:"mfa_arn,omitempty"`
	LastUpdated     time.Time `json:"last_updated"`
}

// Validate validates the identity configuration
func (i *userIdentity) Validate() error {
	// User identities don't require additional validation beyond the provider
	return nil
}

// Environment returns environment variables for this identity
func (i *userIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)
	
	// Get AWS file environment variables using "aws-user" as mock provider
	awsFileManager := environment.NewAWSFileManager()
	awsEnvVars := awsFileManager.GetEnvironmentVariables("aws-user", i.name)
	
	// Convert to map format
	for _, envVar := range awsEnvVars {
		env[envVar.Key] = envVar.Value
	}
	
	// Add environment variables from identity config
	for _, envVar := range i.config.Env {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// Merge merges this identity configuration with component-level overrides
func (i *userIdentity) Merge(component *schema.Identity) types.Identity {
	merged := &userIdentity{
		name: i.name,
		config: &schema.Identity{
			Kind:        i.config.Kind,
			Default:     component.Default, // Component can override default
			Via:         i.config.Via,
			Spec:        make(map[string]interface{}),
			Alias:       i.config.Alias,
			Env: i.config.Env,
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
	merged.config.Env = append(merged.config.Env, component.Env...)

	// Override alias if provided
	if component.Alias != "" {
		merged.config.Alias = component.Alias
	}

	return merged
}
