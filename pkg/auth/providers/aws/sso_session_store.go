package aws

import (
	"sync"
	"time"
)

// sessionTokenStore is an in-process registry that ensures a single device-authorization
// flow runs even when N atmos providers call Authenticate() concurrently for the same
// SSO portal session (keyed by start_url + region).
//
// Without this store, two providers in the same atmos.yaml pointing at the same SSO
// portal — or two parallel `atmos terraform plan` invocations for different identities
// backed by the same portal — would each run their own device-authorization flow.
// With this store, they all wait on the same per-session mutex and reuse the resulting
// token.
//
// The store is process-scoped (a package-level singleton). Cross-process coordination
// is out of scope for v1; see the AWS SSO Token Provider PRD §9 open questions.
type sessionTokenStore struct {
	mu       sync.Mutex
	locks    map[string]*sync.Mutex
	inMemory map[string]ssoTokenCache
}

// defaultSessionStore is the process-wide instance. Tests that need isolation can
// construct their own via newSessionTokenStore() and inject it.
var defaultSessionStore = newSessionTokenStore()

// newSessionTokenStore constructs an empty store. Exposed for tests.
func newSessionTokenStore() *sessionTokenStore {
	return &sessionTokenStore{
		locks:    make(map[string]*sync.Mutex),
		inMemory: make(map[string]ssoTokenCache),
	}
}

// Acquire returns the mutex associated with the given session key, creating one on
// first use. Callers should defer Unlock to ensure release on all return paths.
//
// The intended usage pattern is:
//
//	mu := store.Acquire(sessionKey)
//	mu.Lock()
//	defer mu.Unlock()
//	// double-check the in-memory cache here, since another goroutine may have just
//	// completed authentication for this session while we were waiting on the lock.
func (s *sessionTokenStore) Acquire(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()

	mu, ok := s.locks[key]
	if !ok {
		mu = &sync.Mutex{}
		s.locks[key] = mu
	}
	return mu
}

// Get returns the in-memory cached token for the given session key, plus a bool
// indicating whether the entry exists AND is non-expired (5-minute buffer applied,
// matching the on-disk cache validation in loadCachedToken).
func (s *sessionTokenStore) Get(key string) (ssoTokenCache, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cached, ok := s.inMemory[key]
	if !ok {
		return ssoTokenCache{}, false
	}
	if time.Now().Add(5 * time.Minute).After(cached.ExpiresAt) {
		return ssoTokenCache{}, false
	}
	return cached, true
}

// Put stores a token in the in-memory cache for the given session key. Callers
// should also persist to disk via saveCachedToken so the token survives process
// restart.
func (s *sessionTokenStore) Put(key string, token ssoTokenCache) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.inMemory[key] = token
}

// Forget drops the in-memory token for the given key so a subsequent login
// re-authenticates instead of reusing a logged-out session.
//
// The per-session mutex in s.locks is intentionally retained. Deleting it would
// break single-flight: if a logout overlaps an in-flight device-auth flow that
// holds the mutex, a later Acquire(key) would mint a brand-new mutex and the next
// login would no longer serialize against the in-flight flow — producing two
// device-auth flows for the same portal. The mutex is cheap and bounded by the
// number of distinct portals, so leaking it is harmless.
func (s *sessionTokenStore) Forget(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.inMemory, key)
}
