package types

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestGetExpiration is a helper to test GetExpiration behavior for any credential type.
// SetExpiration should set the Expiration field on the credential instance.
func testGetExpiration(t *testing.T, cred ICredentials, setExpiration func(interface{}, string)) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)

	// Blank.
	exp, err := cred.GetExpiration()
	assert.NoError(t, err)
	assert.Nil(t, exp)

	// Valid.
	setExpiration(cred, now.Add(30*time.Minute).Format(time.RFC3339))
	exp, err = cred.GetExpiration()
	assert.NoError(t, err)
	if assert.NotNil(t, exp) {
		assert.WithinDuration(t, now.Add(30*time.Minute), *exp, time.Second)
	}

	// Invalid.
	setExpiration(cred, "bogus")
	exp, err = cred.GetExpiration()
	assert.Nil(t, exp)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig))
}

func TestAzureCredentials_IsExpired(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name string
		exp  string
		want bool
	}{
		{"no-exp", "", false},
		{"invalid-format", "not-a-time", true},
		{"past", now.Add(-1 * time.Hour).Format(time.RFC3339), true},
		{"future", now.Add(1 * time.Hour).Format(time.RFC3339), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &AzureCredentials{Expiration: tc.exp}
			assert.Equal(t, tc.want, c.IsExpired())
		})
	}
}

func TestAzureCredentials_GetExpiration(t *testing.T) {
	testGetExpiration(t, &AzureCredentials{}, func(c interface{}, exp string) {
		c.(*AzureCredentials).Expiration = exp
	})
}

func TestAzureCredentials_BuildWhoamiInfo(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	c := &AzureCredentials{
		SubscriptionID: "12345678-1234-1234-1234-123456789012",
		Location:       "eastus",
		Expiration:     now.Add(15 * time.Minute).Format(time.RFC3339),
	}

	var w WhoamiInfo
	c.BuildWhoamiInfo(&w)

	assert.Equal(t, "12345678-1234-1234-1234-123456789012", w.Account)
	assert.Equal(t, "eastus", w.Region)
	if assert.NotNil(t, w.Expiration) {
		assert.WithinDuration(t, now.Add(15*time.Minute), *w.Expiration, time.Second)
	}
}

func TestAzureCredentials_Validate(t *testing.T) {
	// Note: Validate requires valid Azure credentials.
	// In test environment without Azure creds, this will fail.
	// This test verifies the method exists and returns appropriate error.
	c := &AzureCredentials{
		AccessToken:    "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsIng1dCI6IkZTaW11RnJGTm9DMHNKWEdtdjEzbk5aY2VEYyIsImtpZCI6IkZTaW11RnJGTm9DMHNKWEdtdjEzbk5aY2VEYyJ9.EXAMPLE",
		TokenType:      "Bearer",
		TenantID:       "12345678-1234-1234-1234-123456789012",
		SubscriptionID: "87654321-4321-4321-4321-210987654321",
	}

	ctx := context.Background()
	_, err := c.Validate(ctx)

	// Should return authentication error (invalid credentials).
	assert.Error(t, err, "Validate should fail with invalid credentials")
	assert.True(t, errors.Is(err, errUtils.ErrAuthenticationFailed), "Should return ErrAuthenticationFailed")
}

func TestAzureCredentials_Validate_EmptySubscriptionID(t *testing.T) {
	// Test that Validate fails fast when subscription ID is empty.
	c := &AzureCredentials{
		AccessToken: "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsIng1dCI6IkZTaW11RnJGTm9DMHNKWEdtdjEzbk5aY2VEYyIsImtpZCI6IkZTaW11RnJGTm9DMHNKWEdtdjEzbk5aY2VEYyJ9.EXAMPLE",
		TokenType:   "Bearer",
		TenantID:    "12345678-1234-1234-1234-123456789012",
		// SubscriptionID is intentionally empty.
	}

	ctx := context.Background()
	_, err := c.Validate(ctx)

	// Should return configuration error without making Azure API call.
	assert.Error(t, err, "Validate should fail with empty subscription ID")
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "Should return ErrInvalidAuthConfig")
	assert.Contains(t, err.Error(), "subscription ID is required", "Error message should mention subscription ID")
}

func TestAzureCredentials_Validate_WithExpiration(t *testing.T) {
	// Test that Validate handles credentials that include an Expiration value.
	now := time.Now().UTC()
	c := &AzureCredentials{
		AccessToken:    "eyJ0eXAiOiJKV1QiLCJhbGciOiJSUzI1NiIsIng1dCI6IkZTaW11RnJGTm9DMHNKWEdtdjEzbk5aY2VEYyIsImtpZCI6IkZTaW11RnJGTm9DMHNKWEdtdjEzbk5aY2VEYyJ9.EXAMPLE",
		TokenType:      "Bearer",
		TenantID:       "12345678-1234-1234-1234-123456789012",
		SubscriptionID: "87654321-4321-4321-4321-210987654321",
		Expiration:     now.Add(1 * time.Hour).Format(time.RFC3339),
	}

	// This will fail validation (invalid creds); this primarily ensures the code
	// path with a non-empty Expiration field does not panic and returns an error.
	ctx := context.Background()
	_, err := c.Validate(ctx)
	assert.Error(t, err, "Should fail with invalid credentials")
}

func TestAzureCredentials_MultipleTokens(t *testing.T) {
	// Test that all token types are properly stored.
	now := time.Now().UTC()
	c := &AzureCredentials{
		AccessToken:        "access-token-here",
		GraphAPIToken:      "graph-token-here",
		GraphAPIExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
		KeyVaultToken:      "keyvault-token-here",
		KeyVaultExpiration: now.Add(1 * time.Hour).Format(time.RFC3339),
		Expiration:         now.Add(1 * time.Hour).Format(time.RFC3339),
		TenantID:           "tenant-123",
		SubscriptionID:     "sub-456",
	}

	// Verify all tokens are present.
	assert.NotEmpty(t, c.AccessToken, "AccessToken should be set")
	assert.NotEmpty(t, c.GraphAPIToken, "GraphAPIToken should be set")
	assert.NotEmpty(t, c.KeyVaultToken, "KeyVaultToken should be set")
	assert.NotEmpty(t, c.Expiration, "Expiration should be set")
	assert.NotEmpty(t, c.GraphAPIExpiration, "GraphAPIExpiration should be set")
	assert.NotEmpty(t, c.KeyVaultExpiration, "KeyVaultExpiration should be set")

	// Test that they're not expired.
	assert.False(t, c.IsExpired(), "Credentials should not be expired")
}

func TestAzureCredentials_EmptyOptionalFields(t *testing.T) {
	// Test that credentials work with only required fields.
	now := time.Now().UTC()
	c := &AzureCredentials{
		AccessToken:    "access-token-here",
		Expiration:     now.Add(1 * time.Hour).Format(time.RFC3339),
		TenantID:       "12345678-1234-1234-1234-123456789012",
		SubscriptionID: "87654321-4321-4321-4321-210987654321",
		// Optional tokens not set.
	}

	// Verify required fields are present.
	assert.NotEmpty(t, c.AccessToken, "AccessToken should be set")
	assert.NotEmpty(t, c.TenantID, "TenantID should be set")
	assert.NotEmpty(t, c.SubscriptionID, "SubscriptionID should be set")
	assert.False(t, c.IsExpired(), "Credentials should not be expired")

	// Verify optional fields are empty.
	assert.Empty(t, c.GraphAPIToken, "GraphAPIToken should be empty")
	assert.Empty(t, c.KeyVaultToken, "KeyVaultToken should be empty")
	assert.Empty(t, c.Location, "Location should be empty")
}
