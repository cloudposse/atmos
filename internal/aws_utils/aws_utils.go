package aws_utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// LoadAWSConfigWithAuth loads AWS config, preferring auth context if available.
/*
	When authContext is provided, it uses the Atmos-managed credentials files and profile.
	Otherwise, it falls back to standard AWS SDK credential resolution.

	Standard AWS SDK credential resolution order:

	Environment variables:
	  AWS_ACCESS_KEY_ID
	  AWS_SECRET_ACCESS_KEY
	  AWS_SESSION_TOKEN (optional, for temporary credentials)

	Shared credentials file:
	  Typically at ~/.aws/credentials
	  Controlled by:
	    AWS_PROFILE (defaults to default)
	    AWS_SHARED_CREDENTIALS_FILE

	Shared config file:
	  Typically at ~/.aws/config
	  Also supports named profiles and region settings

	Amazon EC2 Instance Metadata Service (IMDS):
	  If running on EC2 or ECS
	  Uses IAM roles attached to the instance/task

	Web Identity Token credentials:
	  When AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN are set (e.g., in EKS)

	SSO credentials (if configured)

	Custom credential sources:
	  Provided programmatically using config.WithCredentialsProvider(...)
*/
func LoadAWSConfigWithAuth(
	ctx context.Context,
	region string,
	roleArn string,
	assumeRoleDuration time.Duration,
	authContext *schema.AWSAuthContext,
) (aws.Config, error) {
	defer perf.Track(nil, "aws_utils.LoadAWSConfigWithAuth")()

	var cfgOpts []func(*config.LoadOptions) error

	// If auth context is provided, use Atmos-managed credentials.
	if authContext != nil {
		log.Debug("Using Atmos auth context for AWS SDK",
			"profile", authContext.Profile,
			"credentials", authContext.CredentialsFile,
			"config", authContext.ConfigFile,
		)

		// Set custom credential and config file paths.
		// This overrides the default ~/.aws/credentials and ~/.aws/config.
		cfgOpts = append(cfgOpts,
			config.WithSharedCredentialsFiles([]string{authContext.CredentialsFile}),
			config.WithSharedConfigFiles([]string{authContext.ConfigFile}),
			config.WithSharedConfigProfile(authContext.Profile),
		)

		// Use region from auth context if not explicitly provided.
		if region == "" && authContext.Region != "" {
			region = authContext.Region
		}
	} else {
		// Log default AWS SDK paths and environment for debugging.
		home, _ := os.UserHomeDir()
		defaultCredsFile := filepath.Join(home, ".aws", "credentials")
		defaultConfigFile := filepath.Join(home, ".aws", "config")
		profile := os.Getenv("AWS_PROFILE")
		if profile == "" {
			profile = "default"
		}

		log.Debug("Using standard AWS SDK credential resolution (no auth context provided)",
			"default_credentials_file", defaultCredsFile,
			"default_config_file", defaultConfigFile,
			"profile", profile,
			"AWS_SHARED_CREDENTIALS_FILE", os.Getenv("AWS_SHARED_CREDENTIALS_FILE"),
			"AWS_CONFIG_FILE", os.Getenv("AWS_CONFIG_FILE"),
			"AWS_PROFILE", os.Getenv("AWS_PROFILE"),
		)

		// Check if default files exist.
		if _, err := os.Stat(defaultCredsFile); err == nil {
			log.Debug("Found default AWS credentials file", "path", defaultCredsFile)
		} else {
			log.Debug("Default AWS credentials file not found", "path", defaultCredsFile)
		}

		if _, err := os.Stat(defaultConfigFile); err == nil {
			log.Debug("Found default AWS config file", "path", defaultConfigFile)
		} else {
			log.Debug("Default AWS config file not found", "path", defaultConfigFile)
		}
	}

	// Set region if provided.
	if region != "" {
		log.Debug("Using explicit region", "region", region)
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	// Load base config.
	log.Debug("Loading AWS SDK config", "num_options", len(cfgOpts))
	baseCfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		log.Debug("Failed to load AWS config", "error", err)
		return aws.Config{}, fmt.Errorf("%w: %v", errUtils.ErrLoadAwsConfig, err)
	}
	log.Debug("Successfully loaded AWS SDK config", "region", baseCfg.Region)

	// Conditionally assume role if specified.
	if roleArn != "" {
		log.Debug("Assuming role", "ARN", roleArn)
		stsClient := sts.NewFromConfig(baseCfg)

		creds := stscreds.NewAssumeRoleProvider(stsClient, roleArn, func(o *stscreds.AssumeRoleOptions) {
			o.Duration = assumeRoleDuration
		})

		cfgOpts = append(cfgOpts, config.WithCredentialsProvider(aws.NewCredentialsCache(creds)))

		// Reload full config with assumed role credentials.
		return config.LoadDefaultConfig(ctx, cfgOpts...)
	}

	return baseCfg, nil
}

// LoadAWSConfig loads AWS config using standard AWS SDK credential resolution.
// This is a wrapper around LoadAWSConfigWithAuth for backward compatibility.
// For new code that needs Atmos auth support, use LoadAWSConfigWithAuth instead.
func LoadAWSConfig(ctx context.Context, region string, roleArn string, assumeRoleDuration time.Duration) (aws.Config, error) {
	defer perf.Track(nil, "aws_utils.LoadAWSConfig")()

	return LoadAWSConfigWithAuth(ctx, region, roleArn, assumeRoleDuration, nil)
}
