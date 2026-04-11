package aws

// Refresh token cache (XDG file-based, following SSO cache pattern).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// getRefreshCachePath returns the XDG-compliant cache path for the refresh token.
func (i *userIdentity) getRefreshCachePath() (string, error) {
	cacheDir, err := xdg.GetXDGCacheDir(webflowCacheSubdir, webflowCacheDirPerms)
	if err != nil {
		return "", fmt.Errorf("failed to get XDG cache directory: %w", err)
	}

	identityDir := fmt.Sprintf("%s-%s", i.name, i.realm)
	fullDir := filepath.Join(cacheDir, identityDir)
	if err := os.MkdirAll(fullDir, webflowCacheDirPerms); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return filepath.Join(fullDir, webflowCacheFilename), nil
}

// loadRefreshCache loads the cached refresh token.
func (i *userIdentity) loadRefreshCache() (*webflowRefreshCache, error) {
	path, err := i.getRefreshCachePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no cached refresh token: %w", err)
	}

	var cache webflowRefreshCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse refresh cache: %w", err)
	}

	if cache.RefreshToken == "" {
		return nil, errUtils.ErrWebflowEmptyCachedToken
	}

	return &cache, nil
}

// saveRefreshCache saves the refresh token to cache.
func (i *userIdentity) saveRefreshCache(cache *webflowRefreshCache) {
	path, err := i.getRefreshCachePath()
	if err != nil {
		log.Debug("Failed to get refresh cache path", logKeyIdentity, i.name, "error", err)
		return
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		log.Debug("Failed to marshal refresh cache", logKeyIdentity, i.name, "error", err)
		return
	}

	if err := os.WriteFile(path, data, webflowCacheFilePerms); err != nil {
		log.Debug("Failed to write refresh cache", logKeyIdentity, i.name, "error", err)
		return
	}

	log.Debug("Saved webflow refresh token to cache", logKeyIdentity, i.name, "expiresAt", cache.ExpiresAt)
}

// deleteRefreshCache removes the cached refresh token.
func (i *userIdentity) deleteRefreshCache() {
	path, err := i.getRefreshCachePath()
	if err != nil {
		return
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Debug("Failed to delete refresh cache", logKeyIdentity, i.name, "error", err)
	}
}
