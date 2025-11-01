package aws

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	ssoTokenCacheSubdir    = "aws-sso"
	ssoTokenCacheFilename  = "token.json"
	ssoTokenCacheDirPerms  = 0o700
	ssoTokenCacheFilePerms = 0o600
)

// ssoTokenCache represents a cached SSO access token.
type ssoTokenCache struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
	Region      string    `json:"region"`
	StartURL    string    `json:"startUrl"`
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

// getTokenCachePath returns the XDG-compliant cache path for SSO token.
// Path format: ~/.cache/atmos/aws-sso/<provider-name>/token.json.
func (p *ssoProvider) getTokenCachePath() (string, error) {
	cacheDir, err := p.cacheStorage.GetXDGCacheDir(ssoTokenCacheSubdir, ssoTokenCacheDirPerms)
	if err != nil {
		return "", fmt.Errorf("failed to get XDG cache directory: %w", err)
	}

	// Create provider-specific subdirectory.
	providerCacheDir := filepath.Join(cacheDir, p.name)
	if err := p.cacheStorage.MkdirAll(providerCacheDir, ssoTokenCacheDirPerms); err != nil {
		return "", fmt.Errorf("failed to create provider cache directory: %w", err)
	}

	return filepath.Join(providerCacheDir, ssoTokenCacheFilename), nil
}

// loadCachedToken loads and validates a cached SSO token.
// Returns the token and expiration if valid, or empty values if cache miss or expired.
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
