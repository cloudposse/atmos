package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	awsCloud "github.com/cloudposse/atmos/pkg/auth/cloud/aws"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewSTSClientWithCredentials creates an STS client using the provided credentials and region.
// This is a shared helper used by assume-role and assume-root identities.
func NewSTSClientWithCredentials(
	ctx context.Context,
	awsBase *types.AWSCredentials,
	region string,
	identityConfig *schema.Identity,
) (*sts.Client, string, error) {
	defer perf.Track(nil, "aws.NewSTSClientWithCredentials")()

	// Resolve region with fallback.
	finalRegion := region
	if finalRegion == "" {
		finalRegion = awsBase.Region
	}
	if finalRegion == "" {
		finalRegion = defaultAWSRegion
	}

	configOpts := []func(*config.LoadOptions) error{
		config.WithRegion(finalRegion),
	}

	// Add custom endpoint resolver if configured.
	if identityConfig != nil {
		if resolverOpt := awsCloud.GetResolverConfigOption(identityConfig, nil); resolverOpt != nil {
			configOpts = append(configOpts, resolverOpt)
		}
	}

	// Load config with isolated environment to avoid conflicts with external AWS env vars.
	cfg, err := awsCloud.LoadIsolatedAWSConfig(ctx, configOpts...)
	if err != nil {
		return nil, finalRegion, err
	}
	cfg.Credentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
		awsBase.AccessKeyID, awsBase.SecretAccessKey, awsBase.SessionToken))
	return sts.NewFromConfig(cfg), finalRegion, nil
}
