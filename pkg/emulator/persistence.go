package emulator

import (
	"os"
	"path/filepath"
	"runtime"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/container"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// persistenceCacheSubdir is the XDG cache subdirectory (under the "atmos" root)
// that holds per-instance emulator state directories.
const persistenceCacheSubdir = "emulator"

// instanceCacheSubpath builds the XDG cache subpath for an emulator instance,
// reusing the canonical sanitized runtime name so the directory is flat,
// filesystem-safe, and collision-free across instances (e.g.
// "emulator/atmos-dev-emulator-aws").
func instanceCacheSubpath(stack, name string) string {
	defer perf.Track(nil, "emulator.instanceCacheSubpath")()

	return filepath.Join(persistenceCacheSubdir, container.RuntimeName(stack, cfg.EmulatorComponentType, name))
}

// InstanceDataDir returns (creating it if needed) the host directory that backs
// an emulator instance's persisted state, under the XDG cache. It honors
// ATMOS_XDG_CACHE_HOME / XDG_CACHE_HOME. The returned directory is bind-mounted
// onto the driver's in-container data dir when persistence is enabled.
func InstanceDataDir(stack, name string) (string, error) {
	defer perf.Track(nil, "emulator.InstanceDataDir")()

	if cacheHome, ok := emulatorCacheHomeForBindMounts(); ok {
		fullPath := filepath.Join(cacheHome, "atmos", instanceCacheSubpath(stack, name))
		if err := os.MkdirAll(fullPath, xdg.DefaultCacheDirPerm); err != nil {
			return "", err
		}
		return fullPath, nil
	}
	return xdg.GetXDGCacheDir(instanceCacheSubpath(stack, name), xdg.DefaultCacheDirPerm)
}

// LookupInstanceDataDir resolves the host directory backing an emulator
// instance's persisted state WITHOUT creating it. Used by `reset`, which removes
// the directory.
func LookupInstanceDataDir(stack, name string) string {
	defer perf.Track(nil, "emulator.LookupInstanceDataDir")()

	if cacheHome, ok := emulatorCacheHomeForBindMounts(); ok {
		return filepath.Join(cacheHome, "atmos", instanceCacheSubpath(stack, name))
	}
	return xdg.LookupXDGCacheDir(instanceCacheSubpath(stack, name))
}

func emulatorCacheHomeForBindMounts() (string, bool) {
	if value, _ := xdg.LookupCacheHomeBase(); value != "" {
		return "", false
	}
	home, err := homedir.Dir()
	if err != nil {
		return "", false
	}
	return defaultEmulatorCacheHomeForBindMounts(runtime.GOOS, home)
}

func defaultEmulatorCacheHomeForBindMounts(goos, home string) (string, bool) {
	if goos != "darwin" || home == "" {
		return "", false
	}
	return filepath.Join(home, "Library", "Caches"), true
}
