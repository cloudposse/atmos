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
	"github.com/charmbracelet/log"
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

// Authenticate performs authentication by retrieving long-lived credentials and generating session tokens
func (i *userIdentity) Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error) {
	var longLivedCreds *schema.Credentials
	var err error
	
	// Check if credentials are configured in atmos.yaml (environment templating)
	if accessKeyID, ok := i.config.Credentials["access_key_id"].(string); ok && accessKeyID != "" {
		// Credentials are configured in atmos.yaml - use them directly
		secretAccessKey, _ := i.config.Credentials["secret_access_key"].(string)
		mfaArn, _ := i.config.Credentials["mfa_arn"].(string)
		
		longLivedCreds = &schema.Credentials{
			AWS: &schema.AWSCredentials{
				AccessKeyID:     accessKeyID,
				SecretAccessKey: secretAccessKey,
				MfaArn:          mfaArn,
			},
		}
		
		log.Debug("Using credentials from atmos.yaml configuration", "identity", i.name, "hasAccessKey", accessKeyID != "", "hasMFA", mfaArn != "")
	} else {
		// Fallback to credential store (keyring) for stored credentials
		credStore := atmosCredentials.NewCredentialStore()
		longLivedCreds, err = credStore.Retrieve(i.name)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve AWS User credentials for %q: %w", i.name, err)
		}
		
		log.Debug("Using credentials from keyring", "identity", i.name)
	}
	
	// Get region from identity credentials
	region := "us-east-1" // default
	if r, ok := i.config.Credentials["region"].(string); ok && r != "" {
		region = r
	}
	
	// Debug logging
	log.Debug("AWS User region extraction", "identity", i.name, "region", region, "credentials", i.config.Credentials)
	
	// Set region in credentials if not already set
	if longLivedCreds.AWS.Region == "" {
		longLivedCreds.AWS.Region = region
	}
	
	// Always generate session tokens (with or without MFA)
	return i.generateSessionToken(ctx, longLivedCreds, region)
}

// writeAWSFiles writes credentials to AWS config files using "aws-user" as mock provider
func (i *userIdentity) writeAWSFiles(creds *schema.Credentials, region string) error {
	awsFileManager := environment.NewAWSFileManager()
	
	// Debug logging
	log.Debug("Writing AWS files", "identity", i.name, "region", region, "creds_region", creds.AWS.Region)
	
	// Write credentials to ~/.aws/atmos/aws-user/credentials
	if err := awsFileManager.WriteCredentials("aws-user", i.name, creds.AWS); err != nil {
		return fmt.Errorf("failed to write AWS credentials: %w", err)
	}
	
	// Write config to ~/.aws/atmos/aws-user/config
	if err := awsFileManager.WriteConfig("aws-user", i.name, region, ""); err != nil {
		return fmt.Errorf("failed to write AWS config: %w", err)
	}
	
	return nil
}

// generateSessionToken generates session tokens for AWS User identities (with or without MFA)
func (i *userIdentity) generateSessionToken(ctx context.Context, longLivedCreds *schema.Credentials, region string) (*schema.Credentials, error) {
	// Create AWS config with long-lived credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			longLivedCreds.AWS.AccessKeyID,
			longLivedCreds.AWS.SecretAccessKey,
			"", // no session token for long-lived credentials
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	var input *sts.GetSessionTokenInput
	
	// Check if MFA is required
	if longLivedCreds.AWS.MfaArn != "" {
		// Prompt for MFA token
		var mfaToken string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter MFA Token").
					Description(fmt.Sprintf("MFA Device: %s", longLivedCreds.AWS.MfaArn)).
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
		input = &sts.GetSessionTokenInput{
			SerialNumber: aws.String(longLivedCreds.AWS.MfaArn),
			TokenCode:    aws.String(mfaToken),
			DurationSeconds: aws.Int32(3600), // 1 hour session
		}
	} else {
		// Get session token without MFA
		input = &sts.GetSessionTokenInput{
			DurationSeconds: aws.Int32(3600), // 1 hour session
		}
	}

	result, err := stsClient.GetSessionToken(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get session token: %w", err)
	}

	// Create session credentials (temporary tokens for AWS files)
	sessionCreds := &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     *result.Credentials.AccessKeyId,
			SecretAccessKey: *result.Credentials.SecretAccessKey,
			SessionToken:    *result.Credentials.SessionToken,
			Region:          region,
			Expiration:      result.Credentials.Expiration.Format(time.RFC3339),
		},
	}

	// Write session credentials to AWS files using "aws-user" as mock provider
	if err := i.writeAWSFiles(sessionCreds, region); err != nil {
		return nil, fmt.Errorf("failed to write AWS files: %w", err)
	}

	// Note: We keep the long-lived credentials in the keystore unchanged
	// Only the session tokens are written to AWS config/credentials files
	
	return sessionCreds, nil
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
			Principal:   make(map[string]interface{}),
			Credentials: make(map[string]interface{}),
			Alias:       i.config.Alias,
			Env: i.config.Env,
		},
	}

	// Merge principal
	for k, v := range i.config.Principal {
		merged.config.Principal[k] = v
	}
	for k, v := range component.Principal {
		merged.config.Principal[k] = v // Component overrides
	}

	// Merge credentials
	for k, v := range i.config.Credentials {
		merged.config.Credentials[k] = v
	}
	for k, v := range component.Credentials {
		merged.config.Credentials[k] = v // Component overrides
	}

	// Merge environment variables
	merged.config.Env = append(merged.config.Env, component.Env...)

	// Override alias if provided
	if component.Alias != "" {
		merged.config.Alias = component.Alias
	}

	return merged
}

// IsStandaloneAWSUserChain checks if the authentication chain represents a standalone AWS user identity
func IsStandaloneAWSUserChain(chain []string, identities map[string]schema.Identity) bool {
	if len(chain) != 1 {
		return false
	}
	
	identityName := chain[0]
	if identity, exists := identities[identityName]; exists {
		return identity.Kind == "aws/user"
	}
	
	return false
}

// AuthenticateStandaloneAWSUser handles authentication for standalone AWS user identities
func AuthenticateStandaloneAWSUser(ctx context.Context, identityName string, identities map[string]types.Identity) (*schema.Credentials, error) {
	log.Debug("Authenticating AWS user identity directly", "identity", identityName)
	
	// Get the identity instance
	userIdentity, exists := identities[identityName]
	if !exists {
		return nil, fmt.Errorf("AWS user identity %q not found", identityName)
	}
	
	// AWS user identities authenticate directly without provider credentials
	credentials, err := userIdentity.Authenticate(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("AWS user identity %q authentication failed: %w", identityName, err)
	}
	
	log.Debug("AWS user identity authenticated successfully", "identity", identityName)
	return credentials, nil
}
