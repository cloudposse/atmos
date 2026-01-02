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

func TestBuildAWSConfigFromCreds_RegionHandling(t *testing.T) {
	tests := []struct {
		name              string
		credentialsRegion string
		providedRegion    string
		expectedRegion    string
	}{
		{
			name:              "uses provided region",
			credentialsRegion: "eu-west-1",
			providedRegion:    "us-east-1",
			expectedRegion:    "us-east-1",
		},
		{
			name:              "falls back to credentials region",
			credentialsRegion: "ap-southeast-1",
			providedRegion:    "",
			expectedRegion:    "ap-southeast-1",
		},
		{
			name:              "success with matching regions",
			credentialsRegion: "us-west-2",
			providedRegion:    "us-west-2",
			expectedRegion:    "us-west-2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			awsCreds := &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				Region:          tt.credentialsRegion,
			}

			cfg, err := buildAWSConfigFromCreds(ctx, awsCreds, tt.providedRegion)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedRegion, cfg.Region)
			assert.NotNil(t, cfg.Credentials)
		})
	}
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
