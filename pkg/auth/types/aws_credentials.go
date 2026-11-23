package types

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	errUtils "github.com/cloudposse/atmos/errors"
)

// AWSCredentials defines AWS-specific credential fields.
type AWSCredentials struct {
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	SessionToken    string `json:"session_token,omitempty"`
	Region          string `json:"region,omitempty"`
	Expiration      string `json:"expiration,omitempty"`
	MfaArn          string `json:"mfa_arn,omitempty"`
	SessionDuration string `json:"session_duration,omitempty"` // Duration string (e.g., "12h", "24h")

	// validationEndpoint is the STS endpoint Validate calls (see SetValidationEndpoint).
	// It is runtime-only and never serialized.
	validationEndpoint string
}

// IsExpired returns true if the credentials are expired.
// This implements the ICredentials interface.
func (c *AWSCredentials) IsExpired() bool {
	if c.Expiration == "" {
		return false
	}
	expTime, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return true
	}
	return !time.Now().Before(expTime)
}

// GetExpiration implements ICredentials for AWSCredentials.
func (c *AWSCredentials) GetExpiration() (*time.Time, error) {
	if c.Expiration == "" {
		return nil, nil
	}
	expTime, err := time.Parse(time.RFC3339, c.Expiration)
	if err != nil {
		return nil, fmt.Errorf("%w: failed parsing AWS credential expiration: %w", errUtils.ErrInvalidAuthConfig, err)
	}
	// Convert to local timezone for display to user.
	localTime := expTime.Local()
	return &localTime, nil
}

// BuildWhoamiInfo implements ICredentials for AWSCredentials.
func (c *AWSCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	info.Region = c.Region
	if t, _ := c.GetExpiration(); t != nil {
		info.Expiration = t
	}
}

// awsValidationTimeout bounds the STS GetCallerIdentity call made by Validate, so an
// unreachable endpoint (for example a black-holed proxy) reports quickly instead of
// waiting for the operating system connect timeout on every retry.
const awsValidationTimeout = 15 * time.Second

// awsValidationMaxAttempts keeps the status check quick when the endpoint is down: one
// retry covers a transient failure without the default three attempts and backoff.
const awsValidationMaxAttempts = 2

// awsRejectionErrorCodes are the STS error codes that mean the endpoint answered and
// definitively rejected the credentials (expired, unknown, or badly signed).
var awsRejectionErrorCodes = map[string]struct{}{
	"ExpiredToken":                {},
	"ExpiredTokenException":       {},
	"InvalidClientTokenId":        {},
	"InvalidAccessKeyId":          {},
	"SignatureDoesNotMatch":       {},
	"UnrecognizedClientException": {},
	"AccessDenied":                {},
	"AccessDeniedException":       {},
	"AuthFailure":                 {},
	"InvalidToken":                {},
	"RequestExpired":              {},
}

// SetValidationEndpoint sets the STS endpoint URL that Validate calls, the same
// `spec.endpoint_url` the identity authenticates against (LocalStack, a proxy, or a
// private endpoint). An empty URL uses the public regional STS endpoint.
func (c *AWSCredentials) SetValidationEndpoint(endpointURL string) {
	c.validationEndpoint = endpointURL
}

// Validate validates AWS credentials by calling STS GetCallerIdentity.
// Returns validation info including ARN, account, and expiration.
//
// Expired credentials fail with ErrExpiredCredentials without a network call. When
// STS answers and rejects the credentials the error wraps ErrAuthenticationFailed.
// When STS cannot be reached or the call cannot be made (network, proxy, timeout,
// cancellation, or local configuration such as a missing region), the error wraps
// ErrCredentialValidationUnavailable instead: the credentials may still be valid.
func (c *AWSCredentials) Validate(ctx context.Context) (*ValidationInfo, error) {
	if c.IsExpired() {
		return nil, fmt.Errorf("%w: AWS credentials expired at %s", errUtils.ErrExpiredCredentials, c.Expiration)
	}

	// Note: This is a validation check using explicit credentials, not loading from files,
	// so we create a minimal config with just the credentials, region, and endpoint.
	cfg := aws.Config{
		Region:           c.Region,
		RetryMaxAttempts: awsValidationMaxAttempts,
		Credentials: credentials.NewStaticCredentialsProvider(
			c.AccessKeyID,
			c.SecretAccessKey,
			c.SessionToken,
		),
	}
	if c.validationEndpoint != "" {
		cfg.BaseEndpoint = aws.String(c.validationEndpoint)
	}

	// Create STS client.
	stsClient := sts.NewFromConfig(cfg)

	callCtx, cancel := context.WithTimeout(ctx, awsValidationTimeout)
	defer cancel()

	// Call GetCallerIdentity to validate credentials and get ARN.
	result, err := stsClient.GetCallerIdentity(callCtx, &sts.GetCallerIdentityInput{})
	if err != nil {
		if isAWSCredentialRejection(err) {
			return nil, fmt.Errorf("%w: failed to validate AWS credentials: %w", errUtils.ErrAuthenticationFailed, err)
		}
		return nil, fmt.Errorf("%w: AWS STS could not be reached to validate the credentials: %w", errUtils.ErrCredentialValidationUnavailable, err)
	}

	// Build validation info from GetCallerIdentity response.
	info := &ValidationInfo{
		Principal: aws.ToString(result.Arn),
		Account:   aws.ToString(result.Account),
	}

	// Add expiration time if available.
	if expTime, err := c.GetExpiration(); err == nil && expTime != nil {
		info.Expiration = expTime
	}

	return info, nil
}

// isAWSCredentialRejection reports whether err shows that STS answered and rejected the
// credentials: a known rejection code, any client-fault API error, or an HTTP 401/403.
// Transport failures (connection refused, DNS, proxy, timeout) are not rejections.
func isAWSCredentialRejection(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if _, ok := awsRejectionErrorCodes[apiErr.ErrorCode()]; ok {
			return true
		}
		if apiErr.ErrorFault() == smithy.FaultClient {
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		status := respErr.HTTPStatusCode()
		return status == http.StatusUnauthorized || status == http.StatusForbidden
	}
	return false
}
