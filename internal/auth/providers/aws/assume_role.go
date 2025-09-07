package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// assumeRoleProvider implements AWS assume role authentication.
type assumeRoleProvider struct {
	name   string
	config *schema.Provider
	region string
}

// NewAssumeRoleProvider creates a new AWS assume role provider.
func NewAssumeRoleProvider(name string, providerConfig *schema.Provider) (*assumeRoleProvider, error) {
	if providerConfig.Kind != "aws/assume-role" {
		return nil, fmt.Errorf("%w: invalid provider kind for assume role provider: %s", errUtils.ErrInvalidProviderKind, providerConfig.Kind)
	}

	region := providerConfig.Region
	if region == "" {
		region = "us-east-1" // Default region
	}

	return &assumeRoleProvider{
		name:   name,
		config: providerConfig,
		region: region,
	}, nil
}

// Kind returns the provider kind.
func (p *assumeRoleProvider) Kind() string {
	return "aws/assume-role"
}

// Name returns the configured provider name.
func (p *assumeRoleProvider) Name() string {
	return p.name
}

// PreAuthenticate is a no-op for assume-role provider.
func (p *assumeRoleProvider) PreAuthenticate(_ types.AuthManager) error {
	return nil
}

// Authenticate performs AWS assume role authentication.
func (p *assumeRoleProvider) Authenticate(ctx context.Context) (*schema.Credentials, error) {
	// Load default AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.region))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to load AWS config: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	// Get role ARN from spec
	roleArn, ok := p.config.Spec["role_arn"].(string)
	if !ok || roleArn == "" {
		return nil, fmt.Errorf("%w: role_arn is required in provider spec", errUtils.ErrInvalidProviderConfig)
	}

	// Create STS client
	stsClient := sts.NewFromConfig(cfg)

	// Assume the role
	sessionName := fmt.Sprintf("atmos-%s-%d", p.name, time.Now().Unix())
	assumeRoleInput := &sts.AssumeRoleInput{
		RoleArn:         aws.String(roleArn),
		RoleSessionName: aws.String(sessionName),
	}

	// Add duration if specified
	if p.config.Session != nil && p.config.Session.Duration != "" {
		if duration, err := time.ParseDuration(p.config.Session.Duration); err == nil {
			assumeRoleInput.DurationSeconds = aws.Int32(int32(duration.Seconds()))
		}
	}

	result, err := stsClient.AssumeRole(ctx, assumeRoleInput)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to assume role: %v", errUtils.ErrInvalidProviderConfig, err)
	}

	// Convert to our credential format
	expiration := ""
	if result.Credentials.Expiration != nil {
		expiration = result.Credentials.Expiration.Format(time.RFC3339)
	}

	return &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
			SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
			SessionToken:    aws.ToString(result.Credentials.SessionToken),
			Region:          p.region,
			Expiration:      expiration,
		},
	}, nil
}

// Validate validates the provider configuration.
func (p *assumeRoleProvider) Validate() error {
	if p.config.Spec == nil {
		return fmt.Errorf("%w: spec is required", errUtils.ErrInvalidProviderConfig)
	}

	roleArn, ok := p.config.Spec["role_arn"].(string)
	if !ok || roleArn == "" {
		return fmt.Errorf("%w: role_arn is required in spec", errUtils.ErrInvalidProviderConfig)
	}

	return nil
}

// Environment returns environment variables for this provider.
func (p *assumeRoleProvider) Environment() (map[string]string, error) {
	env := make(map[string]string)
	env["AWS_REGION"] = p.region
	return env, nil
}
