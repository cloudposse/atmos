package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	rdsauth "github.com/aws/aws-sdk-go-v2/feature/rds/auth"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// rdsTokenExpiry is the AWS-fixed lifetime of an RDS IAM authentication token (15 minutes).
// The SDK does not return the expiry, so it is synthesized here to match the service behavior.
const rdsTokenExpiry = 15 * time.Minute

// GetRDSToken generates an RDS IAM database authentication token from Atmos credentials.
// The token is a short-lived, SigV4-presigned "connect" URL used as the database password over
// a TLS connection. It is generated offline (no network call) and is valid for ~15 minutes.
//
// The endpoint must be in host:port form (e.g. mydb.abc123.us-east-2.rds.amazonaws.com:5432). If
// region is empty, the region carried by the credentials is used. The AWS config is loaded in
// isolation (LoadIsolatedAWSConfig) so an ambient AWS_PROFILE or shared config file cannot
// interfere with signing or corrupt the region.
func GetRDSToken(ctx context.Context, creds types.ICredentials, endpoint, region, dbUser string) (string, time.Time, error) {
	defer perf.Track(nil, "aws.GetRDSToken")()

	awsCreds, ok := creds.(*types.AWSCredentials)
	if !ok {
		return "", time.Time{}, fmt.Errorf("%w: expected AWS credentials", errUtils.ErrRDSTokenGeneration)
	}

	// Determine the signing region: an explicit region wins; otherwise fall back to the region
	// carried by the identity's credentials.
	effectiveRegion := region
	if effectiveRegion == "" {
		effectiveRegion = awsCreds.Region
	}

	// Build an ISOLATED AWS config from the identity's static credentials. Isolation is required
	// for correctness, not hygiene: an ambient AWS_PROFILE (common in CI, this command's advertised
	// use case) otherwise makes the SDK read ~/.aws/config and fail with SharedConfigProfileNotExistError
	// before the static credentials take effect, and an ambient AWS_REGION or shared config could sign
	// the token for the wrong region. Token signing needs nothing from the shared config files, so we
	// exclude them entirely. This also keeps the offline unit tests hermetic.
	cfg, err := LoadIsolatedAWSConfig(
		ctx,
		config.WithRegion(effectiveRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			awsCreds.AccessKeyID,
			awsCreds.SecretAccessKey,
			awsCreds.SessionToken,
		)),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
	}

	// BuildAuthToken signs the request offline; it makes no AWS API call.
	token, err := rdsauth.BuildAuthToken(ctx, endpoint, cfg.Region, dbUser, cfg.Credentials)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %w", errUtils.ErrRDSTokenGeneration, err)
	}

	// Log metadata only, never the token value (the token is a credential).
	log.Debug("Generated RDS IAM auth token", "endpoint", endpoint, "region", cfg.Region, "token_length", len(token))

	return token, time.Now().Add(rdsTokenExpiry), nil
}
