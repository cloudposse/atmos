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
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	atmosCredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	defaultUserSessionSeconds = 3600
	awsUserProviderName       = "aws-user"
	logKeyIdentity            = "identity"
	defaultRegion             = "us-east-1"
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
	return awsUserProviderName, nil
}

// Authenticate performs authentication by retrieving long-lived credentials and generating session tokens.
func (i *userIdentity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	var longLivedCreds *types.AWSCredentials

	// Check if credentials are configured in atmos.yaml (environment templating)
	if accessKeyID, ok := i.config.Credentials["access_key_id"].(string); ok && accessKeyID != "" {
		// Credentials are configured in atmos.yaml - use them directly.
		secretAccessKey, _ := i.config.Credentials["secret_access_key"].(string)
		if secretAccessKey == "" {
			return nil, fmt.Errorf("%w: access_key_id is set but secret_access_key is missing for identity %q", errUtils.ErrInvalidAuthConfig, i.name)
		}
		mfaArn, _ := i.config.Credentials["mfa_arn"].(string)

		longLivedCreds = &types.AWSCredentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			MfaArn:          mfaArn,
		}

		log.Debug("Using credentials from atmos.yaml configuration", logKeyIdentity, i.name, "hasAccessKey", accessKeyID != "", "hasMFA", mfaArn != "")
	} else {
		// Fallback to credential store (keyring) for stored credentials
		credStore := atmosCredentials.NewCredentialStore()
		retrieved, err := credStore.Retrieve(i.name)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to retrieve AWS User credentials for %q: %w", errUtils.ErrAwsUserNotConfigured, i.name, err)
		}
		var ok bool
		longLivedCreds, ok = retrieved.(*types.AWSCredentials)
		if !ok {
			return nil, fmt.Errorf("%w: stored credentials are not AWS credentials", errUtils.ErrAwsAuth)
		}

		log.Debug("Using credentials from keyring", logKeyIdentity, i.name)
	}

	// Get region from identity credentials
	region := defaultRegion // default
	if r, ok := i.config.Credentials["region"].(string); ok && r != "" {
		region = r
	}

	log.Debug("AWS User region extracted.", logKeyIdentity, i.name, "region", region)

	// Validate and set region in credentials.
	if longLivedCreds == nil {
		return nil, fmt.Errorf(errUtils.ErrStringWrappingFormat, errUtils.ErrAwsUserNotConfigured, "Have you ran `atmos auth user configure`?")
	}
	if longLivedCreds.Region == "" {
		longLivedCreds.Region = region
	}

	// Always generate session tokens (with or without MFA)
	return i.generateSessionToken(ctx, longLivedCreds, region)
}

// writeAWSFiles writes credentials to AWS config files using "aws-user" as mock provider.
func (i *userIdentity) writeAWSFiles(creds *types.AWSCredentials, region string) error {
	awsFileManager, err := awsCloud.NewAWSFileManager()
	if err != nil {
		return fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrAuthAwsFileManagerFailed, err)
	}

	// Write credentials to ~/.aws/atmos/aws-user/credentials
	if err := awsFileManager.WriteCredentials(awsUserProviderName, i.name, creds); err != nil {
		return fmt.Errorf("%w: failed to write AWS credentials: %v", errUtils.ErrAwsAuth, err)
	}

	// Write config to ~/.aws/atmos/aws-user/config
	if err := awsFileManager.WriteConfig(awsUserProviderName, i.name, region, ""); err != nil {
		return fmt.Errorf("%w: failed to write AWS config: %v", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// generateSessionToken generates session tokens for AWS User identities (with or without MFA).
func (i *userIdentity) generateSessionToken(ctx context.Context, longLivedCreds *types.AWSCredentials, region string) (types.ICredentials, error) {
	// Create AWS config with long-lived credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			longLivedCreds.AccessKeyID,
			longLivedCreds.SecretAccessKey,
			"", // no session token for long-lived credentials
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrAwsAuth, err)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	// Build GetSessionToken input (handles MFA prompt if configured)
	input, err := i.buildGetSessionTokenInput(longLivedCreds)
	if err != nil {
		return nil, err
	}

	result, err := stsClient.GetSessionToken(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get session token: %v", errUtils.ErrAuthenticationFailed, err)
	}

	// Validate result and safely construct session credentials.
	if result == nil || result.Credentials == nil {
		return nil, fmt.Errorf("%w: STS returned empty credentials", errUtils.ErrAuthenticationFailed)
	}

	accessKeyID := aws.ToString(result.Credentials.AccessKeyId)
	secretAccessKey := aws.ToString(result.Credentials.SecretAccessKey)
	sessionToken := aws.ToString(result.Credentials.SessionToken)
	expiration := ""
	if result.Credentials.Expiration != nil {
		expiration = result.Credentials.Expiration.Format(time.RFC3339)
	}

	// Create session credentials (temporary tokens for AWS files).
	sessionCreds := &types.AWSCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SessionToken:    sessionToken,
		Region:          region,
		Expiration:      expiration,
	}

	// Write session credentials to AWS files using "aws-user" as mock provider
	if err := i.writeAWSFiles(sessionCreds, region); err != nil {
		return nil, fmt.Errorf("%w: failed to write AWS files: %v", errUtils.ErrAwsAuth, err)
	}

	// Note: We keep the long-lived credentials in the keystore unchanged
	// Only the session tokens are written to AWS config/credentials files

	return sessionCreds, nil
}

// buildGetSessionTokenInput builds the STS GetSessionToken input, prompting for MFA if required.
func (i *userIdentity) buildGetSessionTokenInput(longLivedCreds *types.AWSCredentials) (*sts.GetSessionTokenInput, error) {
	if longLivedCreds.MfaArn != "" {
		var mfaToken string
		form := newMfaForm(longLivedCreds, &mfaToken)
		if err := form.Run(); err != nil {
			return nil, fmt.Errorf("%w: failed to get MFA token: %v", errUtils.ErrAuthenticationFailed, err)
		}
		return &sts.GetSessionTokenInput{
			SerialNumber:    aws.String(longLivedCreds.MfaArn),
			TokenCode:       aws.String(mfaToken),
			DurationSeconds: aws.Int32(defaultUserSessionSeconds),
		}, nil
	}
	return &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int32(defaultUserSessionSeconds),
	}, nil
}

func newMfaForm(longLivedCreds *types.AWSCredentials, mfaToken *string) *huh.Form {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter MFA Token").
				Description(fmt.Sprintf("MFA Device: %s", longLivedCreds.MfaArn)).
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
	awsFileManager, err := awsCloud.NewAWSFileManager()
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrappingFormat, errUtils.ErrAuthAwsFileManagerFailed, err)
	}
	awsEnvVars := awsFileManager.GetEnvironmentVariables(awsUserProviderName, i.name)

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
func AuthenticateStandaloneAWSUser(ctx context.Context, identityName string, identities map[string]types.Identity) (types.ICredentials, error) {
	log.Debug("Authenticating AWS user identity directly", logKeyIdentity, identityName)

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
func (i *userIdentity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	// Setup AWS files using shared AWS cloud package
	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %v", errUtils.ErrAwsAuth, err)
	}
	if err := awsCloud.SetEnvironmentVariables(stackInfo, providerName, identityName); err != nil {
		return fmt.Errorf("%w: failed to set environment variables: %v", errUtils.ErrAwsAuth, err)
	}
	return nil
}
