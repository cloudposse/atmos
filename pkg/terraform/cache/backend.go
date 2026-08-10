package cache

import (
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// cacheSubpath is the XDG cache subdirectory for the registry cache. It is the
// well-known location the Atmos Native CI cache persists and restores.
const cacheSubpath = "terraform/registry"

// layoutDirs are the top-level directories ensured under the cache root.
var layoutDirs = []string{"providers", "modules", "metadata", "objects", "locks"}

// resolveRoot resolves the cache root: the configured location, else the XDG cache
// directory under terraform/registry.
func resolveRoot(cfg *schema.TerraformCacheConfig) (string, error) {
	defer perf.Track(nil, "tfcache.resolveRoot")()

	if cfg.Location != "" {
		expanded, err := homedir.Expand(cfg.Location)
		if err != nil {
			return "", fmt.Errorf("%w: expanding cache location: %w", errUtils.ErrInvalidConfig, err)
		}
		return expanded, nil
	}

	root, err := xdg.GetXDGCacheDir(cacheSubpath, xdg.DefaultCacheDirPerm)
	if err != nil {
		return "", fmt.Errorf("%w: resolving cache root: %w", errUtils.ErrInvalidConfig, err)
	}
	return root, nil
}

// ensureLayout creates the cache root and its standard subdirectories.
func ensureLayout(root string) error {
	defer perf.Track(nil, "tfcache.ensureLayout")()

	for _, dir := range layoutDirs {
		if err := os.MkdirAll(filepath.Join(root, dir), xdg.DefaultCacheDirPerm); err != nil {
			return fmt.Errorf("%w: creating cache layout %q: %w", errUtils.ErrInvalidConfig, dir, err)
		}
	}
	return nil
}
