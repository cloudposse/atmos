package aws

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/charmbracelet/huh"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	atmosCredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	authUtils "github.com/cloudposse/atmos/pkg/auth/utils"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	defaultUserSessionSeconds = 43200  // 12 hours - max for IAM user without MFA, recommended default
	maxSessionSecondsNoMfa    = 43200  // 12 hours - AWS maximum for IAM user without MFA
	maxSessionSecondsWithMfa  = 129600 // 36 hours - AWS maximum for IAM user with MFA
	minSessionSeconds         = 900    // 15 minutes - AWS minimum
	awsUserProviderName       = "aws-user"
	logKeyIdentity            = "identity"
	logKeyErrorCode           = "error_code"
	defaultRegion             = "us-east-1"
)

// userIdentity implements AWS user identity (passthrough).
type userIdentity struct {
	name   string
	config *schema.Identity
}

// NewUserIdentity creates a new AWS user identity.
func NewUserIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if config == nil {
		return nil, fmt.Errorf("%w: identity %q has nil config", errUtils.ErrInvalidAuthConfig, name)
	}
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

// GetProviderName returns the provider name for this identity.
// AWS user identities always return "aws-user" as they are standalone.
func (i *userIdentity) GetProviderName() (string, error) {
	return awsUserProviderName, nil
}

// Authenticate performs authentication by checking for existing valid session credentials
// or generating new session tokens if needed.
func (i *userIdentity) Authenticate(ctx context.Context, _ types.ICredentials) (types.ICredentials, error) {
	// First, try to load existing session credentials from AWS files.
	// This prevents unnecessary GetSessionToken API calls when valid credentials already exist.
	existingCreds, err := i.LoadCredentials(ctx)
	if err == nil && existingCreds != nil {
		// Check if the loaded credentials are still valid (not expired).
		if !existingCreds.IsExpired() {
			log.Debug("Using existing valid session credentials from AWS files", logKeyIdentity, i.name)
			return existingCreds, nil
		}
		log.Debug("Existing session credentials are expired, generating new ones", logKeyIdentity, i.name)
	} else {
		log.Debug("No existing session credentials found, generating new ones", logKeyIdentity, i.name, "error", err)
	}

	// No valid existing credentials - resolve base credentials and generate new session tokens.
	longLivedCreds, err := i.resolveLongLivedCredentials()
	if err != nil {
		return nil, err
	}

	// Resolve region (from config or default) and ensure it is set.
	region := i.resolveRegion()
	log.Debug("AWS User region extracted.", logKeyIdentity, i.name, "region", region)
	if longLivedCreds.Region == "" {
		longLivedCreds.Region = region
	}

	// Generate a session token (handles MFA when configured).
	return i.generateSessionToken(ctx, longLivedCreds, region)
}

// resolveLongLivedCredentials returns long-lived credentials with deep merge precedence.
// Precedence order (per field): YAML config > Keyring store.
// This allows users to store credentials in keyring but override specific fields (e.g., MFA ARN) in YAML.
func (i *userIdentity) resolveLongLivedCredentials() (*types.AWSCredentials, error) {
	// Start with keyring credentials as base (if available).
	keystoreCreds, keystoreErr := i.credentialsFromStore()

	// Get YAML config values (may be empty or !env that resolved to empty).
	yamlAccessKeyID, _ := i.config.Credentials["access_key_id"].(string)
	yamlSecretAccessKey, _ := i.config.Credentials["secret_access_key"].(string)
	yamlMfaArn, _ := i.config.Credentials["mfa_arn"].(string)

	// If YAML has complete credentials (access key + secret), use YAML entirely.
	if yamlAccessKeyID != "" && yamlSecretAccessKey != "" {
		log.Debug("Using credentials from YAML config", logKeyIdentity, i.name, "hasAccessKey", true, "hasMFA", yamlMfaArn != "")
		return &types.AWSCredentials{
			AccessKeyID:     yamlAccessKeyID,
			SecretAccessKey: yamlSecretAccessKey,
			MfaArn:          yamlMfaArn,
		}, nil
	}

	// If YAML has partial credentials, that's an error.
	if yamlAccessKeyID != "" || yamlSecretAccessKey != "" {
		return nil, fmt.Errorf("%w: access_key_id and secret_access_key must both be provided or both be empty for identity %q", errUtils.ErrInvalidAuthConfig, i.name)
	}

	// YAML has no credentials, fall back to keyring.
	if keystoreErr != nil {
		// If credential prompting is available, prompt for new credentials.
		if PromptCredentialsFunc != nil {
			log.Debug("No credentials found, prompting for new credentials", logKeyIdentity, i.name)
			newCreds, promptErr := PromptCredentialsFunc(i.name, yamlMfaArn)
			if promptErr != nil {
				return nil, fmt.Errorf("%w: AWS User credentials not found for identity %q and prompting failed: %w", errUtils.ErrAwsUserNotConfigured, i.name, promptErr)
			}
			log.Debug("New credentials provided via prompt", logKeyIdentity, i.name)
			return newCreds, nil
		}
		return nil, fmt.Errorf("%w: AWS User credentials not found for identity %q. Please configure credentials by running: atmos auth user configure --identity %s", errUtils.ErrAwsUserNotConfigured, i.name, i.name)
	}

	// Deep merge: Start with keyring, override with non-empty YAML fields.
	result := &types.AWSCredentials{
		AccessKeyID:     keystoreCreds.AccessKeyID,
		SecretAccessKey: keystoreCreds.SecretAccessKey,
		MfaArn:          keystoreCreds.MfaArn,          // Start with keyring MFA ARN
		SessionDuration: keystoreCreds.SessionDuration, // Preserve session duration from keyring
	}

	// Override MFA ARN from YAML if present (allows version-controlled MFA config).
	if yamlMfaArn != "" {
		log.Debug("Overriding MFA ARN from YAML config", logKeyIdentity, i.name, "yaml_mfa_arn", yamlMfaArn, "keyring_mfa_arn", keystoreCreds.MfaArn)
		result.MfaArn = yamlMfaArn
	}

	log.Debug("Using credentials from keyring", logKeyIdentity, i.name, "mfa_source", map[bool]string{true: "yaml", false: "keyring"}[yamlMfaArn != ""])
	return result, nil
}

// credentialsFromConfig builds AWS credentials from identity config if present.
// Returns (nil, nil) when not configured.
func (i *userIdentity) credentialsFromConfig() (*types.AWSCredentials, error) {
	accessKeyID, hasAccessKey := i.config.Credentials["access_key_id"].(string)
	if !hasAccessKey || accessKeyID == "" {
		return nil, nil
	}

	secretAccessKey, _ := i.config.Credentials["secret_access_key"].(string)
	if secretAccessKey == "" {
		return nil, fmt.Errorf("%w: access_key_id is set but secret_access_key is missing for identity %q", errUtils.ErrInvalidAuthConfig, i.name)
	}

	mfaArn, _ := i.config.Credentials["mfa_arn"].(string)
	log.Debug("Using credentials from atmos.yaml configuration", logKeyIdentity, i.name, "hasAccessKey", accessKeyID != "", "hasMFA", mfaArn != "")

	return &types.AWSCredentials{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		MfaArn:          mfaArn,
	}, nil
}

// credentialsFromStore retrieves AWS credentials from the keyring store.
func (i *userIdentity) credentialsFromStore() (*types.AWSCredentials, error) {
	credStore := atmosCredentials.NewCredentialStore()
	retrieved, err := credStore.Retrieve(i.name)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to retrieve AWS User credentials for %q: %w", errUtils.ErrAwsUserNotConfigured, i.name, err)
	}

	longLivedCreds, ok := retrieved.(*types.AWSCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: stored credentials are not AWS credentials", errUtils.ErrAwsAuth)
	}
	if longLivedCreds.AccessKeyID == "" || longLivedCreds.SecretAccessKey == "" {
		return nil, fmt.Errorf("%w: stored AWS user credentials for %q are incomplete (missing access key or secret)", errUtils.ErrAwsUserNotConfigured, i.name)
	}

	log.Debug("Using credentials from keyring", logKeyIdentity, i.name)
	return longLivedCreds, nil
}

// resolveRegion returns the configured region or the default one.
func (i *userIdentity) resolveRegion() string {
	if r, ok := i.config.Credentials["region"].(string); ok && r != "" {
		return r
	}
	return defaultRegion
}

// writeAWSFiles writes credentials to AWS config files using "aws-user" as mock provider.
func (i *userIdentity) writeAWSFiles(creds *types.AWSCredentials, region string) error {
	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}

	// Write credentials to XDG config directory (e.g., ~/.config/atmos/aws/aws-user/credentials on Linux).
	if err := awsFileManager.WriteCredentials(awsUserProviderName, i.name, creds); err != nil {
		return fmt.Errorf("%w: failed to write AWS credentials: %w", errUtils.ErrAwsAuth, err)
	}

	// Write config to XDG config directory (e.g., ~/.config/atmos/aws/aws-user/config on Linux).
	if err := awsFileManager.WriteConfig(awsUserProviderName, i.name, region, ""); err != nil {
		return fmt.Errorf("%w: failed to write AWS config: %w", errUtils.ErrAwsAuth, err)
	}

	return nil
}

// generateSessionToken generates session tokens for AWS User identities (with or without MFA).
func (i *userIdentity) generateSessionToken(ctx context.Context, longLivedCreds *types.AWSCredentials, region string) (types.ICredentials, error) {
	return i.generateSessionTokenWithRetry(ctx, longLivedCreds, region, false)
}

// generateSessionTokenWithRetry is the internal implementation that supports retrying with new credentials.
func (i *userIdentity) generateSessionTokenWithRetry(ctx context.Context, longLivedCreds *types.AWSCredentials, region string, isRetry bool) (types.ICredentials, error) {
	// Call STS to get session token.
	result, err := i.callGetSessionToken(ctx, longLivedCreds, region)
	if err != nil {
		return i.handleSTSErrorWithRetry(ctx, err, region, isRetry)
	}

	// Convert STS result to session credentials and write to AWS files.
	return i.processSTSResult(result, region)
}

// callGetSessionToken makes the STS GetSessionToken API call.
func (i *userIdentity) callGetSessionToken(ctx context.Context, longLivedCreds *types.AWSCredentials, region string) (*sts.GetSessionTokenOutput, error) {
	// Build config options.
	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			longLivedCreds.AccessKeyID, longLivedCreds.SecretAccessKey, "",
		)),
	}
	if resolverOpt := awsCloud.GetResolverConfigOption(i.config, nil); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	cfg, err := awsCloud.LoadIsolatedAWSConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %w", errUtils.ErrAwsAuth, err)
	}

	input, err := i.buildGetSessionTokenInput(longLivedCreds)
	if err != nil {
		return nil, err
	}

	return sts.NewFromConfig(cfg).GetSessionToken(ctx, input)
}

// handleSTSErrorWithRetry handles STS errors and optionally retries with new credentials.
func (i *userIdentity) handleSTSErrorWithRetry(ctx context.Context, err error, region string, isRetry bool) (types.ICredentials, error) {
	newCreds, stsErr := i.handleSTSError(err, isRetry)
	if stsErr != nil {
		return nil, stsErr
	}
	if newCreds != nil {
		log.Debug("Retrying STS call with new credentials", logKeyIdentity, i.name)
		if newCreds.Region == "" {
			newCreds.Region = region
		}
		return i.generateSessionTokenWithRetry(ctx, newCreds, region, true)
	}
	return nil, fmt.Errorf("%w: unexpected state in credential retry", errUtils.ErrAuthenticationFailed)
}

// processSTSResult converts STS result to credentials and writes to AWS files.
func (i *userIdentity) processSTSResult(result *sts.GetSessionTokenOutput, region string) (*types.AWSCredentials, error) {
	if result == nil || result.Credentials == nil {
		return nil, fmt.Errorf("%w: STS returned empty credentials", errUtils.ErrAuthenticationFailed)
	}

	expiration := ""
	if result.Credentials.Expiration != nil {
		expiration = result.Credentials.Expiration.Format(time.RFC3339)
	}

	sessionCreds := &types.AWSCredentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
		Region:          region,
		Expiration:      expiration,
	}

	if err := i.writeAWSFiles(sessionCreds, region); err != nil {
		return nil, fmt.Errorf("%w: failed to write AWS files: %w", errUtils.ErrAwsAuth, err)
	}

	return sessionCreds, nil
}

// handleSTSError processes errors from STS API calls and returns appropriate user-friendly errors.
// It detects specific AWS error codes like InvalidClientTokenId and provides actionable guidance.
// The isRetry parameter indicates if this is a retry attempt after credential prompting.
// Returns: (new credentials if prompting succeeded, error if prompting failed/disabled or on retry).
func (i *userIdentity) handleSTSError(err error, isRetry bool) (*types.AWSCredentials, error) {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return nil, errors.Join(errUtils.ErrAuthenticationFailed, err)
	}

	switch apiErr.ErrorCode() {
	case "InvalidClientTokenId":
		return i.handleInvalidClientTokenId(apiErr, isRetry)
	case "ExpiredTokenException":
		return i.handleExpiredToken(apiErr)
	case "AccessDenied":
		return i.handleAccessDenied(apiErr)
	default:
		return nil, errors.Join(errUtils.ErrAuthenticationFailed, err)
	}
}

// handleInvalidClientTokenId handles the case where AWS access keys have been rotated or revoked.
// It clears stale credentials and optionally prompts for new ones (only on first attempt).
// The isRetry parameter prevents duplicate prompting when the newly-entered credentials also fail.
func (i *userIdentity) handleInvalidClientTokenId(apiErr smithy.APIError, isRetry bool) (*types.AWSCredentials, error) {
	log.Error("AWS credentials are invalid or have been revoked",
		logKeyIdentity, i.name, logKeyErrorCode, apiErr.ErrorCode(), "suggestion", "reconfigure credentials")

	i.clearStaleCredentials()

	// Only prompt for credentials on the first attempt, not on retry.
	// If this is a retry, the user already provided credentials that also failed.
	if PromptCredentialsFunc != nil && !isRetry {
		return i.promptForNewCredentials(apiErr.ErrorCode())
	}

	// Build appropriate error message based on whether this is a retry.
	builder := errUtils.Build(errUtils.ErrCredentialsInvalid).
		WithContext("identity", i.name).
		WithContext(logKeyErrorCode, apiErr.ErrorCode())

	if isRetry {
		builder = builder.
			WithExplanation("The newly-entered AWS credentials are also invalid").
			WithHint("Please verify your access key ID and secret access key are correct").
			WithHintf("Run: atmos auth user configure --identity %s", i.name)
	} else {
		builder = builder.
			WithExplanation("Your AWS access keys have been rotated or revoked on the AWS side").
			WithExplanation("Stale credentials have been automatically cleared from keychain").
			WithHintf("Run: atmos auth user configure --identity %s", i.name)
	}

	return nil, builder.Err()
}

// clearStaleCredentials removes stale credentials from the keyring.
func (i *userIdentity) clearStaleCredentials() {
	credStore := atmosCredentials.NewCredentialStore()
	if delErr := credStore.Delete(i.name); delErr != nil {
		log.Debug("Failed to clear stale credentials from keyring", logKeyIdentity, i.name, "error", delErr)
	} else {
		log.Debug("Cleared stale credentials from keyring", logKeyIdentity, i.name)
	}
}

// promptForNewCredentials prompts the user for new credentials and returns them.
func (i *userIdentity) promptForNewCredentials(errorCode string) (*types.AWSCredentials, error) {
	yamlMfaArn, _ := i.config.Credentials["mfa_arn"].(string)
	newCreds, promptErr := PromptCredentialsFunc(i.name, yamlMfaArn)
	if promptErr != nil {
		log.Debug("Credential prompting failed", logKeyIdentity, i.name, "error", promptErr)
		return nil, errUtils.Build(errUtils.ErrCredentialsInvalid).
			WithExplanation("Your AWS access keys have been rotated or revoked on the AWS side").
			WithExplanation("Credential prompting was cancelled or failed").
			WithHintf("Run: atmos auth user configure --identity %s", i.name).
			WithContext("identity", i.name).
			WithContext(logKeyErrorCode, errorCode).
			Err()
	}
	log.Debug("New credentials provided via prompt", logKeyIdentity, i.name)
	return newCreds, nil
}

// handleExpiredToken handles the case where the session token has expired.
func (i *userIdentity) handleExpiredToken(apiErr smithy.APIError) (*types.AWSCredentials, error) {
	log.Error("AWS session token expired", logKeyIdentity, i.name, logKeyErrorCode, apiErr.ErrorCode())
	return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
		WithExplanation("AWS session token has expired").
		WithHintf("Run: atmos auth login --identity %s", i.name).
		WithContext("identity", i.name).
		WithContext(logKeyErrorCode, apiErr.ErrorCode()).
		Err()
}

// handleAccessDenied handles the case where the user doesn't have permission to call GetSessionToken.
func (i *userIdentity) handleAccessDenied(apiErr smithy.APIError) (*types.AWSCredentials, error) {
	log.Error("Access denied when calling GetSessionToken", logKeyIdentity, i.name, logKeyErrorCode, apiErr.ErrorCode())
	return nil, errUtils.Build(errUtils.ErrAuthenticationFailed).
		WithExplanation("Your IAM user does not have permission to call sts:GetSessionToken").
		WithHint("Ensure your IAM user has the sts:GetSessionToken permission").
		WithContext("identity", i.name).
		WithContext(logKeyErrorCode, apiErr.ErrorCode()).
		Err()
}

// PromptMfaTokenFunc is a helper indirection to allow tests to stub MFA prompting.
// In production, it displays a form to collect the token.
var promptMfaTokenFunc = func(longLivedCreds *types.AWSCredentials) (string, error) {
	var mfaToken string
	form := newMfaForm(longLivedCreds, &mfaToken)
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("%w: failed to get MFA token: %w", errUtils.ErrAuthenticationFailed, err)
	}
	return mfaToken, nil
}

// getSessionDuration returns the configured session duration in seconds.
// It validates the duration against AWS limits based on whether MFA is used.
// Checks identity session config first, then keyring credentials.
// Supports multiple duration formats: integers (seconds), Go duration ("1h30m"), and days ("2d").
func (i *userIdentity) getSessionDuration(hasMfa bool, credentialsDuration string) int32 {
	// Default duration.
	duration := int32(defaultUserSessionSeconds)
	var durationSource string

	// Priority 1: Check identity session config (YAML).
	if i.config.Session != nil && i.config.Session.Duration != "" {
		parsedSeconds, err := authUtils.ParseDurationFlexible(i.config.Session.Duration)
		if err != nil {
			log.Warn("Invalid session duration format in YAML config, using default",
				logKeyIdentity, i.name,
				"configured", i.config.Session.Duration,
				"default", fmt.Sprintf("%ds", defaultUserSessionSeconds),
				"error", err)
		} else if parsedSeconds <= 0 || parsedSeconds > math.MaxInt32 {
			log.Warn("Session duration out of valid range in YAML config, using default",
				logKeyIdentity, i.name,
				"configured", parsedSeconds,
				"default", fmt.Sprintf("%ds", defaultUserSessionSeconds))
		} else {
			duration = int32(parsedSeconds)
			durationSource = "YAML config"
		}
	} else if credentialsDuration != "" {
		// Priority 2: Check keyring credentials.
		parsedSeconds, err := authUtils.ParseDurationFlexible(credentialsDuration)
		if err != nil {
			log.Warn("Invalid session duration format in keyring, using default",
				logKeyIdentity, i.name,
				"configured", credentialsDuration,
				"default", fmt.Sprintf("%ds", defaultUserSessionSeconds),
				"error", err)
		} else if parsedSeconds <= 0 || parsedSeconds > math.MaxInt32 {
			log.Warn("Session duration out of valid range in keyring, using default",
				logKeyIdentity, i.name,
				"configured", parsedSeconds,
				"default", fmt.Sprintf("%ds", defaultUserSessionSeconds))
		} else {
			duration = int32(parsedSeconds)
			durationSource = "keyring"
		}
	} else {
		durationSource = "default"
	}

	// Validate and clamp duration to AWS limits.
	maxDuration := int32(maxSessionSecondsNoMfa)
	if hasMfa {
		maxDuration = int32(maxSessionSecondsWithMfa)
	}

	if duration < int32(minSessionSeconds) {
		log.Warn("Session duration below AWS minimum, clamping to minimum",
			logKeyIdentity, i.name,
			"requested", duration,
			"minimum", minSessionSeconds,
			"source", durationSource)
		duration = int32(minSessionSeconds)
	} else if duration > maxDuration {
		mfaStatus := "without MFA"
		if hasMfa {
			mfaStatus = "with MFA"
		}
		log.Warn("Session duration exceeds AWS maximum, clamping to maximum",
			logKeyIdentity, i.name,
			"requested", duration,
			"maximum", maxDuration,
			"mfa", mfaStatus,
			"source", durationSource)
		duration = maxDuration
	}

	return duration
}

func (i *userIdentity) buildGetSessionTokenInput(longLivedCreds *types.AWSCredentials) (*sts.GetSessionTokenInput, error) {
	// Get configured session duration or use default.
	// Pass the session duration from credentials (if stored in keyring).
	durationSeconds := i.getSessionDuration(longLivedCreds.MfaArn != "", longLivedCreds.SessionDuration)

	if longLivedCreds.MfaArn != "" {
		token, err := promptMfaTokenFunc(longLivedCreds)
		if err != nil {
			return nil, err
		}
		return &sts.GetSessionTokenInput{
			SerialNumber:    aws.String(longLivedCreds.MfaArn),
			TokenCode:       aws.String(token),
			DurationSeconds: aws.Int32(durationSeconds),
		}, nil
	}
	return &sts.GetSessionTokenInput{
		DurationSeconds: aws.Int32(durationSeconds),
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
	// User identities don't require additional validation beyond the provider.
	return nil
}

// Environment returns environment variables for this identity.
func (i *userIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Get AWS file environment variables using "aws-user" as mock provider.
	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return nil, errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}
	awsEnvVars := awsFileManager.GetEnvironmentVariables(awsUserProviderName, i.name)

	// Convert to map format.
	for _, envVar := range awsEnvVars {
		env[envVar.Key] = envVar.Value
	}

	// Add environment variables from identity config.
	for _, envVar := range i.config.Env {
		env[envVar.Key] = envVar.Value
	}

	return env, nil
}

// Paths returns credential files/directories used by this identity.
func (i *userIdentity) Paths() ([]types.Path, error) {
	// AWS user identities don't add additional credential files beyond the provider.
	return []types.Path{}, nil
}

// PrepareEnvironment prepares environment variables for external processes.
// For AWS user identities, we use the shared AWS PrepareEnvironment helper
// which configures credential files, profile, region, and disables IMDS fallback.
func (i *userIdentity) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error) {
	defer perf.Track(nil, "aws.userIdentity.PrepareEnvironment")()

	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return environ, fmt.Errorf("failed to create AWS file manager: %w", err)
	}

	// AWS user identities always use "aws-user" as their provider name.
	credentialsFile := awsFileManager.GetCredentialsPath(awsUserProviderName)
	configFile := awsFileManager.GetConfigPath(awsUserProviderName)

	// Get region from identity config if available.
	region := i.resolveRegion()

	// Use shared AWS environment preparation helper.
	return awsCloud.PrepareEnvironment(environ, i.name, credentialsFile, configFile, region), nil
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

	// Get the identity instance.
	userIdentity, exists := identities[identityName]
	if !exists {
		return nil, fmt.Errorf("%w: AWS user identity %q not found", errUtils.ErrInvalidAuthConfig, identityName)
	}

	// AWS user identities authenticate directly without provider credentials.
	credentials, err := userIdentity.Authenticate(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: AWS user identity %q authentication failed: %w", errUtils.ErrAuthenticationFailed, identityName, err)
	}

	log.Debug("AWS user identity authenticated successfully", "identity", identityName)
	return credentials, nil
}

// PostAuthenticate sets up AWS files and populates auth context after authentication.
func (i *userIdentity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	// Guard against nil parameters to avoid panics.
	if params == nil {
		return fmt.Errorf("%w: PostAuthenticate parameters cannot be nil", errUtils.ErrInvalidAuthConfig)
	}
	if params.Credentials == nil {
		return fmt.Errorf("%w: credentials are required", errUtils.ErrInvalidAuthConfig)
	}

	// Enforce fixed provider for aws/user identities to avoid path drift.
	// User identities always use "aws-user" provider name regardless of caller input.
	providerName := awsUserProviderName
	identityName := i.name

	// Setup AWS files using shared AWS cloud package.
	if err := awsCloud.SetupFiles(providerName, identityName, params.Credentials, ""); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	// Populate auth context (single source of truth for runtime credentials).
	if err := awsCloud.SetAuthContext(&awsCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: providerName,
		IdentityName: identityName,
		Credentials:  params.Credentials,
		BasePath:     "",
	}); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	// Derive environment variables from auth context for spawned processes.
	if err := awsCloud.SetEnvironmentVariables(params.AuthContext, params.StackInfo); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	return nil
}

// CredentialsExist checks if credentials exist for this identity.
func (i *userIdentity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "aws.userIdentity.CredentialsExist")()

	mgr, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return false, err
	}

	credPath := mgr.GetCredentialsPath(awsUserProviderName)

	// Load and parse the credentials file to verify the identity section exists.
	cfg, err := awsCloud.LoadINIFile(credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("load credentials file: %w", err)
	}

	// Check if this identity's section exists in the credentials file.
	sec, err := cfg.GetSection(i.name)
	if err != nil {
		// Section doesn't exist - credentials don't exist for this identity.
		return false, nil
	}

	// Verify the section has actual credential keys (not just an empty section).
	if strings.TrimSpace(sec.Key("aws_access_key_id").String()) == "" {
		return false, nil
	}

	return true, nil
}

// LoadCredentials loads AWS credentials from files using environment variables.
// This is used with noop keyring to enable credential validation in whoami.
func (i *userIdentity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.userIdentity.LoadCredentials")()

	// Get environment variables that specify where credentials are stored.
	env, err := i.Environment()
	if err != nil {
		return nil, fmt.Errorf("failed to get environment variables: %w", err)
	}

	// Load credentials from files using AWS SDK.
	creds, err := loadAWSCredentialsFromEnvironment(ctx, env)
	if err != nil {
		return nil, err
	}

	return creds, nil
}

// Logout removes identity-specific credential storage.
func (i *userIdentity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.userIdentity.Logout")()

	// AWS user identities use "aws-user" as their provider name.
	// Clean up files under XDG config directory (e.g., ~/.config/atmos/aws/aws-user/ on Linux).
	fileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	// Use DeleteIdentity to remove only this identity's sections from shared INI files.
	// This preserves credentials for other identities using the same provider.
	if err := fileManager.DeleteIdentity(ctx, "aws-user", i.name); err != nil {
		log.Debug("Failed to delete AWS files for user identity", "identity", i.name, "error", err)
		return errors.Join(errUtils.ErrLogoutFailed, err)
	}

	log.Debug("Deleted AWS files for user identity", "identity", i.name)
	return nil
}
