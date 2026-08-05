package aws

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestAuthenticate_TwoProvidersSharingSessionHitInMemoryCache asserts the headline
// behavior of the AWS SSO Token Provider work: two providers pointing at the same
// (start_url, region) produce identical session keys and therefore share the
// in-memory session cache. The second provider's Authenticate() call returns
// immediately without touching the network or filesystem.
func TestAuthenticate_TwoProvidersSharingSessionHitInMemoryCache(t *testing.T) {
	const (
		sharedStartURL = "https://acme.awsapps.com/start"
		sharedRegion   = "us-east-1"
	)

	// Construct an isolated session store so the test doesn't depend on
	// (or pollute) the package-level singleton.
	store := newSessionTokenStore()
	key := sessionKey(sharedStartURL, sharedRegion)

	// Seed the store as if the first provider had already authenticated.
	const seededAccessToken = "seeded-access-token"
	store.Put(key, ssoTokenCache{
		AccessToken: seededAccessToken,
		Region:      sharedRegion,
		StartURL:    sharedStartURL,
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	})

	// Build a second provider with the same portal config but a different name.
	provider, err := NewSSOProvider("provider-two", &schema.Provider{
		Kind:     "aws/iam-identity-center",
		StartURL: sharedStartURL,
		Region:   sharedRegion,
	})
	require.NoError(t, err)
	provider.sessionStore = store

	// The second provider must return the seeded token without making any
	// network call.
	creds, err := provider.Authenticate(context.Background())
	require.NoError(t, err)

	awsCreds, ok := creds.(*authTypes.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, seededAccessToken, awsCreds.AccessKeyID,
		"second provider must receive the token cached by the first provider")
	assert.Equal(t, sharedRegion, awsCreds.Region)
}

// TestAuthenticate_DifferentPortalsDoNotShareCache asserts the inverse: providers
// configured against different SSO portals (or different regions) must NOT share
// the cached token. Each gets its own session and its own browser flow.
func TestAuthenticate_DifferentPortalsDoNotShareCache(t *testing.T) {
	store := newSessionTokenStore()

	// Seed portal-A.
	keyA := sessionKey("https://acme.awsapps.com/start", "us-east-1")
	store.Put(keyA, ssoTokenCache{
		AccessToken: "portal-a-token",
		Region:      "us-east-1",
		StartURL:    "https://acme.awsapps.com/start",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	})

	// Provider for portal-B should not see portal-A's token in its in-memory check.
	keyB := sessionKey("https://other.awsapps.com/start", "us-east-1")
	_, ok := store.Get(keyB)
	assert.False(t, ok, "providers for different portals must not share cache entries")

	// And the keys themselves must differ.
	assert.NotEqual(t, keyA, keyB)
}
