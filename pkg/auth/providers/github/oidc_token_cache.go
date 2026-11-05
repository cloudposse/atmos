package github

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	ghoidc "github.com/cloudposse/atmos/pkg/github/oidc"
)

// oidcTokenMinRemaining is how much lifetime a cached JWT must have left to be reused. Downstream
// identities exchange the JWT straight away (for example AWS AssumeRoleWithWebIdentity), so a
// minute of margin covers clock skew and the exchange itself.
const oidcTokenMinRemaining = 60 * time.Second

// processJWTCache holds GitHub OIDC JWTs fetched by this process. It lives only in memory: the
// provider stays ambient, so nothing is written to the keyring or to disk, but one Atmos command
// that authenticates several identities rooted at the provider makes one token request, not one
// per identity.
var processJWTCache = newJWTCache()

type jwtCacheEntry struct {
	token     string
	expiresAt time.Time
}

// jwtCache is a mutex-guarded map of JWTs plus a singleflight group, so concurrent callers that
// miss the cache share one fetch instead of each calling the token endpoint.
type jwtCache struct {
	mu      sync.Mutex
	entries map[string]jwtCacheEntry
	flights singleflight.Group
}

func newJWTCache() *jwtCache {
	return &jwtCache{entries: make(map[string]jwtCacheEntry)}
}

// jwtCacheKey identifies a token by request URL, request token and audience. The request token is
// part of the key so a JWT minted for one bearer is never handed to a caller presenting another.
func jwtCacheKey(requestURL, requestToken, audience string) string {
	return requestURL + "\x00" + requestToken + "\x00" + audience
}

// get returns a cached token that still has more than oidcTokenMinRemaining of lifetime at now.
func (c *jwtCache) get(key string, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if entry.expiresAt.Sub(now) <= oidcTokenMinRemaining {
		delete(c.entries, key)
		return "", false
	}
	return entry.token, true
}

// put caches token when its "exp" claim can be read and leaves enough lifetime to be reused.
// A token without a readable expiry is not cached, so the next caller fetches a fresh one.
func (c *jwtCache) put(key, token string, now time.Time) {
	expiresAt, err := ghoidc.ExpiresAt(token)
	if err != nil || expiresAt.Sub(now) <= oidcTokenMinRemaining {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = jwtCacheEntry{token: token, expiresAt: expiresAt}
}

// reset drops every cached token. Tests use it to isolate cases from each other.
func (c *jwtCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]jwtCacheEntry)
}
