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
	return time.Now().After(expTime)
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
	return &expTime, nil
}

// BuildWhoamiInfo implements ICredentials for AWSCredentials.
func (c *AWSCredentials) BuildWhoamiInfo(info *WhoamiInfo) {
	info.Region = c.Region
	if t, _ := c.GetExpiration(); t != nil {
		info.Expiration = t
	}
}

// Validate validates AWS credentials by calling STS GetCallerIdentity.
// Returns the expiration time if available, or an error if credentials are invalid.
func (c *AWSCredentials) Validate(ctx context.Context) (*time.Time, error) {
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

	// Call GetCallerIdentity to validate credentials.
	_, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("%w: failed to validate AWS credentials: %w", errUtils.ErrAuthenticationFailed, err)
	}

	// Return expiration time if available.
	if expTime, err := c.GetExpiration(); err == nil && expTime != nil {
		return expTime, nil
	}

	// No expiration available (long-term credentials).
	return nil, nil
}
