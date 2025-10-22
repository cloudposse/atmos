package aws_utils

import (
	"context"
	"fmt"
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
// If authContext is provided, it uses the Atmos-managed credentials files and profile.
// Otherwise, it falls back to standard AWS SDK credential resolution.
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
	}

	// Set region if provided.
	if region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	// Load base config.
	baseCfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("%w: %v", errUtils.ErrLoadAwsConfig, err)
	}

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
