// Package plugin centralizes Terraform provider plugin-cache policy shared by
// the normal Terraform command pipeline and internal output lookups.
package plugin

import (
	"encoding/hex"
	"hash/fnv"
	"os"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// CacheDirEnv is Terraform's provider plugin-cache environment variable.
	CacheDirEnv = "TF_PLUGIN_CACHE_DIR"
	// CacheMayBreakLockFileEnv allows Terraform to reuse an Atmos-managed cache
	// before a fully portable dependency lock entry exists.
	CacheMayBreakLockFileEnv = "TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE"
)

var (
	getXDGCacheDir = xdg.GetXDGCacheDir
	absolutePath   = filepath.Abs
)

// Cache describes the effective provider plugin cache for one Terraform subprocess.
// Environment is populated only when Atmos selected the cache automatically; callers
// retain complete control over an explicit TF_PLUGIN_CACHE_DIR override.
type Cache struct {
	Directory   string
	Environment map[string]string
	Automatic   bool
}

// Resolve applies Atmos's provider-cache policy. An explicit cache directory takes
// precedence over the configured default. Empty and root-directory overrides are
// rejected to preserve the existing Atmos safety behavior.
func Resolve(atmosConfig *schema.AtmosConfiguration, override string, overrideSet bool) Cache {
	defer perf.Track(atmosConfig, "plugin.Resolve")()

	if overrideSet && IsValidDirectory(override, "environment variable") {
		return Cache{Directory: override}
	}

	if atmosConfig == nil || !atmosConfig.Components.Terraform.PluginCache {
		return Cache{}
	}

	directory := atmosConfig.Components.Terraform.PluginCacheDir
	if directory != "" && !IsValidDirectory(directory, "components.terraform.plugin_cache_dir") {
		return Cache{}
	}
	if directory == "" {
		cacheDir, err := getXDGCacheDir("terraform/plugins", xdg.DefaultCacheDirPerm)
		if err != nil {
			log.Warn("Failed to create plugin cache directory", "error", err)
			return Cache{}
		}
		directory = cacheDir
	}

	if directory == "" {
		return Cache{}
	}

	return Cache{
		Directory: directory,
		Automatic: true,
		Environment: map[string]string{
			CacheDirEnv:              directory,
			CacheMayBreakLockFileEnv: "true",
		},
	}
}

// IsValidDirectory reports whether path is a safe, absolute provider-cache directory.
// Relative paths are rejected because they would resolve against whichever
// working directory happens to be current, producing a different effective
// cache location per component instead of one shared cache.
func IsValidDirectory(path, source string) bool {
	defer perf.Track(nil, "plugin.IsValidDirectory")()

	if path == "" {
		log.Warn("TF_PLUGIN_CACHE_DIR is empty, ignoring and using Atmos default", "source", source)
		return false
	}
	if path == "/" {
		log.Warn("TF_PLUGIN_CACHE_DIR is set to root '/', ignoring and using Atmos default", "source", source)
		return false
	}
	if !filepath.IsAbs(path) {
		log.Warn("Provider plugin cache directory must be an absolute path, ignoring and using Atmos default", "source", source, "path", path)
		return false
	}
	return true
}

// InitLockPath returns a stable, machine-local lock path for serializing
// Terraform init calls that share this cache. Relative cache directories are
// resolved from the current process directory.
func (c Cache) InitLockPath() string {
	defer perf.Track(nil, "plugin.Cache.InitLockPath")()
	return c.InitLockPathForWorkdir("")
}

// InitLockPathForWorkdir returns a stable, machine-local lock path for a
// Terraform working directory. It intentionally lives outside the cache and
// working directories so it never becomes provider-cache data or a repository
// artifact.
func (c Cache) InitLockPathForWorkdir(workdir string) string {
	defer perf.Track(nil, "plugin.Cache.InitLockPathForWorkdir")()

	if c.Directory == "" {
		return ""
	}
	directory := c.Directory
	if workdir != "" && !filepath.IsAbs(directory) {
		directory = filepath.Join(workdir, directory)
	}
	abs, err := absolutePath(directory)
	if err != nil {
		abs = directory
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(abs))
	return filepath.Join(os.TempDir(), "atmos-plugin-cache-init-"+hex.EncodeToString(h.Sum(nil)))
}
