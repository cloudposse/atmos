package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// BuildAWSConfigFromCreds creates an AWS config from Atmos credentials.
// Used by ECR and EKS integrations to create AWS SDK clients.
//
// The config is loaded in isolation from the user's AWS environment (shared
// config and credentials files, AWS_PROFILE and ambient credential variables),
// like the rest of Atmos auth: the credentials are already resolved, and a
// stale AWS_PROFILE or a setting in ~/.aws/config must not break or alter the
// clients built from them.
func BuildAWSConfigFromCreds(ctx context.Context, creds types.ICredentials, region string) (aws.Config, error) {
	defer perf.Track(nil, "aws.BuildAWSConfigFromCreds")()

	awsCreds, ok := creds.(*types.AWSCredentials)
	if !ok {
		return aws.Config{}, fmt.Errorf("%w: expected AWS credentials", errUtils.ErrAuthenticationFailed)
	}

	// Determine region.
	effectiveRegion := region
	if effectiveRegion == "" {
		effectiveRegion = awsCreds.Region
	}

	// Build config with static credentials.
	cfg, err := LoadIsolatedAWSConfig(ctx,
		config.WithRegion(effectiveRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			awsCreds.AccessKeyID,
			awsCreds.SecretAccessKey,
			awsCreds.SessionToken,
		)),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("%w: %w", errUtils.ErrAuthenticationFailed, err)
	}

	return cfg, nil
}
