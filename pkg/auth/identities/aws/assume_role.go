package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
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
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
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
		return nil, fmt.Errorf("%w: failed to assume role: %w", errUtils.ErrAuthenticationFailed, err)
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

// PostAuthenticate sets up AWS files after authentication.
func (i *assumeRoleIdentity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	// Setup AWS files using shared AWS cloud package.
	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %w", errUtils.ErrAwsAuth, err)
	}
	if err := awsCloud.SetEnvironmentVariables(stackInfo, providerName, identityName); err != nil {
		return fmt.Errorf("%w: failed to set environment variables: %w", errUtils.ErrAwsAuth, err)
	}
	return nil
}

// sanitizeRoleSessionName sanitizes the role session name to be used in AssumeRole.
func sanitizeRoleSessionName(s string) string {
	// Allowed: letters, digits, + = , . @ -  characters.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '+' || r == '=' || r == ',' || r == '.' || r == '@' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := b.String()
	if len(name) > maxSessionNameLength {
		name = name[:maxSessionNameLength]
	}
	return name
}
