package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	log "github.com/charmbracelet/log"
	errUtils "github.com/cloudposse/atmos/errors"
	awsCloud "github.com/cloudposse/atmos/internal/auth/cloud/aws"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// assumeRoleIdentity implements AWS assume role identity.
type assumeRoleIdentity struct {
	name   string
	config *schema.Identity
	region string
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

	var roleArn string
	var okBool bool
	if roleArn, okBool = i.config.Principal["assume_role"].(string); !okBool || roleArn == "" {
		return nil, fmt.Errorf("%w: assume_role is required in principal", errUtils.ErrInvalidIdentityConfig)
	}

	// Create AWS config using base credentials
	i.region = awsBase.Region
	if i.region == "" {
		i.region = "us-east-1"
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(i.region))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrInvalidIdentityConfig, err)
	}

	// Inject base credentials into STS client
	cfg.Credentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(awsBase.AccessKeyID, awsBase.SecretAccessKey, awsBase.SessionToken))

	// Create STS client.
	stsClient := sts.NewFromConfig(cfg)

	// Assume the role.
	sessionName := fmt.Sprintf("atmos-%s-%d", i.name, time.Now().Unix())
	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String(sessionName),
	}

	// Add external ID if specified.
	if externalID, ok := i.config.Principal["external_id"].(string); ok && externalID != "" {
		assumeRoleInput.ExternalId = aws.String(externalID)
	}

	// Add duration if specified.
	var durationStr string
	durationStr, _ = i.config.Principal["duration"].(string)
	if durationStr != "" {
		if duration, err := time.ParseDuration(durationStr); err == nil {
			assumeRoleInput.DurationSeconds = aws.Int32(int32(duration.Seconds()))
		} else {
			log.Warn("Invalid duration specified for assume role", "duration", durationStr)
		}
	}

	result, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role: %v", errUtils.ErrAuthenticationFailed, err)
	}
	if result == nil || result.Credentials == nil {
		return nil, fmt.Errorf("%w: STS returned empty credentials", errUtils.ErrAuthenticationFailed)
	}
	// Convert to our credential format.
	var expiration string
	if result.Credentials != nil {
		expiration = result.Credentials.Expiration.Format(time.RFC3339)
	}

	return &types.AWSCredentials{
		AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(result.Credentials.SessionToken),
		Region:          i.region,
		Expiration:      expiration,
	}, nil
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
		// This assume role identity chains through another identity
		// For caching purposes, we'll use the chained identity name.
		return i.config.Via.Identity, nil
	}
	return "", fmt.Errorf("%w: assume role identity %q has no valid via configuration", errUtils.ErrInvalidIdentityConfig, i.name)
}

// PostAuthenticate sets up AWS files after authentication.
func (i *assumeRoleIdentity) PostAuthenticate(ctx context.Context, stackInfo *schema.ConfigAndStacksInfo, providerName, identityName string, creds types.ICredentials) error {
	// Setup AWS files using shared AWS cloud package.
	if err := awsCloud.SetupFiles(providerName, identityName, creds); err != nil {
		return fmt.Errorf("%w: failed to setup AWS files: %v", errUtils.ErrAwsAuth, err)
	}
	return nil
}
