package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecrpublic"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// ECRPublicRegistryURL is the fixed registry URL for ECR Public.
	ECRPublicRegistryURL = "public.ecr.aws"

	// ECRPublicAuthRegion is the only region that supports ECR Public authentication.
	// AWS docs: "always authenticate to the us-east-1 Region".
	ECRPublicAuthRegion = "us-east-1"
)

// ECRPublicSupportedRegions contains the regions where ECR Public service endpoints exist.
// Auth (GetAuthorizationToken) is only supported in us-east-1, but the service
// has endpoints in both us-east-1 and us-west-2 for other API operations.
var ECRPublicSupportedRegions = map[string]bool{
	"us-east-1": true,
	"us-west-2": true,
}

// ECRPublicAuthResult contains ECR Public authorization token information.
type ECRPublicAuthResult struct {
	Username  string    // Always "AWS".
	Password  string    //nolint:gosec // G117: This is an authorization token, not a password secret.
	ExpiresAt time.Time // Token expiration time.
}

// GetPublicAuthorizationToken retrieves ECR Public credentials using AWS credentials.
// The auth call is always made to us-east-1, which is the only region that supports it.
func GetPublicAuthorizationToken(ctx context.Context, creds types.ICredentials) (*ECRPublicAuthResult, error) {
	defer perf.Track(nil, "aws.GetPublicAuthorizationToken")()

	// Build AWS config from credentials, forcing us-east-1 for auth.
	cfg, err := buildAWSConfigFromCreds(ctx, creds, ECRPublicAuthRegion)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to build AWS config: %w", errUtils.ErrECRPublicAuthFailed, err)
	}

	// Create ECR Public client.
	client := ecrpublic.NewFromConfig(cfg)

	// Get authorization token.
	result, err := client.GetAuthorizationToken(ctx, &ecrpublic.GetAuthorizationTokenInput{})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrECRPublicAuthFailed, err)
	}

	if result.AuthorizationData == nil || result.AuthorizationData.AuthorizationToken == nil {
		return nil, fmt.Errorf("%w: no authorization data returned", errUtils.ErrECRPublicAuthFailed)
	}

	// Decode the authorization token (base64 encoded "username:password").
	decoded, err := base64.StdEncoding.DecodeString(*result.AuthorizationData.AuthorizationToken)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode token: %w", errUtils.ErrECRPublicAuthFailed, err)
	}

	// Split into username and password.
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid token format", errUtils.ErrECRPublicAuthFailed)
	}

	// Parse expiration time.
	var expiresAt time.Time
	if result.AuthorizationData.ExpiresAt != nil {
		expiresAt = *result.AuthorizationData.ExpiresAt
	}

	return &ECRPublicAuthResult{
		Username:  parts[0],
		Password:  parts[1],
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateECRPublicRegion validates that the given region is supported by ECR Public.
func ValidateECRPublicRegion(region string) error {
	defer perf.Track(nil, "aws.ValidateECRPublicRegion")()

	if !ECRPublicSupportedRegions[region] {
		return fmt.Errorf("%w: %q (supported: us-east-1, us-west-2)", errUtils.ErrECRPublicInvalidRegion, region)
	}
	return nil
}

// IsECRPublicRegistry checks if a URL is the ECR Public registry.
func IsECRPublicRegistry(url string) bool {
	defer perf.Track(nil, "aws.IsECRPublicRegistry")()

	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Match "public.ecr.aws" with optional trailing path.
	return url == ECRPublicRegistryURL || strings.HasPrefix(url, ECRPublicRegistryURL+"/")
}
