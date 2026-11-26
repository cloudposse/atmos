package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
	pkgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
	pluginrelease "github.com/cloudposse/atmos/pkg/plugin/release"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	releaseCheckFlag     = "check"
	releaseInventoryFlag = "inventory"
	releaseFailFastFlag  = "fail-fast"
)

func inventoryReleaseFlags() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag(releaseInventoryFlag, "", "", "Release packages from a committed repository-relative inventory"),
		flags.WithStringFlag("head", "", "HEAD", "Committed revision to release"),
		flags.WithStringFlag("base", "", "", "Base revision for independent version checks (defaults to the enabled CI provider)"),
		flags.WithBoolFlag(releaseCheckFlag, "", false, "Build, test, and pack without publishing"),
		flags.WithStringFlag("catalog", "", "", "Reconcile a catalog from the current origin default-branch inventory after publication"),
		flags.WithBoolFlag(releaseFailFastFlag, "", false, "Stop at the first failing package instead of releasing the remaining packages"),
		flags.WithEnvVars(releaseInventoryFlag, "ATMOS_PLUGIN_RELEASE_INVENTORY"),
		flags.WithEnvVars(releaseFailFastFlag, "ATMOS_PLUGIN_RELEASE_FAIL_FAST"),
		flags.WithEnvVars("head", "ATMOS_PLUGIN_RELEASE_HEAD"),
		flags.WithEnvVars("base", "ATMOS_PLUGIN_RELEASE_BASE"),
	}
}

func createReleasePlan(cmd *cobra.Command, values *viper.Viper) (*releaseplan.Plan, error) {
	base, err := releaseBase(values.GetString("base"))
	if err != nil {
		return nil, err
	}
	return releaseplan.Create(cmd.Context(), releaseplan.WithInventory(values.GetString(releaseInventoryFlag)), releaseplan.WithHead(values.GetString("head")), releaseplan.WithBase(base))
}

func releaseBase(explicit string) (string, error) {
	if explicit != "" || !ci.Enabled(atmosConfig) {
		return explicit, nil
	}
	provider := ci.ResolveProvider()
	if provider == nil {
		return "", nil
	}
	resolution, err := provider.ResolveBase()
	if err != nil {
		return "", fmt.Errorf("%w: resolve CI base: %w", errUtils.ErrPluginReleasePlan, err)
	}
	if resolution == nil {
		return "", nil
	}
	if resolution.SHA != "" {
		return resolution.SHA, nil
	}
	return resolution.Ref, nil
}

func runInventoryRelease(cmd *cobra.Command, values *viper.Viper) error {
	plan, err := createReleasePlan(cmd, values)
	if err != nil {
		return err
	}
	manager, err := newManager(cmd)
	if err != nil {
		return err
	}
	factory := func(entry releaseplan.Entry, project string) pluginrelease.Backend {
		// Each package owns its destination; publication settings remain shared.
		packageValues := viper.New()
		for key, value := range values.AllSettings() {
			packageValues.Set(key, value)
		}
		packageValues.Set(publishDestinationFlag, entry.Repository)
		return &releaseBackend{manager: manager, values: packageValues, project: project}
	}
	failFast := values.GetBool(releaseFailFastFlag)
	results, runErr := pluginrelease.RunInventory(cmd.Context(), plan.Packages.Include, factory, pluginrelease.WithInventoryCheck(values.GetBool(releaseCheckFlag)), pluginrelease.WithInventoryFailFast(failFast))
	target := values.GetString("catalog")
	return completeInventoryRelease(cmd.Context(), runErr, &inventoryCompletion{
		failFast:  failFast,
		succeeded: len(results),
		catalog:   target != "",
		released:  releasedPackages(results),
		reconcile: func(released []catalog.ReleaseSelection) error {
			return reconcileReleasedCatalog(cmd, manager, values, target, released)
		},
		present: func() error { return presentInventoryReleases(plan, results, values) },
	})
}

// inventoryCompletion holds what remains to do after the packages ran.
type inventoryCompletion struct {
	failFast  bool
	succeeded int
	catalog   bool
	// Released lists the package versions released in this run; only they are reconciled into the catalog.
	released  []catalog.ReleaseSelection
	reconcile func(released []catalog.ReleaseSelection) error
	present   func() error
}

// completeInventoryRelease reconciles the catalog and reports the packages that were released.
// After a partial failure the successful packages are still added to the catalog and reported,
// and the package errors are returned so the command exits non-zero. Fail-fast mode, a run in
// which no package succeeded, and cancellation return the package errors without either step.
func completeInventoryRelease(ctx context.Context, runErr error, c *inventoryCompletion) error {
	if runErr != nil && (c.failFast || c.succeeded == 0 || ctx.Err() != nil) {
		return runErr
	}
	if c.catalog {
		if err := c.reconcile(c.released); err != nil {
			return errors.Join(runErr, err)
		}
	}
	return errors.Join(runErr, c.present())
}

// releasedPackages returns the name, version, and repository of each package released in
// this run, in plan order.
func releasedPackages(results []pluginrelease.InventoryRelease) []catalog.ReleaseSelection {
	released := make([]catalog.ReleaseSelection, 0, len(results))
	for index := range results {
		entry := &results[index].Entry
		released = append(released, catalog.ReleaseSelection{Name: entry.Name, Version: entry.Version, Repository: entry.Repository})
	}
	return released
}

func presentInventoryReleases(plan *releaseplan.Plan, results []pluginrelease.InventoryRelease, values *viper.Viper) error {
	// Report success only after all required signing, verification, and catalog work.
	if err := writeReleasePlanOutputs(plan); err != nil {
		return err
	}
	if values.GetBool(releaseCheckFlag) {
		ui.Successf("Checked %d plugin releases.", len(results))
		return nil
	}
	published := make([]pluginoutput.PublishedRelease, 0, len(results))
	for _, result := range results {
		files, err := result.Release.Files()
		if err != nil {
			return err
		}
		published = append(published, pluginoutput.PublishedRelease{Release: result.Release, Repository: result.Entry.Repository, Signed: values.GetBool(signFlag), Files: files})
	}
	return pluginoutput.PublishedBatch(published, values.GetString(outputFormatFlag))
}

func reconcileReleasedCatalog(cmd *cobra.Command, manager *manage.Manager, values *viper.Viper, alias string, released []catalog.ReleaseSelection) error {
	target, err := catalogReference(alias)
	if err != nil {
		return err
	}
	head, err := pluginrelease.CurrentInventoryHead(cmd.Context(), pkgit.NewExecRunner())
	if err != nil {
		return err
	}
	selections, err := catalog.ReadInventory(cmd.Context(), catalog.WithInventory(values.GetString(releaseInventoryFlag)), catalog.WithHead(head))
	if err != nil {
		return err
	}
	selections = onlyPackages(selections, released)
	if len(selections) == 0 {
		return nil
	}
	options := []catalog.SyncOption{catalog.WithSkipMissing()}
	if values.GetBool(verifyPublicFlag) {
		options = append(options, catalog.WithPublicVerification(newPublicVerifier()))
	}
	return catalog.NewSyncer(manager.Installer.Registry, manager.Installer.Descriptors).Sync(cmd.Context(), target, selections, options...)
}

// onlyPackages keeps the catalog selections that match a package version released in this run:
// the same name, the same normalized version, and the same repository. The inventory is re-read
// at the current default-branch head, so it can list a package this run did not release (one
// that failed, never started, or was added after planning) or a different version of a released
// package; those keep their existing catalog entry instead of a version that this run did not
// publish and verify.
func onlyPackages(selections []catalog.ReleaseSelection, released []catalog.ReleaseSelection) []catalog.ReleaseSelection {
	return slices.DeleteFunc(selections, func(selection catalog.ReleaseSelection) bool {
		return !slices.ContainsFunc(released, func(candidate catalog.ReleaseSelection) bool {
			return sameRelease(&selection, &candidate)
		})
	})
}

// sameRelease reports whether two selections name the same package version in the same repository.
func sameRelease(left, right *catalog.ReleaseSelection) bool {
	return left.Name == right.Name && left.Repository == right.Repository && catalog.NormalizeVersion(left.Version) == catalog.NormalizeVersion(right.Version)
}
