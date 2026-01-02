package aws

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestBuildAWSConfigFromCreds_InvalidCredentialsType(t *testing.T) {
	// Test with non-AWS credentials type - should return error.
	ctx := context.Background()

	// Create a mock credentials type that isn't AWSCredentials.
	mockCreds := &mockNonAWSCredentials{}

	_, err := buildAWSConfigFromCreds(ctx, mockCreds, "us-east-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrECRAuthFailed)
	assert.Contains(t, err.Error(), "expected AWS credentials")
}

func TestBuildAWSConfigFromCreds_UsesProvidedRegion(t *testing.T) {
	ctx := context.Background()

	awsCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "token123",
		Region:          "eu-west-1", // Credentials have eu-west-1.
	}

	// Provide different region - should use provided region, not credential's region.
	cfg, err := buildAWSConfigFromCreds(ctx, awsCreds, "us-east-1")
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.Region) // Should use provided region.
}

func TestBuildAWSConfigFromCreds_FallsBackToCredentialsRegion(t *testing.T) {
	ctx := context.Background()

	awsCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "token123",
		Region:          "ap-southeast-1",
	}

	// Empty provided region - should fall back to credential's region.
	cfg, err := buildAWSConfigFromCreds(ctx, awsCreds, "")
	require.NoError(t, err)
	assert.Equal(t, "ap-southeast-1", cfg.Region) // Should use credentials' region.
}

func TestBuildAWSConfigFromCreds_Success(t *testing.T) {
	ctx := context.Background()

	awsCreds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "",
		Region:          "us-west-2",
	}

	cfg, err := buildAWSConfigFromCreds(ctx, awsCreds, "us-west-2")
	require.NoError(t, err)
	assert.NotNil(t, cfg.Credentials)
	assert.Equal(t, "us-west-2", cfg.Region)
}

// mockNonAWSCredentials is a mock that implements ICredentials but isn't AWSCredentials.
type mockNonAWSCredentials struct{}

func (m *mockNonAWSCredentials) IsExpired() bool {
	return false
}

func (m *mockNonAWSCredentials) GetExpiration() (*time.Time, error) {
	return nil, nil
}

func (m *mockNonAWSCredentials) BuildWhoamiInfo(_ *types.WhoamiInfo) {
	// No-op for mock.
}

func (m *mockNonAWSCredentials) Validate(_ context.Context) (*types.ValidationInfo, error) {
	return nil, nil
}

func TestECRAuthResult_Fields(t *testing.T) {
	// Test ECRAuthResult struct fields.
	expiresAt := time.Now().Add(12 * time.Hour)
	result := &ECRAuthResult{
		Username:  "AWS",
		Password:  "test-token-123",
		Registry:  "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		ExpiresAt: expiresAt,
	}

	assert.Equal(t, "AWS", result.Username)
	assert.Equal(t, "test-token-123", result.Password)
	assert.Equal(t, "123456789012.dkr.ecr.us-east-1.amazonaws.com", result.Registry)
	assert.Equal(t, expiresAt, result.ExpiresAt)
}
