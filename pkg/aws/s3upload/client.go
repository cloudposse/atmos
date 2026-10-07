package s3upload

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/aws/identity"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewClient resolves isolated AWS credentials and endpoints without changing the process environment.
func NewClient(ctx context.Context, region string, authContext *schema.AWSAuthContext, env map[string]string) (Client, error) {
	defer perf.Track(nil, "s3upload.NewClient")()
	cfg, err := identity.LoadConfigWithAuthAndEnv(ctx, region, "", 0, authContext, env)
	if err != nil {
		return nil, err
	}
	if authContext == nil && env["AWS_PROFILE"] == "" {
		if err := applyStaticCredentials(&cfg, env); err != nil {
			return nil, err
		}
	}
	return s3.NewFromConfig(cfg, func(options *s3.Options) {
		if endpoint := resolveEndpoint(authContext, env); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
			options.UsePathStyle = true
		}
	}), nil
}

func applyStaticCredentials(cfg *aws.Config, env map[string]string) error {
	// Workflow identity preparation supplies an isolated environment. Static
	// credentials in it must reach the SDK too; never export them via os.Setenv.
	key, secret := env["AWS_ACCESS_KEY_ID"], env["AWS_SECRET_ACCESS_KEY"]
	if key == "" && secret == "" {
		return nil
	}
	if key == "" || secret == "" {
		return fmt.Errorf("%w: both AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are required", errUtils.ErrS3Upload)
	}
	cfg.Credentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(key, secret, env["AWS_SESSION_TOKEN"]))
	return nil
}

func resolveEndpoint(authContext *schema.AWSAuthContext, env map[string]string) string {
	if authContext != nil {
		return authContext.EndpointURL
	}
	if endpoint := env["AWS_ENDPOINT_URL_S3"]; endpoint != "" {
		return endpoint
	}
	return env["AWS_ENDPOINT_URL"]
}
