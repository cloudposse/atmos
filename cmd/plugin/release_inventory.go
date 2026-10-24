package plugin

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/flags"
	pkgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
	pluginrelease "github.com/cloudposse/atmos/pkg/plugin/release"
	"github.com/cloudposse/atmos/pkg/plugin/releaseplan"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	releaseCheckFlag     = "check"
	releaseInventoryFlag = "inventory"
)

func inventoryReleaseFlags() []flags.Option {
	return []flags.Option{
		flags.WithStringFlag(releaseInventoryFlag, "", "", "Release packages from a committed repository-relative inventory"),
		flags.WithStringFlag("head", "", "HEAD", "Committed revision to release"),
		flags.WithStringFlag("base", "", "", "Base revision for independent version checks (defaults to the enabled CI provider)"),
		flags.WithBoolFlag(releaseCheckFlag, "", false, "Build, test, and pack without publishing"),
		flags.WithStringFlag("catalog", "", "", "Reconcile a catalog from the current origin default-branch inventory after publication"),
		flags.WithEnvVars(releaseInventoryFlag, "ATMOS_PLUGIN_RELEASE_INVENTORY"),
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
	results, err := pluginrelease.RunInventory(cmd.Context(), plan.Packages.Include, factory, pluginrelease.WithInventoryCheck(values.GetBool(releaseCheckFlag)))
	if err != nil {
		return err
	}
	if target := values.GetString("catalog"); target != "" {
		if err := reconcileReleasedCatalog(cmd, manager, values, target); err != nil {
			return err
		}
	}
	return presentInventoryReleases(plan, results, values)
}

func presentInventoryReleases(plan *releaseplan.Plan, results []*pluginoci.Release, values *viper.Viper) error {
	// Report success only after all required signing, verification, and catalog work.
	if err := writeReleasePlanOutputs(plan); err != nil {
		return err
	}
	if values.GetBool(releaseCheckFlag) {
		ui.Successf("Checked %d plugin releases.", len(plan.Packages.Include))
		return nil
	}
	published := make([]pluginoutput.PublishedRelease, 0, len(results))
	for index, result := range results {
		files, err := result.Files()
		if err != nil {
			return err
		}
		published = append(published, pluginoutput.PublishedRelease{Release: result, Repository: plan.Packages.Include[index].Repository, Signed: values.GetBool(signFlag), Files: files})
	}
	return pluginoutput.PublishedBatch(published, values.GetString(outputFormatFlag))
}

func reconcileReleasedCatalog(cmd *cobra.Command, manager *manage.Manager, values *viper.Viper, alias string) error {
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
	options := []catalog.SyncOption{catalog.WithSkipMissing()}
	if values.GetBool(verifyPublicFlag) {
		options = append(options, catalog.WithPublicVerification(newPublicVerifier()))
	}
	return catalog.NewSyncer(manager.Installer.Registry, manager.Installer.Descriptors).Sync(cmd.Context(), target, selections, options...)
}
