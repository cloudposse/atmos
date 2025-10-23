package mock

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestCredentials_BuildWhoamiInfo_NoSecretLeak(t *testing.T) {
	// Create mock credentials with sensitive data.
	creds := &Credentials{
		AccessKeyID:     "MOCK_ACCESS_KEY",
		SecretAccessKey: "MOCK_SECRET_KEY",
		SessionToken:    "MOCK_SESSION_TOKEN",
		Region:          "us-east-1",
		Expiration:      time.Now().Add(1 * time.Hour),
	}

	// Build WhoamiInfo.
	info := &types.WhoamiInfo{}
	creds.BuildWhoamiInfo(info)

	// Verify non-sensitive data is set.
	assert.Equal(t, "us-east-1", info.Region)
	assert.NotNil(t, info.Expiration)
	assert.NotNil(t, info.Environment)

	// Verify sensitive credentials are NOT in Environment map.
	assert.NotContains(t, info.Environment, "AWS_ACCESS_KEY_ID", "Access key should not be in Environment")
	assert.NotContains(t, info.Environment, "AWS_SECRET_ACCESS_KEY", "Secret key should not be in Environment")
	assert.NotContains(t, info.Environment, "AWS_SESSION_TOKEN", "Session token should not be in Environment")

	// Verify non-sensitive environment variables ARE present.
	assert.Equal(t, "us-east-1", info.Environment["AWS_REGION"])
	assert.Equal(t, "us-east-1", info.Environment["AWS_DEFAULT_REGION"])

	// Verify credentials are stored in non-serializable field.
	assert.NotNil(t, info.Credentials)

	// Verify serialization doesn't leak secrets.
	jsonBytes, err := json.Marshal(info)
	require.NoError(t, err)
	jsonStr := string(jsonBytes)

	// Check that sensitive values are NOT in JSON output.
	assert.NotContains(t, jsonStr, "MOCK_ACCESS_KEY", "Access key should not appear in JSON")
	assert.NotContains(t, jsonStr, "MOCK_SECRET_KEY", "Secret key should not appear in JSON")
	assert.NotContains(t, jsonStr, "MOCK_SESSION_TOKEN", "Session token should not appear in JSON")

	// Verify region is still in JSON (non-sensitive).
	assert.Contains(t, jsonStr, "us-east-1", "Region should appear in JSON")
}

func TestCredentials_BuildWhoamiInfo_NilCheck(t *testing.T) {
	creds := &Credentials{
		AccessKeyID:     "MOCK_KEY",
		SecretAccessKey: "MOCK_SECRET",
		Region:          "us-west-2",
	}

	// Should not panic with nil info.
	assert.NotPanics(t, func() {
		creds.BuildWhoamiInfo(nil)
	})
}

func TestCredentials_BuildWhoamiInfo_ZeroExpiration(t *testing.T) {
	creds := &Credentials{
		AccessKeyID: "MOCK_KEY",
		Region:      "eu-west-1",
		// Expiration is zero time.
	}

	info := &types.WhoamiInfo{}
	creds.BuildWhoamiInfo(info)

	// Expiration should not be set when it's zero.
	assert.Nil(t, info.Expiration, "Expiration should be nil for zero time")
}

func TestCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name       string
		expiration time.Time
		expected   bool
	}{
		{
			name:       "expired credentials",
			expiration: time.Now().Add(-1 * time.Hour),
			expected:   true,
		},
		{
			name:       "valid credentials",
			expiration: time.Now().Add(1 * time.Hour),
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &Credentials{
				Expiration: tt.expiration,
			}
			assert.Equal(t, tt.expected, creds.IsExpired())
		})
	}
}

func TestCredentials_GetExpiration(t *testing.T) {
	expTime := time.Now().Add(2 * time.Hour)
	creds := &Credentials{
		Expiration: expTime,
	}

	exp, err := creds.GetExpiration()
	require.NoError(t, err)
	require.NotNil(t, exp)
	assert.Equal(t, expTime, *exp)
}
