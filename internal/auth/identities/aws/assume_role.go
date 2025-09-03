package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
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

// Authenticate performs authentication using assume role
func (i *assumeRoleIdentity) Authenticate(ctx context.Context, baseCreds *schema.Credentials) (*schema.Credentials, error) {
	if baseCreds == nil || baseCreds.AWS == nil {
		return nil, fmt.Errorf("base AWS credentials are required")
	}

	// Get role ARN from spec
	roleArn, ok := i.config.Spec["role_arn"].(string)
	if !ok || roleArn == "" {
		return nil, fmt.Errorf("role_arn is required in spec")
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
	if result.Credentials.Expiration != nil {
		expiration = result.Credentials.Expiration.Format(time.RFC3339)
	}

	return &schema.Credentials{
		AWS: &schema.AWSCredentials{
			AccessKeyID:     aws.ToString(result.Credentials.AccessKeyId),
			SecretAccessKey: aws.ToString(result.Credentials.SecretAccessKey),
			SessionToken:    aws.ToString(result.Credentials.SessionToken),
			Region:          baseCreds.AWS.Region,
			Expiration:      expiration,
		},
	}, nil
}

// Validate validates the identity configuration
func (i *assumeRoleIdentity) Validate() error {
	if i.config.Spec == nil {
		return fmt.Errorf("spec is required")
	}

	roleArn, ok := i.config.Spec["role_arn"].(string)
	if !ok || roleArn == "" {
		return fmt.Errorf("role_arn is required in spec")
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
