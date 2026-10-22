package install

import (
	"errors"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
)

// RepairContext explains why a corrupt cached release is not repaired automatically.
type RepairContext int

const (
	// RepairAvailable means an online, non-frozen install repairs the release.
	RepairAvailable RepairContext = iota
	// RepairFrozen means the caller must not mutate the cache or lock.
	RepairFrozen
	// RepairOffline means repair needs registry access that policy forbids.
	RepairOffline
	// RepairAutomatic means an ordinary command found the corruption and has not repaired it.
	RepairAutomatic
)

// ReleaseDirectory returns the cache directory that holds a locked release for a platform.
func (c *Cache) ReleaseDirectory(entry *pluginlock.Entry, platform string) (string, error) {
	defer perf.Track(nil, "plugininstall.Cache.ReleaseDirectory")()

	return c.location(entry.IndexDigest, platform)
}

// CorruptReleaseError names the alias, the cache path, and the exact command that repairs
// a corrupt cached release. The returned error satisfies ErrPluginCacheCorrupt and keeps
// every cause (including ErrOCIIntegrity) in its chain.
func (c *Cache) CorruptReleaseError(cause error, alias string, entry *pluginlock.Entry, platform string, repair RepairContext) error {
	defer perf.Track(nil, "plugininstall.Cache.CorruptReleaseError")()

	// Only the sentinels stay in the chain: file-level causes such as os.ErrNotExist
	// must not make callers treat a corrupt release as merely absent and skip it.
	detail := strings.ReplaceAll(cause.Error(), "\n", "; ")
	cause = errors.Join(errUtils.ErrPluginCacheCorrupt, errUtils.ErrOCIIntegrity)
	if alias == "" {
		alias = entry.Plugin
	}
	directory, err := c.location(entry.IndexDigest, platform)
	if err != nil {
		directory = c.Directory
	}
	builder := errUtils.Build(cause).
		WithExplanationf("The cached release of plugin `%s` (%s, locked digest `%s`) no longer matches its verified contents: %s.", alias, entry.Version, entry.IndexDigest, detail).
		WithHintf("Plugin `%s` is cached at `%s`; a file there was modified, added, or removed after it was verified.", alias, directory)
	switch repair {
	case RepairFrozen:
		builder.WithHintf("`--frozen` never changes the cache. Run `atmos plugin install` without `--frozen` to quarantine the corrupt release and re-download it by its locked digest.")
	case RepairOffline:
		builder.WithHintf("Offline policy blocks the re-download. Run `atmos plugin install` while online (or with `--from-layout <dir>`) to quarantine the corrupt release and re-download plugin `%s` by its locked digest.", alias)
	default:
		builder.WithHintf("Run `atmos plugin install` to quarantine the corrupt release and re-download plugin `%s` by its locked digest.", alias)
	}
	return builder.Err()
}
