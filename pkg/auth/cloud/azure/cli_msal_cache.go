package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/perf"
)

// msalExtensionsLockSuffix is the suffix msal-extensions (the persistence layer the
// Azure CLI uses for ~/.azure/msal_token_cache.json) appends to the cache path for
// its cross-process lock file. Locking the same file makes Atmos and `az` exclude
// each other while either one rewrites the shared cache.
const msalExtensionsLockSuffix = ".lockfile"

// AzureCLIMSALCacheLockPath returns the lock file path msal-extensions uses for the
// given Azure CLI MSAL cache file.
func AzureCLIMSALCacheLockPath(cachePath string) string {
	return cachePath + msalExtensionsLockSuffix
}

// withMSALCacheFileLock runs fn while holding the cross-process locks every writer
// of an MSAL token cache file may take:
//
//   - `<cache>.lockfile`, the msal-extensions lock that az uses, so Atmos and az
//     exclude each other.
//   - `<cache>.lock`, the lock older Atmos releases used, so a newer and an older
//     Atmos running at the same time do not interleave a read and a write either.
//
// The locks are always taken in this order, and az and older Atmos each take only
// one of them, so no lock-order cycle (deadlock) is possible. Windows FileLock is a
// no-op. A lock that cannot be acquired within fileLockTimeout (or before ctx is
// done) returns ErrFileLockTimeout.
func withMSALCacheFileLock(ctx context.Context, cachePath string, fn func() error) error {
	lockCtx, cancel := context.WithTimeout(ctx, fileLockTimeout)
	defer cancel()

	err := cache.NewFileLockAtPath(AzureCLIMSALCacheLockPath(cachePath)).WithLockContext(lockCtx, func() error {
		// cache.NewFileLock locks cachePath + ".lock", the legacy Atmos lock name.
		return cache.NewFileLock(cachePath).WithLockContext(lockCtx, fn)
	})
	if errors.Is(err, errUtils.ErrCacheLocked) {
		return fmt.Errorf("%w: %w", ErrFileLockTimeout, err)
	}
	return err
}

// UpdateAzureCLIMSALCache performs a read-modify-write of the Azure CLI MSAL token
// cache (normally ~/.azure/msal_token_cache.json) while holding the
// msal-extensions-compatible lock (and the legacy Atmos lock, see
// withMSALCacheFileLock) for the whole update, so a concurrent `az` (or another
// Atmos process) cannot interleave its own write between the read and the write.
//
// The file is co-owned with the user's own `az login` session, so it is never
// reset: a UTF-8 BOM is stripped, a missing or empty file starts an empty cache,
// and any other unparseable content returns ErrAzureCLIMSALCacheUnparseable and
// leaves the file untouched. The mutate callback receives a non-nil map and edits
// it in place; returning an error aborts the update without writing.
func UpdateAzureCLIMSALCache(cachePath string, mutate func(msalCache map[string]interface{}) error) error {
	defer perf.Track(nil, "azure.UpdateAzureCLIMSALCache")()

	if err := os.MkdirAll(filepath.Dir(cachePath), DirPermissions); err != nil {
		return fmt.Errorf("%w: failed to create %s: %w", errUtils.ErrAzureCLIMSALCacheUpdate, filepath.Dir(cachePath), err)
	}

	return withMSALCacheFileLock(context.Background(), cachePath, func() error {
		msalCache, err := loadMSALCache(cachePath)
		if err != nil {
			return err
		}
		if err := mutate(msalCache); err != nil {
			return err
		}
		data, err := json.MarshalIndent(msalCache, "", "  ")
		if err != nil {
			return fmt.Errorf("%w: failed to marshal: %w", errUtils.ErrAzureCLIMSALCacheUpdate, err)
		}
		if err := credentialFileSystem.WriteFileAtomic(cachePath, data, FilePermissions); err != nil {
			return fmt.Errorf("%w: failed to write %s: %w", errUtils.ErrAzureCLIMSALCacheUpdate, cachePath, err)
		}
		return nil
	})
}

// loadMSALCache loads the Azure CLI MSAL cache for an update. A missing,
// empty, or whitespace-only file yields an empty cache (there is nothing to lose),
// matching MSAL's own deserialization of an empty state.
func loadMSALCache(cachePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), nil
		}
		return nil, fmt.Errorf("%w: failed to read %s: %w", errUtils.ErrAzureCLIMSALCacheUpdate, cachePath, err)
	}

	data = bytes.TrimSpace(stripBOM(data))
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}

	var msalCache map[string]interface{}
	if err := json.Unmarshal(data, &msalCache); err != nil {
		return nil, fmt.Errorf("%w: %s (left unchanged): %w", errUtils.ErrAzureCLIMSALCacheUnparseable, cachePath, err)
	}
	if msalCache == nil {
		// The file held a JSON null.
		msalCache = make(map[string]interface{})
	}
	return msalCache, nil
}
