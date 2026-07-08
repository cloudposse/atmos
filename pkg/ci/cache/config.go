package cache

import (
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// DefaultKeyPrefix namespaces auto-derived cache keys.
	defaultKeyPrefix = "atmos-cache-"

	// CompressionGzip is the only supported compression today.
	compressionGzip = "gzip"

	// CacheRootDirPerm is the permission for the well-known cache root.
	cacheRootDirPerm = 0o755

	// ToolchainLockFilename is the default toolchain lockfile name used for
	// deriving the default cache key.
	toolchainLockFilename = "toolchain.lock.yaml"

	// Auto modes for ci.cache.auto.
	autoOff     = "off"
	autoRestore = "restore"
	autoSave    = "save"
	autoBoth    = "both"
)

// Config is the fully-resolved cache configuration used by the manager. It is
// derived from schema.CICacheConfig plus computed defaults, so both the CLI
// commands and the lifecycle hooks operate on identical inputs.
type Config struct {
	// Enabled is the master switch (ci.cache.enabled).
	Enabled bool

	// Auto is the automatic mode: off | restore | save | both.
	Auto string

	// Root is the absolute well-known cache directory that is archived.
	Root string

	// Includes are root-relative subpaths to include; empty means the whole root.
	Includes []string

	// Key is the resolved exact cache key.
	Key string

	// RestoreKeys are prefix fallbacks tried (in order) when Key is absent.
	RestoreKeys []string

	// Compression is the archive compression (currently always gzip).
	Compression string

	// AllowUnsafeAuthCache opts out of the default exclusion of Atmos's own
	// auth session-cache subdirectories from the archive (see
	// DefaultExcludedPaths).
	AllowUnsafeAuthCache bool
}

// CacheRoot returns the absolute path of the well-known cache root that the CI
// cache archives. It defaults to the Atmos XDG cache directory (~/.cache/atmos)
// and is overridable via ATMOS_XDG_CACHE_HOME / XDG_CACHE_HOME. The toolchain
// install path is a sub-path of this root, so caching the root captures it.
func CacheRoot() (string, error) {
	defer perf.Track(nil, "cache.CacheRoot")()

	root, err := xdg.GetXDGCacheDir("", cacheRootDirPerm)
	if err != nil {
		return "", err
	}
	return root, nil
}

// ResolveConfig builds a Config from the Atmos configuration, filling in
// defaults for any unset fields. It does not require the cache to be enabled;
// callers gate on Config.Enabled.
func ResolveConfig(atmosConfig *schema.AtmosConfiguration) (*Config, error) {
	defer perf.Track(atmosConfig, "cache.ResolveConfig")()

	cc := schema.CICacheConfig{}
	if atmosConfig != nil {
		cc = atmosConfig.CI.Cache
	}

	root, err := resolveRoot(&cc)
	if err != nil {
		return nil, err
	}

	compression := cc.Compression
	if compression == "" {
		compression = compressionGzip
	}

	auto := cc.Auto
	if auto == "" {
		auto = autoOff
	}

	cfg := &Config{
		Enabled:              cc.Enabled,
		Auto:                 auto,
		Root:                 root,
		Includes:             normalizeIncludes(cc.Paths),
		Compression:          compression,
		AllowUnsafeAuthCache: cc.AllowUnsafeAuthCache,
	}

	if err := resolveKeys(cfg, &cc, root, atmosConfig); err != nil {
		return nil, err
	}

	return cfg, nil
}

// resolveRoot returns the absolute cache root, honoring an explicit override.
func resolveRoot(cc *schema.CICacheConfig) (string, error) {
	if cc.Root != "" {
		abs, err := filepath.Abs(cc.Root)
		if err != nil {
			return "", err
		}
		if mkErr := os.MkdirAll(abs, cacheRootDirPerm); mkErr != nil {
			return "", mkErr
		}
		return abs, nil
	}
	return CacheRoot()
}

// resolveKeys fills cfg.Key and cfg.RestoreKeys, rendering any template and
// applying defaults derived from the toolchain lockfile.
func resolveKeys(cfg *Config, cc *schema.CICacheConfig, root string, atmosConfig *schema.AtmosConfiguration) error {
	if cc.Key != "" {
		key, err := renderKey(cc.Key, keyBaseDir(atmosConfig, root))
		if err != nil {
			return err
		}
		cfg.Key = key
	} else {
		cfg.Key = defaultKey(defaultLockfilePath(atmosConfig, root))
	}

	if len(cc.RestoreKeys) > 0 {
		cfg.RestoreKeys = renderRestoreKeys(cc.RestoreKeys, keyBaseDir(atmosConfig, root))
	} else if cc.Key == "" {
		// Only auto-add the default restore key when using the default key.
		cfg.RestoreKeys = []string{defaultRestoreKey()}
	}
	return nil
}

// renderRestoreKeys renders each restore-key template, skipping any that fail.
func renderRestoreKeys(templates []string, baseDir string) []string {
	out := make([]string, 0, len(templates))
	for _, tmpl := range templates {
		if key, err := renderKey(tmpl, baseDir); err == nil && key != "" {
			out = append(out, key)
		}
	}
	return out
}

// keyBaseDir returns the directory used to resolve hashFiles() globs in key
// templates. It is the current working directory so users can hash repo files
// (e.g. ".tool-versions"); falls back to the cache root.
func keyBaseDir(_ *schema.AtmosConfiguration, root string) string {
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return root
}

// defaultLockfilePath returns the absolute path to the toolchain lockfile used
// for default key derivation. It honors an explicit toolchain.lock_file config,
// otherwise looks under the cache root's toolchain sub-path.
func defaultLockfilePath(atmosConfig *schema.AtmosConfiguration, root string) string {
	if atmosConfig != nil && atmosConfig.Toolchain.LockFile != "" {
		abs, err := filepath.Abs(atmosConfig.Toolchain.LockFile)
		if err == nil {
			return abs
		}
		return atmosConfig.Toolchain.LockFile
	}
	return filepath.Join(root, "toolchain", toolchainLockFilename)
}

// normalizeIncludes cleans include paths and drops empties.
func normalizeIncludes(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = filepath.Clean(p)
		if p == "" || p == "." {
			continue
		}
		out = append(out, p)
	}
	return out
}

// AutoRestoreEnabled reports whether automatic restore-on-start is configured.
func (c *Config) AutoRestoreEnabled() bool {
	defer perf.Track(nil, "cache.Config.AutoRestoreEnabled")()

	return c.Enabled && (c.Auto == autoRestore || c.Auto == autoBoth)
}

// AutoSaveEnabled reports whether automatic save-on-end is configured.
func (c *Config) AutoSaveEnabled() bool {
	defer perf.Track(nil, "cache.Config.AutoSaveEnabled")()

	return c.Enabled && (c.Auto == autoSave || c.Auto == autoBoth)
}

// validate ensures the resolved config is usable.
func (c *Config) validate() error {
	if c.Key == "" {
		return errUtils.ErrCacheKeyRequired
	}
	if c.Root == "" {
		return errUtils.ErrCacheInvalidArgs
	}
	return nil
}
