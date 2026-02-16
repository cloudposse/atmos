package types

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestIDToken creates a test JWT ID token with the given claims.
func createTestIDToken(t *testing.T, claims map[string]interface{}) string {
	t.Helper()

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	signature := "fake-signature"

	return header + "." + payloadB64 + "." + signature
}

func TestOktaCredentials_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		expected  bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(time.Hour),
			expected:  false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-time.Hour),
			expected:  true,
		},
		{
			name:      "expires within 5 minutes (considered expired)",
			expiresAt: time.Now().Add(3 * time.Minute),
			expected:  true,
		},
		{
			name:      "zero time (expired)",
			expiresAt: time.Time{},
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &OktaCredentials{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.expected, creds.IsExpired())
		})
	}
}

func TestOktaCredentials_GetExpiration(t *testing.T) {
	t.Run("with expiration", func(t *testing.T) {
		expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)
		creds := &OktaCredentials{ExpiresAt: expiresAt}

		exp, err := creds.GetExpiration()
		require.NoError(t, err)
		require.NotNil(t, exp)
		// Compare truncated to handle timezone conversion.
		assert.Equal(t, expiresAt.Unix(), exp.Unix())
	})

	t.Run("zero expiration", func(t *testing.T) {
		creds := &OktaCredentials{}

		exp, err := creds.GetExpiration()
		require.NoError(t, err)
		assert.Nil(t, exp)
	})
}

func TestOktaCredentials_BuildWhoamiInfo(t *testing.T) {
	t.Run("with ID token", func(t *testing.T) {
		idToken := createTestIDToken(t, map[string]interface{}{
			"email": "user@example.com",
			"sub":   "00u123456789",
			"iss":   "https://company.okta.com",
		})

		expiresAt := time.Now().Add(time.Hour)
		creds := &OktaCredentials{
			OrgURL:    "https://company.okta.com",
			IDToken:   idToken,
			ExpiresAt: expiresAt,
		}

		info := &WhoamiInfo{}
		creds.BuildWhoamiInfo(info)

		assert.Equal(t, "user@example.com", info.Principal)
		assert.Equal(t, "https://company.okta.com", info.Account)
		assert.NotNil(t, info.Expiration)
	})

	t.Run("without ID token uses OrgURL as account", func(t *testing.T) {
		creds := &OktaCredentials{
			OrgURL: "https://company.okta.com",
		}

		info := &WhoamiInfo{}
		creds.BuildWhoamiInfo(info)

		assert.Equal(t, "https://company.okta.com", info.Account)
	})

	t.Run("with sub claim as principal (no email)", func(t *testing.T) {
		idToken := createTestIDToken(t, map[string]interface{}{
			"sub": "00u123456789",
			"iss": "https://company.okta.com",
		})

		creds := &OktaCredentials{
			OrgURL:  "https://company.okta.com",
			IDToken: idToken,
		}

		info := &WhoamiInfo{}
		creds.BuildWhoamiInfo(info)

		assert.Equal(t, "00u123456789", info.Principal)
	})
}

func TestOktaCredentials_Validate(t *testing.T) {
	ctx := context.Background()

	t.Run("valid credentials with ID token", func(t *testing.T) {
		idToken := createTestIDToken(t, map[string]interface{}{
			"email": "user@example.com",
			"sub":   "00u123456789",
			"iss":   "https://company.okta.com",
			"exp":   time.Now().Add(time.Hour).Unix(),
		})

		creds := &OktaCredentials{
			OrgURL:      "https://company.okta.com",
			AccessToken: "valid-access-token",
			IDToken:     idToken,
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		info, err := creds.Validate(ctx)
		require.NoError(t, err)
		assert.Equal(t, "user@example.com", info.Principal)
		assert.Equal(t, "https://company.okta.com", info.Account)
	})

	t.Run("valid credentials without ID token", func(t *testing.T) {
		creds := &OktaCredentials{
			OrgURL:      "https://company.okta.com",
			AccessToken: "valid-access-token",
			ExpiresAt:   time.Now().Add(time.Hour),
		}

		info, err := creds.Validate(ctx)
		require.NoError(t, err)
		assert.Equal(t, "https://company.okta.com", info.Account)
	})

	t.Run("empty access token", func(t *testing.T) {
		creds := &OktaCredentials{
			OrgURL:    "https://company.okta.com",
			ExpiresAt: time.Now().Add(time.Hour),
		}

		_, err := creds.Validate(ctx)
		require.Error(t, err)
	})

	t.Run("expired credentials", func(t *testing.T) {
		creds := &OktaCredentials{
			OrgURL:      "https://company.okta.com",
			AccessToken: "valid-access-token",
			ExpiresAt:   time.Now().Add(-time.Hour),
		}

		_, err := creds.Validate(ctx)
		require.Error(t, err)
	})
}

func TestOktaCredentials_CanRefresh(t *testing.T) {
	tests := []struct {
		name                  string
		refreshToken          string
		refreshTokenExpiresAt time.Time
		expected              bool
	}{
		{
			name:         "no refresh token",
			refreshToken: "",
			expected:     false,
		},
		{
			name:                  "refresh token with no expiration",
			refreshToken:          "test-refresh",
			refreshTokenExpiresAt: time.Time{},
			expected:              true,
		},
		{
			name:                  "refresh token not expired",
			refreshToken:          "test-refresh",
			refreshTokenExpiresAt: time.Now().Add(time.Hour),
			expected:              true,
		},
		{
			name:                  "refresh token expired",
			refreshToken:          "test-refresh",
			refreshTokenExpiresAt: time.Now().Add(-time.Hour),
			expected:              false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds := &OktaCredentials{
				RefreshToken:          tt.refreshToken,
				RefreshTokenExpiresAt: tt.refreshTokenExpiresAt,
			}
			assert.Equal(t, tt.expected, creds.CanRefresh())
		})
	}
}
