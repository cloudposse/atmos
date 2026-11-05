package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

// inCI reports whether the process runs in CI. Tests replace it.
var inCI = telemetry.IsCI

// ensureSelections shares explicit installation's verification and transaction path for packages
// an ordinary command needs (see neededPackages). Existing locks remain immutable; only missing
// declarations acquire locks.
//
// Decision table for each needed package:
//   - locked and cached and intact: nothing to do, no network access.
//   - locked, not cached: restore the exact locked digest (also allowed in CI, because the lock
//     makes the result reproducible).
//   - not locked: install and write the lock, except in CI, which is refused with
//     ErrPluginUnlockedInCI because an unreviewed lock must not be created by a pipeline.
//   - cached release corrupt: treated as missing and restored, unless the process is offline or
//     in CI, in which case the hinted corruption error is returned and the cache is untouched.
//
// Every restoration prints one notice naming the source, digest and signature status.
func ensureSelections(ctx context.Context, atmos *schema.AtmosConfiguration, declarations map[string]schema.PluginPackage, file *pluginlock.File, cache *install.Cache) error {
	pending, unlocked, err := missingSelections(atmos, declarations, file, cache)
	if err != nil || len(pending) == 0 {
		return err
	}
	if len(unlocked) != 0 && inCI() {
		return unlockedInCI(unlocked)
	}
	registry, err := Descriptors()
	if err != nil {
		return err
	}
	installer, err := install.NewService(atmos, registry)
	if err != nil {
		return err
	}
	selected := *atmos
	selected.Plugins.Packages = pending
	manager := manage.Manager{Project: ProjectDirectory(atmos), Config: &selected, Installer: installer}
	if err := manager.Restore(ctx, manage.RestoreOptions{}); err != nil {
		return err
	}
	return announceRestored(atmos, slices.Sorted(maps.Keys(pending)))
}

func unlockedInCI(aliases []string) error {
	slices.Sort(aliases)
	return errUtils.Build(fmt.Errorf("%w: %s", errUtils.ErrPluginUnlockedInCI, strings.Join(aliases, ", "))).
		WithHint("Run `atmos plugin install` locally and commit `.atmos.plugins.lock.yaml`, so CI restores the exact reviewed release.").
		Err()
}

// announceRestored prints one notice per automatically restored package.
func announceRestored(atmos *schema.AtmosConfiguration, aliases []string) error {
	file, err := pluginlock.Load(ProjectDirectory(atmos))
	if err != nil {
		return err
	}
	for _, alias := range aliases {
		entry, found := file.Spec.Packages[alias]
		if !found {
			continue
		}
		summary := fmt.Sprintf("Plugin `%s` was not installed, so it was installed automatically from `%s` (version %s, digest `%s`)", alias, entry.Source, entry.Version, entry.IndexDigest)
		switch signature := entry.Verification; {
		case signature.Signature == "" || signature.Signature == "unverified":
			ui.Warningf("%s; its signature is unverified. Review `.atmos.plugins.lock.yaml` and commit it.", summary)
		case signature.Identity != "":
			ui.Infof("%s; signature %s by %s.", summary, signature.Signature, signature.Identity)
		default:
			ui.Infof("%s; signature %s.", summary, signature.Signature)
		}
	}
	return nil
}

// missingSelections returns the packages to restore and, among them, those with no lock entry.
func missingSelections(atmos *schema.AtmosConfiguration, declarations map[string]schema.PluginPackage, file *pluginlock.File, cache *install.Cache) (pending map[string]schema.PluginPackage, unlocked []string, err error) {
	pending = make(map[string]schema.PluginPackage)
	for _, alias := range slices.Sorted(maps.Keys(declarations)) {
		declaration := declarations[alias]
		entry, found := file.Spec.Packages[alias]
		if !found {
			pending[alias] = declaration
			unlocked = append(unlocked, alias)
			continue
		}
		if entry.Source != declaration.Source || entry.Constraint != declaration.Version {
			return nil, nil, fmt.Errorf("%w: %s", errUtils.ErrPluginFrozenDrift, alias)
		}
		restore, err := needsRestore(atmos, cache, alias, &entry)
		if err != nil {
			return nil, nil, err
		}
		if restore {
			pending[alias] = declaration
		}
	}
	return pending, unlocked, nil
}

func needsRestore(atmos *schema.AtmosConfiguration, cache *install.Cache, alias string, entry *pluginlock.Entry) (bool, error) {
	_, err := cache.Installed(entry, install.CurrentPlatform())
	switch {
	case err == nil:
		return false, nil
	case errors.Is(err, errUtils.ErrPluginCacheCorrupt):
		if atmos.Plugins.Policy.Offline {
			return false, cache.CorruptReleaseError(err, alias, entry, install.CurrentPlatform(), install.RepairOffline)
		}
		if inCI() {
			return false, cache.CorruptReleaseError(err, alias, entry, install.CurrentPlatform(), install.RepairAutomatic)
		}
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return true, nil
	default:
		return false, fmt.Errorf("%s: %w", alias, err)
	}
}
