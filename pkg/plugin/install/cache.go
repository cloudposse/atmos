package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginconfig "github.com/cloudposse/atmos/pkg/plugin/config"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	privateDirectoryMode = 0o700
	payloadDirectory     = "payload"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// Cache stores verified release layouts and their materialized native payloads.
type Cache struct{ Directory string }

// NewCache chooses the existing XDG data boundary, with an optional explicit root.
func NewCache(directory string) (*Cache, error) {
	defer perf.Track(nil, "plugininstall.NewCache")()

	if directory == "" {
		var err error
		directory, err = xdg.GetXDGDataDir("plugins", privateDirectoryMode)
		if err != nil {
			return nil, err
		}
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	return &Cache{Directory: absolute}, nil
}

// CurrentPlatform selects the running process's platform, never an emulated fallback.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func CurrentPlatform() string { return pluginplatform.Current() }

func (c *Cache) location(digest, platform string) (string, error) {
	if !digestPattern.MatchString(digest) || !filepath.IsLocal(platform) || strings.Contains(platform, "\\") {
		return "", errUtils.ErrPluginInvalidConfig
	}
	return filepath.Join(c.Directory, "releases", strings.TrimPrefix(digest, "sha256:"), filepath.FromSlash(platform)), nil
}

// Installed checks locked bytes entirely offline before returning an executable directory.
func (c *Cache) Installed(entry *pluginlock.Entry, platform string) (string, error) {
	defer perf.Track(nil, "plugininstall.Cache.Installed")()

	if _, err := c.location(entry.IndexDigest, platform); err != nil {
		return "", err
	}
	platform, err := pluginplatform.Select(entry.Platforms, platform)
	if err != nil {
		return "", err
	}
	directory, err := c.location(entry.IndexDigest, platform)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(directory); err != nil {
		return "", errors.Join(errUtils.ErrPluginNotInstalled, err)
	}
	index, err := atmosoci.LoadIndex(filepath.Join(directory, "oci"), entry.IndexDigest)
	if err != nil {
		return "", errors.Join(errUtils.ErrPluginCacheCorrupt, errUtils.ErrOCIIntegrity, err)
	}
	release, err := pluginoci.Inspect(index, entry.IndexDigest)
	if err != nil {
		return "", errors.Join(errUtils.ErrPluginCacheCorrupt, err)
	}
	if err := pluginlock.CheckRelease(entry, release); err != nil {
		return "", err
	}
	payload := filepath.Join(directory, payloadDirectory)
	if err := release.VerifyInstalled(platform, payload); err != nil {
		return "", err
	}
	return payload, nil
}

// Store stages the chosen platform, validates extensions, then atomically promotes it.
// A partial download never replaces a usable installation.
func (c *Cache) Store(ctx context.Context, release *pluginoci.Release, platform string, descriptors *extension.Registry) (string, error) {
	defer perf.Track(nil, "plugininstall.Cache.Store")()

	if _, err := c.location(release.Digest, platform); err != nil {
		return "", err
	}
	platform, err := pluginplatform.Select(release.Platforms, platform)
	if err != nil {
		return "", err
	}
	directory, err := c.location(release.Digest, platform)
	if err != nil {
		return "", err
	}
	selected := release.Platforms[platform]
	if err := os.MkdirAll(filepath.Dir(directory), privateDirectoryMode); err != nil {
		return "", err
	}
	err = filelock.New(directory+".lock").WithExclusive(ctx, func() error {
		if _, err := os.Stat(directory); err == nil {
			err := c.verifyExisting(release, platform, directory)
			if !errors.Is(err, errUtils.ErrPluginCacheCorrupt) {
				return err
			}
			if err := c.repair(release, directory); err != nil {
				return err
			}
		}
		return stageRelease(ctx, release, &cacheStage{directory: directory, platform: platform, selected: selected.Digest, descriptors: descriptors})
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, payloadDirectory), nil
}

func (c *Cache) verifyExisting(release *pluginoci.Release, platform, directory string) error {
	index, err := atmosoci.LoadIndex(filepath.Join(directory, "oci"), release.Digest)
	if err != nil {
		return errors.Join(errUtils.ErrPluginCacheCorrupt, errUtils.ErrOCIIntegrity, err)
	}
	cached, err := pluginoci.Inspect(index, release.Digest)
	if err != nil {
		return errors.Join(errUtils.ErrPluginCacheCorrupt, err)
	}
	return cached.VerifyInstalled(platform, filepath.Join(directory, payloadDirectory))
}

// repair quarantines a corrupt release so Store can stage verified bytes in its place.
// The caller holds the release lock and has already resolved the release by digest.
func (c *Cache) repair(release *pluginoci.Release, directory string) error {
	moved, err := pluginconfig.Quarantine(directory, time.Now())
	if err != nil {
		return errors.Join(errUtils.ErrPluginCacheCorrupt, err)
	}
	ui.Warningf("Cached plugin release `%s` %s was corrupt; quarantined it at `%s` and re-fetching the verified release.", release.Name, release.Version, moved)
	return nil
}

func stageRelease(ctx context.Context, release *pluginoci.Release, target *cacheStage) error {
	stage, err := os.MkdirTemp(filepath.Dir(target.directory), ".install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := atmosoci.SaveSelection(filepath.Join(stage, "oci"), release.Index, []v1.Hash{target.selected}); err != nil {
		return err
	}
	payload := filepath.Join(stage, payloadDirectory)
	if err := release.Extract(target.platform, payload); err != nil {
		return err
	}
	if _, err := target.descriptors.Validate(payload, release.Definition.Spec.Capabilities); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(stage, target.directory)
}

type cacheStage struct {
	directory, platform string
	selected            v1.Hash
	descriptors         *extension.Registry
}
