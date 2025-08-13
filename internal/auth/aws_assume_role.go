package auth

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"os"
	"time"
)

type awsAssumeRole struct {
	Common   schema.IdentityProviderDefaultConfig `yaml:",inline"`
	Identity schema.Identity                      `yaml:",inline"`

	RoleArn         string `yaml:"role_arn,omitempty" json:"role_arn,omitempty" mapstructure:"role_arn,omitempty"`
	SessionDuration int32  `yaml:"session_duration,omitempty" json:"session_duration,omitempty" mapstructure:"session_duration,omitempty"`
}

// Validate checks if the required fields are set
func (i *awsAssumeRole) Validate() error {
	if i.RoleArn == "" {
		return fmt.Errorf("role_arn is required for AWS assume role")
	}

	if i.Identity.Profile == "" {
		return fmt.Errorf("profile is required for AWS assume role")
	}

	// Set default region if not specified
	if i.Common.Region == "" {
		i.Common.Region = "us-east-1" // Default region
	}

	return nil
}

func (config *awsAssumeRole) getProfile() string {
	return config.Identity.Profile
}
func (config *awsAssumeRole) getRegion() string {
	return config.Common.Region
}

// Login verifies AWS credentials are available in the default profile
func (i *awsAssumeRole) Login() error {
	log.Debug("Validating AWS credentials")

	// Set up session duration if not specified
	if i.SessionDuration == 0 {
		i.SessionDuration = 3600 // Default to 1 hour
	}

	// Verify that credentials are available - load from the specified profile if given
	ctx := context.Background()

	// Create config options to specify profile or use default
	var opts []func(*config.LoadOptions) error

	// Ensure we have a region
	if i.Common.Region == "" {
		i.Common.Region = "us-east-1" // Default region
	}

	// Add region to the configuration
	opts = append(opts, config.WithRegion(i.Common.Region))

	// If we're assuming a role, we need to make sure we're properly loading the source profile
	// The source profile comes from the identity's profile
	if i.Identity.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(i.Identity.Profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS credentials: %w", err)
	}

	// Verify credentials work by calling STS GetCallerIdentity
	stsClient := sts.NewFromConfig(cfg)
	identity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to validate AWS credentials: %w", err)
	}

	log.Debug("AWS credentials validated",
		"user", aws.ToString(identity.UserId),
		"account", aws.ToString(identity.Account),
		"arn", aws.ToString(identity.Arn),
	)

	return nil
}

// AssumeRole assumes the specified IAM role
func (i *awsAssumeRole) AssumeRole() error {
	ctx := context.Background()

	// Load the AWS configuration with the specified profile
	var opts []func(*config.LoadOptions) error

	// Ensure we have a region
	if i.Common.Region == "" {
		i.Common.Region = "us-east-1" // Default region
	}

	// Add region to the configuration
	opts = append(opts, config.WithRegion(i.Common.Region))

	// If we're assuming a role, we need to make sure we're properly loading the source profile
	if i.Identity.Profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(i.Identity.Profile))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create an STS client
	stsClient := sts.NewFromConfig(cfg)

	// Get the current identity to use as session name
	callerIdentity, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get caller identity: %w", err)
	}

	// Extract user ID for session name
	sessionName := fmt.Sprintf("AtmosSession-%s", os.Getenv("USER"))

	log.Debug("Assuming role",
		"role_arn", i.RoleArn,
		"source_identity", aws.ToString(callerIdentity.Arn),
		"session_name", sessionName,
	)

	// Assume the specified role
	result, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(i.RoleArn),
		RoleSessionName: aws.String(sessionName),
		DurationSeconds: aws.Int32(i.SessionDuration),
	})
	if err != nil {
		return fmt.Errorf("failed to assume role %s: %w", i.RoleArn, err)
	}

	// Write credentials to the AWS credentials file
	if result.Credentials != nil {
		WriteAwsCredentials(
			i.Identity.Profile,
			aws.ToString(result.Credentials.AccessKeyId),
			aws.ToString(result.Credentials.SecretAccessKey),
			aws.ToString(result.Credentials.SessionToken),
			"aws-assume-role",
		)
		log.Info("✅ Successfully assumed role",
			"role", i.RoleArn,
			"profile", i.Identity.Profile,
			"expires", result.Credentials.Expiration.Local().Format(time.RFC1123),
		)
		return nil
	}

	return fmt.Errorf("no credentials returned when assuming role %s", i.RoleArn)
}

func (i *awsAssumeRole) Logout() error {
	// Remove the credentials from the AWS credentials file
	return RemoveAwsCredentials(i.Identity.Profile)
}
