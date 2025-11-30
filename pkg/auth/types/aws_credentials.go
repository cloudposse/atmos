package types

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"

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

// Validate validates AWS credentials by calling STS GetCallerIdentity.
// Returns validation info including ARN, account, and expiration.
func (c *AWSCredentials) Validate(ctx context.Context) (*ValidationInfo, error) {
	// Import here to avoid circular dependency issues.
	// Note: This is a validation check using explicit credentials, not loading from files,
	// so we create a minimal config with just the credentials and region.
	cfg := aws.Config{
		Region: c.Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			c.AccessKeyID,
			c.SecretAccessKey,
			c.SessionToken,
		),
	}

	// Create STS client.
	stsClient := sts.NewFromConfig(cfg)

	// Call GetCallerIdentity to validate credentials and get ARN.
	result, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to validate AWS credentials: %w", errUtils.ErrAuthenticationFailed, err)
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
