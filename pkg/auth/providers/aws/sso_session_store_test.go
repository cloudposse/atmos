package aws

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionKey_Deterministic(t *testing.T) {
	// Same inputs must produce identical keys — this is the contract that lets two
	// providers in atmos.yaml share a session.
	a := sessionKey("https://acme.awsapps.com/start", "us-east-1")
	b := sessionKey("https://acme.awsapps.com/start", "us-east-1")
	assert.Equal(t, a, b, "session key must be deterministic for identical inputs")
}

func TestSessionKey_DiffersByStartURL(t *testing.T) {
	a := sessionKey("https://acme.awsapps.com/start", "us-east-1")
	b := sessionKey("https://other.awsapps.com/start", "us-east-1")
	assert.NotEqual(t, a, b, "different start URLs must produce different keys")
}

func TestSessionKey_DiffersByRegion(t *testing.T) {
	a := sessionKey("https://acme.awsapps.com/start", "us-east-1")
	b := sessionKey("https://acme.awsapps.com/start", "us-west-2")
	assert.NotEqual(t, a, b, "different regions must produce different keys")
}

func TestSessionKey_IsHexSHA1(t *testing.T) {
	k := sessionKey("https://acme.awsapps.com/start", "us-east-1")
	// SHA1 hex-encoded is 40 chars.
	assert.Len(t, k, 40)
	for _, c := range k {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"session key must be lowercase hex: got %q", c)
	}
}

func TestSessionTokenStore_AcquireReturnsSameMutexForSameKey(t *testing.T) {
	s := newSessionTokenStore()
	mu1 := s.Acquire("portal-a")
	mu2 := s.Acquire("portal-a")
	assert.Same(t, mu1, mu2, "Acquire must return the same mutex pointer for the same key")
}

func TestSessionTokenStore_AcquireReturnsDistinctMutexesForDifferentKeys(t *testing.T) {
	s := newSessionTokenStore()
	mu1 := s.Acquire("portal-a")
	mu2 := s.Acquire("portal-b")
	assert.NotSame(t, mu1, mu2, "Acquire must isolate mutexes across distinct keys")
}

func TestSessionTokenStore_GetMissOnEmpty(t *testing.T) {
	s := newSessionTokenStore()
	_, ok := s.Get("never-set")
	assert.False(t, ok)
}

func TestSessionTokenStore_PutThenGet(t *testing.T) {
	s := newSessionTokenStore()
	token := ssoTokenCache{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
		StartURL:    "https://example.com",
		Region:      "us-east-1",
	}
	s.Put("portal-a", token)

	got, ok := s.Get("portal-a")
	require.True(t, ok)
	assert.Equal(t, "tok", got.AccessToken)
}

func TestSessionTokenStore_GetMissOnExpired(t *testing.T) {
	// Token expiring in 1 minute should be treated as expired because of the
	// 5-minute safety buffer in Get().
	s := newSessionTokenStore()
	s.Put("portal-a", ssoTokenCache{
		AccessToken: "soon-expired",
		ExpiresAt:   time.Now().Add(1 * time.Minute),
	})

	_, ok := s.Get("portal-a")
	assert.False(t, ok, "tokens within the 5-minute expiry buffer must be treated as missing")
}

func TestSessionTokenStore_Forget(t *testing.T) {
	s := newSessionTokenStore()
	s.Put("portal-a", ssoTokenCache{
		AccessToken: "tok",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	})
	// Touch the lock map too.
	_ = s.Acquire("portal-a")

	s.Forget("portal-a")

	_, ok := s.Get("portal-a")
	assert.False(t, ok, "Forget must drop the in-memory entry")
}

func TestSessionTokenStore_ConcurrentAcquireIsRaceSafe(t *testing.T) {
	// Sanity check that the package-level lock around the inner map prevents map
	// concurrent-write panics under contention. Run with -race for full coverage.
	s := newSessionTokenStore()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "portal"
			if idx%2 == 0 {
				key = "other-portal"
			}
			mu := s.Acquire(key)
			mu.Lock()
			s.Put(key, ssoTokenCache{
				AccessToken: "tok",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
			})
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	_, ok := s.Get("portal")
	assert.True(t, ok)
}
