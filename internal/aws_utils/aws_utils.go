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
)

// LoadAWSConfig loads AWS config.
/*
	It looks for credentials in the following order:

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
func LoadAWSConfig(ctx context.Context, region string, roleArn string, assumeRoleDuration time.Duration) (aws.Config, error) {
	defer perf.Track(nil, "aws_utils.LoadAWSConfig")()

	var cfgOpts []func(*config.LoadOptions) error

	// Conditionally set the region
	if region != "" {
		cfgOpts = append(cfgOpts, config.WithRegion(region))
	}

	// Load base config (from env, profile, etc.)
	// Note: We intentionally use config.LoadDefaultConfig here instead of LoadIsolatedAWSConfig
	// because this function is used in contexts where we want to honor environment variables
	// (e.g., Terraform backend configuration). The auth-specific code uses LoadIsolatedAWSConfig.
	baseCfg, err := config.LoadDefaultConfig(ctx, cfgOpts...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("%w: %v", errUtils.ErrLoadAwsConfig, err)
	}

	// Conditionally assume the role
	if roleArn != "" {
		log.Debug("Assuming role", "ARN", roleArn)
		stsClient := sts.NewFromConfig(baseCfg)

		creds := stscreds.NewAssumeRoleProvider(stsClient, roleArn, func(o *stscreds.AssumeRoleOptions) {
			o.Duration = assumeRoleDuration
		})

		cfgOpts = append(cfgOpts, config.WithCredentialsProvider(aws.NewCredentialsCache(creds)))

		// Reload full config with assumed role credentials
		return config.LoadDefaultConfig(ctx, cfgOpts...)
	}

	return baseCfg, nil
}
