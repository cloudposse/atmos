package azure

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// msalCache implements cache.ExportReplace for MSAL token cache.
// It stores tokens in ~/.azure/msal_token_cache.json for compatibility with Azure CLI.
type msalCache struct {
	cachePath string
}

// NewMSALCache creates a new MSAL cache instance.
// If cachePath is empty, uses the default Azure CLI location (~/.azure/msal_token_cache.json).
func NewMSALCache(cachePath string) (cache.ExportReplace, error) {
	if cachePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		cachePath = filepath.Join(homeDir, ".azure", "msal_token_cache.json")
	}

	// Ensure cache directory exists.
	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, DirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &msalCache{
		cachePath: cachePath,
	}, nil
}

// Replace loads the cache from disk into memory.
func (c *msalCache) Replace(ctx context.Context, u cache.Unmarshaler, hints cache.ReplaceHints) error {
	// Check context cancellation.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Acquire file lock for reading to prevent race with concurrent writes.
	lockPath := c.cachePath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock MSAL cache file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	// Read cache file.
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("MSAL cache file does not exist, starting with empty cache", "path", c.cachePath)
			return nil // Empty cache is OK.
		}
		return fmt.Errorf("failed to read MSAL cache: %w", err)
	}

	// Unmarshal into MSAL's internal format.
	if err := u.Unmarshal(data); err != nil {
		log.Debug("Failed to unmarshal MSAL cache, starting fresh", "error", err)
		return nil // Corrupted cache is OK, start fresh.
	}

	log.Debug("Loaded MSAL cache from disk", "path", c.cachePath, "size", len(data))
	return nil
}

// Export writes the cache from memory to disk.
func (c *msalCache) Export(ctx context.Context, m cache.Marshaler, hints cache.ExportHints) error {
	// Check context cancellation.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Marshal MSAL's internal format.
	data, err := m.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal MSAL cache: %w", err)
	}

	// Acquire file lock to prevent concurrent writes.
	lockPath := c.cachePath + ".lock"
	lock, err := AcquireFileLock(lockPath)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			log.Debug("Failed to unlock MSAL cache file", "lock_file", lockPath, "error", unlockErr)
		}
	}()

	// Write to disk with secure permissions.
	if err := os.WriteFile(c.cachePath, data, FilePermissions); err != nil {
		return fmt.Errorf("failed to write MSAL cache: %w", err)
	}

	log.Debug("Exported MSAL cache to disk", "path", c.cachePath, "size", len(data))
	return nil
}

// GetCachePath returns the path to the MSAL cache file.
func (c *msalCache) GetCachePath() string {
	return c.cachePath
}
