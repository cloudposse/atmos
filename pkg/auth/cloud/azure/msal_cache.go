package azure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/cache"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// msalCache implements cache.ExportReplace for MSAL token cache.
// It stores tokens in ~/.azure/msal_token_cache.json for compatibility with Azure CLI.
//
// The file can be shared with other processes: for the empty realm it is the Azure
// CLI's own cache, and several Atmos processes may use one realm cache. So the
// file is never reset or overwritten wholesale. Replace and Export each run under
// the msal-extensions lock (see withMSALCacheFileLock), and Export is a
// read-modify-write that applies only the entries MSAL changed since Replace.
type msalCache struct {
	cachePath string

	// mu guards base. MSAL may run several Replace calls concurrently.
	mu sync.Mutex
	// base is the state MSAL loaded in the last Replace (or wrote in the last
	// Export). Nil means MSAL holds nothing loaded from disk.
	base msalCacheState
}

// resolveMSALCachePath returns the on-disk MSAL token cache path for a realm.
// An empty realm resolves to the shared Azure CLI cache (~/.azure/msal_token_cache.json);
// a non-empty realm resolves to the isolated Atmos cache
// (~/.azure/atmos/{realm}/msal_token_cache.json). This is the single source of truth for
// the path shared by NewMSALCache and RemoveMSALCache.
func resolveMSALCachePath(realm string) (string, error) {
	// os.UserHomeDir (not homedir.Dir) intentionally: every sibling realm-cache resolver in
	// this package (device_code_cache.go, setup.go, refresh_token.go) uses it, and the realm
	// cache path must stay in lockstep with those writers. homedir.Dir's process-wide cache
	// could resolve a different home than the live-$HOME writers and would break the package's
	// $HOME-swapping test isolation.
	homeDir, err := os.UserHomeDir() //nolint:forbidigo // See comment above: package-consistency and cache-desync avoidance.
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	// Include realm in path for credential isolation.
	if realm != "" {
		return filepath.Join(homeDir, ".azure", "atmos", realm, "msal_token_cache.json"), nil
	}
	return filepath.Join(homeDir, ".azure", "msal_token_cache.json"), nil
}

// NewMSALCache creates a new MSAL cache instance.
// If cachePath is empty, uses the default Azure CLI location (~/.azure/atmos/{realm}/msal_token_cache.json).
// The realm parameter provides credential isolation between different repositories.
func NewMSALCache(cachePath string, realm string) (cache.ExportReplace, error) {
	if cachePath == "" {
		resolved, err := resolveMSALCachePath(realm)
		if err != nil {
			return nil, err
		}
		cachePath = resolved
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

// RemoveMSALCache deletes the realm-scoped MSAL token cache so a subsequent login cannot
// silently reuse the previous session's cached account and refresh token. Without this, a
// role or PIM change never takes effect after `atmos auth login` because Authenticate()
// tries a silent MSAL acquisition from this cache first and gets back the stale token —
// producing persistent 403s until the file is removed by hand. A missing file is not an
// error (the steady state after a prior logout).
//
// It refuses to touch the shared Azure CLI cache (~/.azure/msal_token_cache.json, the
// empty-realm path): that file is co-owned with a user's own `az login` session, and
// clearing it on an Atmos logout would sign the user out of az as well.
func RemoveMSALCache(realm string) error {
	defer perf.Track(nil, "azure.RemoveMSALCache")()

	if realm == "" {
		log.Debug("Skipping MSAL cache removal for empty realm (shared Azure CLI cache is co-owned with `az login`)")
		return nil
	}

	cachePath, err := resolveMSALCachePath(realm)
	if err != nil {
		return err
	}

	if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove MSAL token cache: %w", err)
	}

	log.Debug("Removed MSAL token cache", "path", cachePath, "realm", realm)
	return nil
}

// Replace loads the cache from disk into memory.
//
// A UTF-8 BOM is stripped first. A cache that is not a JSON object, or that MSAL
// cannot load, is not loaded: authentication continues without cached tokens (and
// a warning), and Export will not overwrite the file.
func (c *msalCache) Replace(ctx context.Context, u cache.Unmarshaler, hints cache.ReplaceHints) error {
	// Check context cancellation.
	if err := ctx.Err(); err != nil {
		return err
	}

	return withMSALCacheFileLock(ctx, c.cachePath, func() error {
		data, err := os.ReadFile(c.cachePath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Debug("MSAL cache file does not exist, starting with empty cache", "path", c.cachePath)
				c.setBase(nil)
				return nil
			}
			return fmt.Errorf("failed to read MSAL cache: %w", err)
		}

		data = normalizeMSALCacheBytes(data)
		state, err := decodeMSALCacheState(data)
		if err != nil {
			c.setBase(nil)
			log.Warn("Ignoring an MSAL token cache that is not valid JSON; tokens will not be cached until the file is repaired or removed",
				"path", c.cachePath, "error", err)
			return nil
		}
		if len(data) == 0 {
			c.setBase(nil)
			return nil
		}
		if err := u.Unmarshal(data); err != nil {
			// MSAL starts from an empty cache; Export still keeps every entry on disk.
			c.setBase(nil)
			log.Warn("MSAL could not load the token cache; continuing without cached tokens", "path", c.cachePath, "error", err)
			return nil
		}
		c.setBase(state)
		log.Debug("Loaded MSAL cache from disk", "path", c.cachePath, "size", len(data))
		return nil
	})
}

// Export writes the cache from memory to disk.
//
// It re-reads the file under the lock and writes it back with only the entries
// MSAL changed since Replace (see mergeMSALCacheState), atomically. If the file on
// disk is not valid JSON, nothing is written and a warning is logged, so a damaged
// cache (which may be the user's own `az login` cache) is never replaced.
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
	ours, err := decodeMSALCacheState(normalizeMSALCacheBytes(data))
	if err != nil {
		return fmt.Errorf("failed to marshal MSAL cache: %w", err)
	}

	return withMSALCacheFileLock(ctx, c.cachePath, func() error {
		current, err := readMSALCacheState(c.cachePath)
		if errors.Is(err, errUtils.ErrAzureMSALCacheUnparseable) {
			log.Warn("Not saving tokens to an MSAL token cache that is not valid JSON; repair or remove the file", "path", c.cachePath, "error", err)
			return nil
		}
		if err != nil {
			return err
		}

		merged, err := json.MarshalIndent(mergeMSALCacheState(current, c.getBase(), ours), "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal MSAL cache: %w", err)
		}
		if err := writeSharedMSALCache(c.cachePath, merged); err != nil {
			return fmt.Errorf("failed to write MSAL cache: %w", err)
		}
		// MSAL's in-memory state now matches what was just persisted for its entries.
		c.setBase(ours)
		log.Debug("Exported MSAL cache to disk", "path", c.cachePath, "size", len(merged))
		return nil
	})
}

// setBase records the state MSAL holds from disk.
func (c *msalCache) setBase(state msalCacheState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = state
}

// getBase returns the state MSAL holds from disk.
func (c *msalCache) getBase() msalCacheState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base
}

// GetCachePath returns the path to the MSAL cache file.
func (c *msalCache) GetCachePath() string {
	return c.cachePath
}
