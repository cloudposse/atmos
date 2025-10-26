package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const maxSessionNameLength = 64

// assumeRoleIdentity implements AWS assume role identity.
type assumeRoleIdentity struct {
	name    string
	config  *schema.Identity
	region  string
	roleArn string
}

// newSTSClient creates an STS client using the base credentials and configured region.
func (i *assumeRoleIdentity) newSTSClient(ctx context.Context, awsBase *types.AWSCredentials) (*sts.Client, error) {
	region := i.region
	if region == "" {
		region = awsBase.Region
	}
	if region == "" {
		region = "us-east-1"
	}
	// Persist the resolved region back onto the identity so it is available for serialization.
	i.region = region

	// Build config options
	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	// Add custom endpoint resolver if configured
	if resolverOpt := awsCloud.GetResolverConfigOption(i.config, nil); resolverOpt != nil {
		configOpts = append(configOpts, resolverOpt)
	}

	// Load config with isolated environment to avoid conflicts with external AWS env vars.
	cfg, err := awsCloud.LoadIsolatedAWSConfig(ctx, configOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %w", errUtils.ErrInvalidIdentityConfig, err)
	}
	cfg.Credentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(awsBase.AccessKeyID, awsBase.SecretAccessKey, awsBase.SessionToken))
	return sts.NewFromConfig(cfg), nil
}

// toAWSCredentials converts STS AssumeRole output to AWSCredentials with validation.
func (i *assumeRoleIdentity) toAWSCredentials(result *sts.AssumeRoleOutput) (types.ICredentials, error) {
	if result == nil || result.Credentials == nil {
		return nil, fmt.Errorf("%w: STS returned empty credentials", errUtils.ErrAuthenticationFailed)
	}
	expiration := ""
	if result.Credentials != nil && result.Credentials.Expiration != nil {
		expiration = result.Credentials.Expiration.Format(time.RFC3339)
	}
	// Ensure a non-empty region is serialized.
	finalRegion := i.region
	if finalRegion == "" {
		finalRegion = "us-east-1"
	}
	return &types.AWSCredentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
		Region:          finalRegion,
		Expiration:      expiration,
	}, nil
}

// buildAssumeRoleInput constructs the STS AssumeRoleInput including optional external ID and duration.

func (i *assumeRoleIdentity) buildAssumeRoleInput() *sts.AssumeRoleInput {
	raw := fmt.Sprintf("atmos-%s-%d", i.name, time.Now().Unix())
	sessionName := sanitizeRoleSessionName(raw)
	input := &sts.AssumeRoleInput{
		RoleArn:         aws.String(i.roleArn),
		RoleSessionName: aws.String(sessionName),
	}
	if externalID, ok := i.config.Principal["external_id"].(string); ok && externalID != "" {
		input.ExternalId = aws.String(externalID)
	}
	if durationStr, ok := i.config.Principal["duration"].(string); ok && durationStr != "" {
		if duration, err := time.ParseDuration(durationStr); err == nil {
			input.DurationSeconds = aws.Int32(int32(duration.Seconds()))
		} else {
			log.Warn("Invalid duration specified for assume role", "duration", durationStr)
		}
	}
	return input
}

// NewAssumeRoleIdentity creates a new AWS assume role identity.
func NewAssumeRoleIdentity(name string, config *schema.Identity) (types.Identity, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: identity name is empty", errUtils.ErrInvalidIdentityConfig)
	}
	if config == nil {
		return nil, fmt.Errorf("%w: identity config is nil", errUtils.ErrInvalidIdentityConfig)
	}
	if config.Kind != "aws/assume-role" {
		return nil, fmt.Errorf("%w: invalid identity kind for assume role: %s", errUtils.ErrInvalidIdentityKind, config.Kind)
	}

	return &assumeRoleIdentity{
		name:   name,
		config: config,
	}, nil
}

// Kind returns the identity kind.
func (i *assumeRoleIdentity) Kind() string {
	return "aws/assume-role"
}

// Authenticate performs authentication using assume role.
func (i *assumeRoleIdentity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
	// Note: Caching is now handled at the manager level to prevent duplicates.

	awsBase, ok := baseCreds.(*types.AWSCredentials)
	if !ok {
		return nil, fmt.Errorf("%w: base AWS credentials are required for assume-role", errUtils.ErrInvalidIdentityConfig)
	}

	// Validate identity configuration, sets roleArn and region.
	if err := i.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid assume role identity: %w", errUtils.ErrInvalidIdentityConfig, err)
	}

	// Create STS client with base credentials.
	stsClient, err := i.newSTSClient(ctx, awsBase)
	if err != nil {
		return nil, err
	}

	// Build AssumeRole input (handles optional external ID and duration).
	assumeRoleInput := i.buildAssumeRoleInput()

	result, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return nil, errors.Join(errUtils.ErrAuthenticationFailed, err)
	}
	return i.toAWSCredentials(result)
}

// Validate validates the identity configuration.
func (i *assumeRoleIdentity) Validate() error {
	if i.config.Principal == nil {
		return fmt.Errorf("%w: principal is required", errUtils.ErrInvalidIdentityConfig)
	}

	// Check role ARN in principal or spec (backward compatibility).
	var roleArn string
	var ok bool
	if roleArn, ok = i.config.Principal["assume_role"].(string); !ok || roleArn == "" {
		return fmt.Errorf("%w: assume_role is required in principal", errUtils.ErrInvalidIdentityConfig)
	}
	i.roleArn = roleArn

	var region string
	if region, ok = i.config.Principal["region"].(string); ok {
		i.region = region
	}

	return nil
}

// Environment returns environment variables for this identity.
func (i *assumeRoleIdentity) Environment() (map[string]string, error) {
	env := make(map[string]string)

	// Get provider name for AWS file paths.
	providerName, err := i.GetProviderName()
	if err != nil {
		return nil, err
	}

	// Get AWS file environment variables.
	awsFileManager, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return nil, errors.Join(errUtils.ErrAuthAwsFileManagerFailed, err)
	}
	awsEnvVars := awsFileManager.GetEnvironmentVariables(providerName, i.name)

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

// GetProviderName extracts the provider name from the identity configuration.
func (i *assumeRoleIdentity) GetProviderName() (string, error) {
	if i.config.Via != nil && i.config.Via.Provider != "" {
		return i.config.Via.Provider, nil
	}
	if i.config.Via != nil && i.config.Via.Identity != "" {
		// This assume role identity chains through another identity.
		// For caching purposes, we'll use the chained identity name.
		return i.config.Via.Identity, nil
	}
	return "", fmt.Errorf("%w: assume role identity %q has no valid via configuration", errUtils.ErrInvalidIdentityConfig, i.name)
}

// PostAuthenticate sets up AWS files and populates auth context after authentication.
func (i *assumeRoleIdentity) PostAuthenticate(ctx context.Context, params *types.PostAuthenticateParams) error {
	// Guard against nil parameters to avoid panics.
	if params == nil {
		return fmt.Errorf("%w: PostAuthenticate parameters cannot be nil", errUtils.ErrInvalidAuthConfig)
	}
	if params.Credentials == nil {
		return fmt.Errorf("%w: credentials are required", errUtils.ErrInvalidAuthConfig)
	}

	// Setup AWS files using shared AWS cloud package.
	if err := awsCloud.SetupFiles(params.ProviderName, params.IdentityName, params.Credentials, ""); err != nil {
		return errors.Join(errUtils.ErrAwsAuth, err)
	}

	// Populate auth context (single source of truth for runtime credentials).
	if err := awsCloud.SetAuthContext(&awsCloud.SetAuthContextParams{
		AuthContext:  params.AuthContext,
		StackInfo:    params.StackInfo,
		ProviderName: params.ProviderName,
		IdentityName: params.IdentityName,
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

// sanitizeRoleSessionName sanitizes the role session name to be used in AssumeRole.
// Allowed characters are ASCII letters, digits, and "+=,.@-".
// Anything else is replaced with '-'.
func sanitizeRoleSessionName(s string) string {
	// Allowed: letters, digits, + = , . @ -  characters.
	var b strings.Builder
	for _, r := range s {
		if isAllowed(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := b.String()
	return sanitizeRoleSessionNameLengthAndTrim(name)
}

func isAtoZ(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isAllowed(r rune) bool {
	return isAtoZ(r) || isDigit(r) || r == '+' || r == '=' || r == ',' || r == '.' || r == '@' || r == '-'
}

func sanitizeRoleSessionNameLengthAndTrim(name string) string {
	if len(name) > maxSessionNameLength {
		name = name[:maxSessionNameLength]
	}
	// Remove trailing dashes to ensure valid session name.
	name = strings.TrimRight(name, "-")
	if name == "" {
		name = "atmos-session"
	}
	return name
}

// CredentialsExist checks if credentials exist for this identity.
func (i *assumeRoleIdentity) CredentialsExist() (bool, error) {
	defer perf.Track(nil, "aws.assumeRoleIdentity.CredentialsExist")()

	providerName, err := i.GetProviderName()
	if err != nil {
		return false, err
	}

	mgr, err := awsCloud.NewAWSFileManager("")
	if err != nil {
		return false, err
	}

	credPath := mgr.GetCredentialsPath(providerName)

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
func (i *assumeRoleIdentity) LoadCredentials(ctx context.Context) (types.ICredentials, error) {
	defer perf.Track(nil, "aws.assumeRoleIdentity.LoadCredentials")()

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
func (i *assumeRoleIdentity) Logout(ctx context.Context) error {
	defer perf.Track(nil, "aws.assumeRoleIdentity.Logout")()

	// AWS assume-role identities don't have identity-specific storage.
	// File cleanup is handled by the provider's Logout method.
	// Keyring cleanup is handled by AuthManager.
	log.Debug("Logout called for assume-role identity (no identity-specific cleanup)", "identity", i.name)
	return nil
}
