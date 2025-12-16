package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ECRAuthResult contains ECR authorization token information.
type ECRAuthResult struct {
	Username  string    // Always "AWS".
	Password  string    // Decoded authorization token.
	Registry  string    // e.g., 123456789012.dkr.ecr.us-east-1.amazonaws.com.
	ExpiresAt time.Time // Token expiration time.
}

// ecrRegistryPattern matches ECR registry URLs.
var ecrRegistryPattern = regexp.MustCompile(`^(\d{12})\.dkr\.ecr\.([a-z0-9-]+)\.amazonaws\.com$`)

// GetAuthorizationToken retrieves ECR credentials using AWS credentials.
// The accountID parameter is optional - if empty, uses the caller's account.
func GetAuthorizationToken(ctx context.Context, creds types.ICredentials, accountID, region string) (*ECRAuthResult, error) {
	defer perf.Track(nil, "aws.GetAuthorizationToken")()

	// Build AWS config from credentials.
	cfg, err := buildAWSConfigFromCreds(ctx, creds, region)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to build AWS config: %w", errUtils.ErrECRAuthFailed, err)
	}

	// Create ECR client.
	client := ecr.NewFromConfig(cfg)

	// Build input.
	input := &ecr.GetAuthorizationTokenInput{}
	if accountID != "" {
		input.RegistryIds = []string{accountID}
	}

	// Get authorization token.
	result, err := client.GetAuthorizationToken(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrECRAuthFailed, err)
	}

	if len(result.AuthorizationData) == 0 {
		return nil, fmt.Errorf("%w: no authorization data returned", errUtils.ErrECRAuthFailed)
	}

	authData := result.AuthorizationData[0]
	if authData.AuthorizationToken == nil {
		return nil, fmt.Errorf("%w: authorization token is nil", errUtils.ErrECRAuthFailed)
	}

	// Decode the authorization token (base64 encoded "username:password").
	decoded, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode token: %w", errUtils.ErrECRAuthFailed, err)
	}

	// Split into username and password.
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: invalid token format", errUtils.ErrECRAuthFailed)
	}

	// Parse expiration time.
	var expiresAt time.Time
	if authData.ExpiresAt != nil {
		expiresAt = *authData.ExpiresAt
	}

	// Build registry URL.
	registry := ""
	if authData.ProxyEndpoint != nil {
		// Remove https:// prefix if present.
		registry = strings.TrimPrefix(*authData.ProxyEndpoint, "https://")
	}

	return &ECRAuthResult{
		Username:  parts[0],
		Password:  parts[1],
		Registry:  registry,
		ExpiresAt: expiresAt,
	}, nil
}

// BuildRegistryURL constructs ECR registry URL from account ID and region.
func BuildRegistryURL(accountID, region string) string {
	defer perf.Track(nil, "aws.BuildRegistryURL")()

	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", accountID, region)
}

// ParseRegistryURL extracts account ID and region from ECR registry URL.
// Returns error if URL is not a valid ECR registry URL.
func ParseRegistryURL(registryURL string) (accountID, region string, err error) {
	defer perf.Track(nil, "aws.ParseRegistryURL")()

	// Remove https:// prefix if present.
	registryURL = strings.TrimPrefix(registryURL, "https://")

	matches := ecrRegistryPattern.FindStringSubmatch(registryURL)
	if len(matches) != 3 {
		return "", "", fmt.Errorf("%w: %s", errUtils.ErrECRInvalidRegistry, registryURL)
	}

	return matches[1], matches[2], nil
}

// IsECRRegistry checks if a URL is an ECR registry URL.
func IsECRRegistry(url string) bool {
	url = strings.TrimPrefix(url, "https://")
	return ecrRegistryPattern.MatchString(url)
}

// buildAWSConfigFromCreds creates an AWS config from Atmos credentials.
func buildAWSConfigFromCreds(ctx context.Context, creds types.ICredentials, region string) (aws.Config, error) {
	awsCreds, ok := creds.(*types.AWSCredentials)
	if !ok {
		return aws.Config{}, fmt.Errorf("%w: expected AWS credentials", errUtils.ErrECRAuthFailed)
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
