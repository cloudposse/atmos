package aws

import (
	"crypto/sha1" //nolint:gosec // SHA1 used for cache keying only, not security; matches AWS SDK's ssocreds layout.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/cachepaths"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// This aliases cachepaths.AWSSSOSubdir, the single source of truth also
	// used by pkg/ci/cache to exclude this directory from CI cache archives.
	ssoTokenCacheSubdir     = cachepaths.AWSSSOSubdir
	ssoTokenCacheSessionDir = "sessions"
	ssoTokenCacheDirPerms   = 0o700
	ssoTokenCacheFilePerms  = 0o600
)

// sessionKey derives a deterministic key for an SSO portal session from the tuple
// (start_url, region). Providers that share both values share a session token —
// this is the mechanism that collapses N atmos providers pointing at the same SSO
// portal into a single browser flow.
//
// SHA1 is used for compatibility with the AWS SDK's ssocreds cache file naming
// convention. It is not used for any security-sensitive purpose.
func sessionKey(startURL, region string) string {
	h := sha1.New() //nolint:gosec // see file-level comment.
	h.Write([]byte(startURL + "|" + region))
	return hex.EncodeToString(h.Sum(nil))
}

// ssoTokenCache represents a cached SSO access token.
// The JSON schema is kept compatible with the AWS SDK's ssocreds cache format so the
// same file can be consumed by a future migration to ssocreds.SSOTokenProvider, and so
// `aws sso login`-style tooling can be opted into via Option C in the PRD.
type ssoTokenCache struct {
	AccessToken           string    `json:"accessToken"`
	ExpiresAt             time.Time `json:"expiresAt"`
	Region                string    `json:"region"`
	StartURL              string    `json:"startUrl"`
	RefreshToken          string    `json:"refreshToken,omitempty"`
	ClientID              string    `json:"clientId,omitempty"`
	ClientSecret          string    `json:"clientSecret,omitempty"`
	RegistrationExpiresAt time.Time `json:"registrationExpiresAt,omitempty"`
}

// CacheStorage defines interface for token cache storage operations.
// This interface enables testing without filesystem dependencies.
type CacheStorage interface {
	// ReadFile reads the cache file at the given path.
	ReadFile(path string) ([]byte, error)
	// WriteFile writes data to the cache file at the given path.
	WriteFile(path string, data []byte, perm os.FileMode) error
	// Remove deletes the cache file at the given path.
	Remove(path string) error
	// MkdirAll creates directory path with permissions.
	MkdirAll(path string, perm os.FileMode) error
	// GetXDGCacheDir returns the XDG cache directory for the given subdirectory.
	GetXDGCacheDir(subdir string, perm os.FileMode) (string, error)
}

// defaultCacheStorage implements CacheStorage using real filesystem operations.
type defaultCacheStorage struct{}

func (d *defaultCacheStorage) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (d *defaultCacheStorage) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (d *defaultCacheStorage) Remove(path string) error {
	return os.Remove(path)
}

func (d *defaultCacheStorage) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (d *defaultCacheStorage) GetXDGCacheDir(subdir string, perm os.FileMode) (string, error) {
	return xdg.GetXDGCacheDir(subdir, perm)
}

// getTokenCachePath returns the XDG-compliant cache path for the SSO token, keyed by
// the SSO session (start_url + region) — not by provider name.
//
// Path format: ~/.cache/atmos/aws-sso/sessions/<sha1(start_url|region)>.json.
//
// Two `aws/iam-identity-center` providers that point at the same SSO portal will hash
// to the same path and share the same on-disk token, eliminating duplicate browser
// flows. Renaming a provider in atmos.yaml no longer invalidates the cached token.
func (p *ssoProvider) getTokenCachePath() (string, error) {
	cacheDir, err := p.cacheStorage.GetXDGCacheDir(ssoTokenCacheSubdir, ssoTokenCacheDirPerms)
	if err != nil {
		return "", fmt.Errorf("failed to get XDG cache directory: %w", err)
	}

	// Create the shared sessions subdirectory.
	sessionsDir := filepath.Join(cacheDir, ssoTokenCacheSessionDir)
	if err := p.cacheStorage.MkdirAll(sessionsDir, ssoTokenCacheDirPerms); err != nil {
		return "", fmt.Errorf("failed to create sessions cache directory: %w", err)
	}

	return filepath.Join(sessionsDir, sessionKey(p.startURL, p.region)+".json"), nil
}

// loadFullCachedToken loads the full session token bundle from disk if it exists,
// is non-expired (with 5-minute buffer), and matches the provider's portal config.
// Returns (token, true) on hit and (zero, false) on any miss reason. This is the
// fast path used during Authenticate() before deciding whether to refresh or run
// device auth.
func (p *ssoProvider) loadFullCachedToken() (ssoTokenCache, bool) {
	cached, ok := p.readCacheFile()
	if !ok {
		return ssoTokenCache{}, false
	}
	if time.Now().Add(5 * time.Minute).After(cached.ExpiresAt) {
		log.Debug("On-disk SSO token expired", "expiresAt", cached.ExpiresAt)
		return ssoTokenCache{}, false
	}
	if cached.Region != p.region || cached.StartURL != p.startURL {
		log.Debug("On-disk token config mismatch", "cachedRegion", cached.Region, "configRegion", p.region)
		return ssoTokenCache{}, false
	}
	return cached, true
}

// loadExpiredCachedTokenForRefresh loads an *expired* cached token specifically for
// the refresh-token path. Returns (token, true) when an expired-but-refresh-capable
// token exists on disk; the caller should attempt tryRefreshToken with it. Returns
// (zero, false) when there's no cache file at all, when the file has no refresh
// token, or when the portal config doesn't match this provider.
func (p *ssoProvider) loadExpiredCachedTokenForRefresh() (ssoTokenCache, bool) {
	cached, ok := p.readCacheFile()
	if !ok {
		return ssoTokenCache{}, false
	}
	if cached.RefreshToken == "" {
		return ssoTokenCache{}, false
	}
	if cached.Region != p.region || cached.StartURL != p.startURL {
		return ssoTokenCache{}, false
	}
	return cached, true
}

// saveFullCachedToken writes a complete token bundle to disk. Failures are logged
// and treated as non-fatal — the in-memory session store still holds the token, so
// the current process keeps working; only persistence across restart is lost.
func (p *ssoProvider) saveFullCachedToken(token ssoTokenCache) error {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		log.Debug("Failed to get token cache path, skipping cache save", "error", err)
		return nil
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		log.Debug("Failed to marshal token cache", "error", err)
		return nil
	}

	if err := p.cacheStorage.WriteFile(tokenPath, data, ssoTokenCacheFilePerms); err != nil {
		log.Debug("Failed to write token cache", "error", err)
		return nil
	}

	log.Debug("Saved SSO token to cache", "path", tokenPath, "expiresAt", token.ExpiresAt)
	return nil
}

// readCacheFile is the shared low-level read used by loadFullCachedToken and
// loadExpiredCachedTokenForRefresh. It returns the parsed token if the file exists
// and is valid JSON; expiry/validity checks are the caller's responsibility.
func (p *ssoProvider) readCacheFile() (ssoTokenCache, bool) {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		log.Debug("Failed to get token cache path", "error", err)
		return ssoTokenCache{}, false
	}

	data, err := p.cacheStorage.ReadFile(tokenPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debug("Failed to read cached token", "error", err)
		}
		return ssoTokenCache{}, false
	}

	var cache ssoTokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		log.Debug("Failed to parse cached token, will re-authenticate", "error", err)
		return ssoTokenCache{}, false
	}

	return cache, true
}

// loadCachedToken loads and validates a cached SSO token.
// Returns the token and expiration if valid, or empty values if cache miss or expired.
//
// Deprecated: prefer loadFullCachedToken which returns the complete cache bundle
// including refresh-token fields. This helper is retained for tests and incremental
// migration.
func (p *ssoProvider) loadCachedToken() (string, time.Time, error) {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, just skip caching.
		log.Debug("Failed to get token cache path, skipping cache check", "error", err)
		return "", time.Time{}, nil
	}

	// Check if cache file exists.
	data, err := p.cacheStorage.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("No cached SSO token found", "path", tokenPath)
			return "", time.Time{}, nil
		}
		log.Debug("Failed to read cached token", "error", err)
		return "", time.Time{}, nil
	}

	// Parse cached token.
	var cache ssoTokenCache
	if err := json.Unmarshal(data, &cache); err != nil {
		log.Debug("Failed to parse cached token, will re-authenticate", "error", err)
		return "", time.Time{}, nil
	}

	// Validate token hasn't expired (with 5 minute buffer).
	if time.Now().Add(5 * time.Minute).After(cache.ExpiresAt) {
		log.Debug("Cached SSO token expired", "expiresAt", cache.ExpiresAt)
		return "", time.Time{}, nil
	}

	// Validate token matches current provider config.
	if cache.Region != p.region || cache.StartURL != p.startURL {
		log.Debug("Cached token config mismatch", "cachedRegion", cache.Region, "configRegion", p.region)
		return "", time.Time{}, nil
	}

	log.Debug("Using cached SSO token", "expiresAt", cache.ExpiresAt)
	return cache.AccessToken, cache.ExpiresAt, nil
}

// saveCachedToken saves an SSO access token to the cache.
func (p *ssoProvider) saveCachedToken(accessToken string, expiresAt time.Time) error {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, just skip caching (non-fatal).
		log.Debug("Failed to get token cache path, skipping cache save", "error", err)
		return nil
	}

	cache := ssoTokenCache{
		AccessToken: accessToken,
		ExpiresAt:   expiresAt,
		Region:      p.region,
		StartURL:    p.startURL,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		log.Debug("Failed to marshal token cache", "error", err)
		return nil // Non-fatal.
	}

	if err := p.cacheStorage.WriteFile(tokenPath, data, ssoTokenCacheFilePerms); err != nil {
		log.Debug("Failed to write token cache", "error", err)
		return nil // Non-fatal.
	}

	log.Debug("Saved SSO token to cache", "path", tokenPath, "expiresAt", expiresAt)
	return nil
}

// deleteCachedToken removes the cached SSO token.
func (p *ssoProvider) deleteCachedToken() error {
	tokenPath, err := p.getTokenCachePath()
	if err != nil {
		// If we can't get cache path, nothing to delete.
		return nil
	}

	if err := p.cacheStorage.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		log.Debug("Failed to delete cached token", "error", err)
		return nil // Non-fatal.
	}

	log.Debug("Deleted cached SSO token", "path", tokenPath)
	return nil
}
