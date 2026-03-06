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
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(effectiveRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			awsCreds.AccessKeyID,
			awsCreds.SecretAccessKey,
			awsCreds.SessionToken,
		)),
	)
	if err != nil {
		return aws.Config{}, err
	}

	return cfg, nil
}
