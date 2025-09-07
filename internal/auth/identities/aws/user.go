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
	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/internal/auth/cloud/aws"
	atmosCredentials "github.com/cloudposse/atmos/internal/auth/credentials"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// userIdentity implements AWS user identity (passthrough).
type userIdentity struct {
	name   string
	config *schema.Identity
}

// NewUserIdentity creates a new AWS user identity.
func NewUserIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config.Kind != "aws/user" {
		return nil, fmt.Errorf("%w: invalid identity kind for user: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	return &userIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *userIdentity) Kind() string {
	return "aws/user"
}

// GetProviderName returns the provider name for this identity
// AWS user identities always return "aws-user" as they are standalone.
func (i *userIdentity) GetProviderName() (string, error) {
	return "aws-user", nil
}

// Authenticate performs authentication by retrieving long-lived credentials and generating session tokens.
func (i *userIdentity) Authenticate(ctx context.Context, baseCreds *types.Credentials) (*types.Credentials, error) {
	var longLivedCreds *types.Credentials
	var err error

	// Check if credentials are configured in atmos.yaml (environment templating)
	if accessKeyID, ok := i.config.Credentials["access_key_id"].(string); ok && accessKeyID != "" {
		// Credentials are configured in atmos.yaml - use them directly
		secretAccessKey, _ := i.config.Credentials["secret_access_key"].(string)
		mfaArn, _ := i.config.Credentials["mfa_arn"].(string)

		longLivedCreds = &types.Credentials{
			AWS: &types.AWSCredentials{
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
			return nil, fmt.Errorf("%w: failed to retrieve AWS User credentials for %q: %w", errUtils.ErrAwsUserNotConfigured, i.name, err)
		}

		log.Debug("Using credentials from keyring", "identity", i.name)
	}

	// Get region from identity credentials
	region := "us-east-1" // default
	if r, ok := i.config.Credentials["region"].(string); ok && r != "" {
		region = r
	}

	log.Debug("AWS User region extracted.", "identity", i.name, "region", region)

	// Validate and set region in credentials.
	if longLivedCreds == nil || longLivedCreds.AWS == nil {
		return nil, fmt.Errorf(errUtils.ErrStringWrappingFormat, errUtils.ErrAwsUserNotConfigured, "Have you ran `atmos auth user configure`?")
	}
	if longLivedCreds.AWS.Region == "" {
		longLivedCreds.AWS.Region = region
	}

	// Always generate session tokens (with or without MFA)
	return i.generateSessionToken(ctx, longLivedCreds, region)
}

// writeAWSFiles writes credentials to AWS config files using "aws-user" as mock provider.
func (i *userIdentity) writeAWSFiles(creds *types.Credentials, region string) error {
	awsFileManager := awsCloud.NewAWSFileManager()

	// Write credentials to ~/.aws/atmos/aws-user/credentials
	if err := awsFileManager.WriteCredentials("aws-user", i.name, creds.AWS); err != nil {
		return fmt.Errorf("%w: failed to write AWS credentials: %v", errUtils.ErrAwsAuth, err)
	}

	// Write config to ~/.aws/atmos/aws-user/config
	if err := awsFileManager.WriteConfig("aws-user", i.name, region, ""); err != nil {
		return fmt.Errorf("%w: failed to write AWS config: %v", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// generateSessionToken generates session tokens for AWS User identities (with or without MFA).
func (i *userIdentity) generateSessionToken(ctx context.Context, longLivedCreds *types.Credentials, region string) (*types.Credentials, error) {
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
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrAwsAuth, err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	var input *sts.GetSessionTokenInput

	// Check if MFA is required
	if longLivedCreds.AWS.MfaArn != "" {
		// Prompt for MFA token
		var mfaToken string
		form := newMfaForm(longLivedCreds, &mfaToken)

		if err := form.Run(); err != nil {
			return nil, fmt.Errorf("%w: failed to get MFA token: %v", errUtils.ErrUnsupportedInputType, err)
		}

		// Get session token with MFA
		input = &sts.GetSessionTokenInput{
			SerialNumber:    aws.String(longLivedCreds.AWS.MfaArn),
			TokenCode:       aws.String(mfaToken),
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
		return nil, fmt.Errorf("%w: failed to get session token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Create session credentials (temporary tokens for AWS files)
	sessionCreds := &types.Credentials{
		AWS: &types.AWSCredentials{
			AccessKeyID:     *result.Credentials.AccessKeyId,
			SecretAccessKey: *result.Credentials.SecretAccessKey,
			SessionToken:    *result.Credentials.SessionToken,
			Region:          region,
			Expiration:      result.Credentials.Expiration.Format(time.RFC3339),
		},
	}

	// Write session credentials to AWS files using "aws-user" as mock provider
	if err := i.writeAWSFiles(sessionCreds, region); err != nil {
		return nil, fmt.Errorf("%w: failed to write AWS files: %v", errUtils.ErrAwsAuth, err)
	}

	// Note: We keep the long-lived credentials in the keystore unchanged
	// Only the session tokens are written to AWS config/credentials files

	return sessionCreds, nil
}

func newMfaForm(longLivedCreds *types.Credentials, mfaToken *string) *huh.Form {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter MFA Token").
				Description(fmt.Sprintf("MFA Device: %s", longLivedCreds.AWS.MfaArn)).
				Value(mfaToken).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("%w: MFA token is required", errUtils.ErrAwsAuth)
					}
					if len(s) != 6 {
						return fmt.Errorf("%w: MFA token must be 6 digits", errUtils.ErrAwsAuth)
					}
					return nil
				}),
		),
	)
	return form
}

// Validate validates the identity configuration.
func (i *userIdentity) Validate() error {
	// User identities don't require additional validation beyond the provider
	return nil
}

// Environment returns environment variables for this identity.
func (i *userIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Get AWS file environment variables using "aws-user" as mock provider
	awsFileManager := awsCloud.NewAWSFileManager()
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

// IsStandaloneAWSUserChain checks if the authentication chain represents a standalone AWS user identity.
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

// AuthenticateStandaloneAWSUser handles authentication for standalone AWS user identities.
func AuthenticateStandaloneAWSUser(ctx context.Context, identityName string, identities map[string]types.Identity) (*types.Credentials, error) {
	log.Debug("Authenticating AWS user identity directly", "identity", identityName)

	// Get the identity instance
	userIdentity, exists := identities[identityName]
	if !exists {
		return nil, fmt.Errorf("%w: AWS user identity %q not found", errUtils.ErrInvalidAuthConfig, identityName)
	}

	// AWS user identities authenticate directly without provider credentials.
	credentials, err := userIdentity.Authenticate(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: AWS user identity %q authentication failed: %v", errUtils.ErrAuthenticationFailed, identityName, err)
	}

	log.Debug("AWS user identity authenticated successfully", "identity", identityName)
	return credentials, nil
}

// PostAuthenticate sets up AWS files after authentication.
func (i *userIdentity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds *types.Credentials) error {
	// Setup AWS files using shared AWS cloud package
	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %v", errUtils.ErrAwsAuth, err)
	}
	if err := awsCloud.SetEnvironmentVariables(stackInfo, providerName, identityName); err != nil {
		return fmt.Errorf("%w: failed to set environment variables: %v", errUtils.ErrAwsAuth, err)
	}
	return nil
}
