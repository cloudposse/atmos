package auth

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
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

	return nil
}

// Login verifies AWS credentials are available in the default profile
func (i *awsAssumeRole) Login() error {
	log.Debug("Validating AWS credentials")

	// Set up session duration if not specified
	if i.SessionDuration == 0 {
		i.SessionDuration = 3600 // Default to 1 hour
	}

	// Verify that credentials are available
	ctx := context.Background()
	_, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to load AWS credentials: %w", err)
	}

	// Assume the role
	stsClient := sts.New(sts.Options{
		Region: i.Common.Region,
	})
	log.Debug("Assuming role", "role", i.RoleArn, "source_identity", i.Identity.Profile)
	result, err := stsClient.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         aws.String(i.RoleArn),
		SourceIdentity:  aws.String(i.Identity.Profile),
		RoleSessionName: aws.String(fmt.Sprintf("atmos-%s-%s", i.Identity.Profile, os.Getenv("USER"))),
		DurationSeconds: aws.Int32(i.SessionDuration),
	})
	if err != nil {
		return fmt.Errorf("failed to assume role %s: %w", i.RoleArn, err)
	}
	log.Debug("Assumed role", "role", result.AssumedRoleUser.Arn)
	//
	//// Save the temporary credentials
	//i.Identity.Credentials.AccessKeyId = *result.Credentials.AccessKeyId
	//i.Identity.Credentials.SecretAccessKey = *result.Credentials.SecretAccessKey
	//i.Identity.Credentials.SessionToken = *result.Credentials.SessionToken
	return nil
}

// AssumeRole assumes the specified IAM role
func (i *awsAssumeRole) AssumeRole() error {
	ctx := context.Background()

	// Load the AWS configuration
	cfg, err := config.LoadDefaultConfig(ctx)
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
	sessionName := fmt.Sprintf("AtmosSession-%s", aws.ToString(callerIdentity.UserId))

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
			"expires", result.Credentials.Expiration,
		)
		return nil
	}

	return fmt.Errorf("no credentials returned when assuming role %s", i.RoleArn)
}

func (i *awsAssumeRole) Logout() error {
	// Remove the credentials from the AWS credentials file
	return RemoveAwsCredentials(i.Identity.Profile)
}
