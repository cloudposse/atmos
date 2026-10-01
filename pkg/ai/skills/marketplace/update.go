package marketplace

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// UpdateSkill reinstalls name if a newer bundled catalog version is available,
// and is a no-op (not an error) if it's already current. Only bundled (catalog)
// skills are supported -- there's no cheap way to check a git-sourced skill's
// upstream version without a fetch, so those return ErrSkillUpdateNotSupported
// pointing at the manual `install <source> --force` path instead.
func (i *Installer) UpdateSkill(ctx context.Context, name string, opts *InstallOptions) error {
	defer perf.Track(nil, "marketplace.Installer.UpdateSkill")()

	installed, err := i.localRegistry.Get(name)
	if err != nil {
		return err
	}

	available, ok := LookupBundledSkill(name)
	if !ok {
		return fmt.Errorf("%w: %q is not a bundled skill; run `atmos ai skill install %s --force` to refresh it manually",
			ErrSkillUpdateNotSupported, name, installed.Source)
	}

	if !SkillVersionOutdated(installed.Version, available.Version) {
		ui.Successf("%s is already up to date (%s)", name, installed.Version)
		return nil
	}

	opts.Force = true
	return i.Install(ctx, name, *opts)
}

// UpdateAllBundled reinstalls every installed bundled skill whose catalog
// version differs from what's currently installed, skipping skills that are
// already current or that aren't installed at all. Community/git-sourced
// skills are never touched -- see UpdateSkill's doc comment.
func (i *Installer) UpdateAllBundled(opts *InstallOptions) error {
	defer perf.Track(nil, "marketplace.Installer.UpdateAllBundled")()

	catalog, err := Catalog()
	if err != nil {
		return fmt.Errorf("failed to load skill catalog: %w", err)
	}

	outdated := make([]AvailableSkill, 0, len(catalog))
	for _, available := range catalog {
		installed, getErr := i.localRegistry.Get(available.Name)
		if getErr != nil {
			continue // Not installed; nothing to update.
		}
		if SkillVersionOutdated(installed.Version, available.Version) {
			outdated = append(outdated, available)
		}
	}

	if len(outdated) == 0 {
		ui.Success("All installed bundled skills are already up to date")
		return nil
	}

	names := make([]string, 0, len(outdated))
	for _, a := range outdated {
		names = append(names, a.Name)
	}
	printItemColumns(fmt.Sprintf("%d bundled skill(s) have updates available:", len(outdated)), names)

	if !opts.SkipConfirm {
		if err := requireConfirmation(fmt.Sprintf("Update %d skill(s)?", len(outdated)), ErrUpdateCancelled); err != nil {
			return err
		}
	}

	opts.Force = true
	return i.runBatchInstallWithSpinner(opts.BasePath, opts, func(clients []string) []batchInstallOutcome {
		outcomes := make([]batchInstallOutcome, len(outdated))
		for idx := range outdated {
			outcomes[idx] = i.installOneBundledSkill(&outdated[idx], opts, clients)
		}
		return outcomes
	})
}

// SkillVersionOutdated reports whether installedVersion should be considered
// stale relative to catalogVersion. A plain string inequality is enough:
// skill versions aren't guaranteed to be semver, and this only decides
// whether to trigger a --force reinstall, not a strict ordering check.
// Exported so `atmos ai skill list --detailed`'s "update available" indicator
// and `atmos ai skill update`'s reinstall decision use the exact same rule.
func SkillVersionOutdated(installedVersion, catalogVersion string) bool {
	defer perf.Track(nil, "marketplace.SkillVersionOutdated")()

	return installedVersion != "" && catalogVersion != "" && installedVersion != catalogVersion
}
