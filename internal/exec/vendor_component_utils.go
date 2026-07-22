package exec

import (
	"errors"
	"fmt"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
)

const ociScheme = "oci://"

var (
	ErrMissingMixinURI      = errors.New("'uri' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrMissingMixinFilename = errors.New("'filename' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrUriMustSpecified     = errors.New("'uri' must be specified in 'source.uri' in the component vendoring config file")
)

// ComponentSkipFunc matches otiai10/copy's Skip function signature.
type ComponentSkipFunc func(os.FileInfo, string, string) (bool, error)

// ReadAndProcessComponentVendorConfigFile reads and processes the component vendoring config file
// `component.yaml`. Delegates path resolution and manifest reading to pkg/vendoring
// (ResolveComponentPath, FindComponentManifestFile, ReadComponentManifest) -- the shared,
// centralized-sentinel implementation also used by vendoring.DiscoverComponentManifests -- rather
// than hand-rolling its own copy of that lookup.
func ReadAndProcessComponentVendorConfigFile(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	componentType string,
) (schema.VendorComponentConfig, string, error) {
	defer perf.Track(atmosConfig, "exec.ReadAndProcessComponentVendorConfigFile")()

	var componentConfig schema.VendorComponentConfig

	componentPath, err := vendoring.ResolveComponentPath(atmosConfig, component, componentType)
	if err != nil {
		return componentConfig, "", err
	}

	manifestFile, err := vendoring.FindComponentManifestFile(componentPath)
	if err != nil {
		return componentConfig, "", err
	}

	manifest, err := vendoring.ReadComponentManifest(manifestFile)
	if err != nil {
		return componentConfig, "", err
	}

	return *manifest, componentPath, nil
}

// ExecuteComponentVendorInternal executes the 'atmos vendor pull' command for a component.
// Supports all protocols (local files, Git, Mercurial, HTTP, HTTPS, Amazon S3, Google GCP).
// URL and archive formats described in https://github.com/hashicorp/go-getter.
// https://www.allee.xyz/en/posts/getting-started-with-go-getter.
// https://github.com/otiai10/copy.
// https://opencontainers.org/.
// https://github.com/google/go-containerregistry.
// https://docs.aws.amazon.com/AmazonECR/latest/public/public-registries.html.

// createComponentSkipFunc creates a skip function for component vendoring.
// Delegates to pkg/vendor for the shared implementation.
func createComponentSkipFunc(tempDir string, vendorComponentSpec *schema.VendorComponentSpec) func(os.FileInfo, string, string) (bool, error) {
	return vendor.CreateSkipFunc(tempDir, vendorComponentSpec.Source.IncludedPaths, vendorComponentSpec.Source.ExcludedPaths)
}

// checkComponentExcludes checks if the file matches any of the excluded patterns.
// Delegates to pkg/vendor for the shared implementation.
func checkComponentExcludes(excludePaths []string, src, trimmedSrc string) (bool, error) {
	return vendor.ShouldExcludeFile(excludePaths, trimmedSrc)
}

func ExecuteComponentVendorInternal(
	atmosConfig *schema.AtmosConfiguration,
	vendorComponentSpec *schema.VendorComponentSpec,
	component string,
	componentPath string,
	opts install.InstallOptions,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteComponentVendorInternal")()

	packages, err := buildComponentVendorPackages(buildComponentPackagesOptions{
		AtmosConfig:         atmosConfig,
		VendorComponentSpec: vendorComponentSpec,
		Component:           component,
		ComponentPath:       componentPath,
		RefreshLock:         opts.RefreshLock,
	})
	if err != nil {
		return err
	}
	packages, err = install.FilterPending(atmosConfig, packages, opts)
	if err != nil {
		return err
	}
	if len(packages) > 0 {
		return executeVendorModel(packages, opts, atmosConfig)
	}
	return nil
}

// ExecuteComponentVendorPullBatch resolves and pulls multiple components declared via their own
// component.yaml manifests in a single batched run (one progress bar, one completion summary),
// instead of one executeVendorModel call per component. Used by `atmos vendor update --pull`
// to avoid a separate "0/1" progress block per updated component.
//
// Resolution errors are propagated immediately (fail-fast): silently skipping a component whose
// component.yaml fails to parse would silently under-pull, matching the existing single-component
// behavior in handleComponentVendor (internal/exec/vendor.go), which also fails fast.
func ExecuteComponentVendorPullBatch(
	atmosConfig *schema.AtmosConfiguration,
	components []string,
	componentType string,
	opts install.InstallOptions,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteComponentVendorPullBatch")()

	if len(components) == 0 {
		return nil
	}

	var allPackages []install.VendorPackage
	for _, component := range components {
		config, componentPath, err := ReadAndProcessComponentVendorConfigFile(atmosConfig, component, componentType)
		if err != nil {
			return fmt.Errorf("component %q: %w", component, err)
		}
		packages, err := buildComponentVendorPackages(buildComponentPackagesOptions{
			AtmosConfig:         atmosConfig,
			VendorComponentSpec: &config.Spec,
			Component:           component,
			ComponentPath:       componentPath,
			RefreshLock:         opts.RefreshLock,
		})
		if err != nil {
			return fmt.Errorf("component %q: %w", component, err)
		}
		packages, err = install.FilterPending(atmosConfig, packages, opts)
		if err != nil {
			return fmt.Errorf("component %q: verify vendor lock: %w", component, err)
		}
		allPackages = append(allPackages, packages...)
	}

	if len(allPackages) == 0 {
		return nil
	}
	return executeVendorModel(allPackages, opts, atmosConfig)
}

// handleVendorPullSweep implements "atmos vendor pull --everything" (and bare "atmos vendor pull",
// which defaults --everything to true — see setDefaultEverythingFlag) for a repo with no vendor.yaml:
// it discovers every component.yaml/component.yml manifest under the configured component-type
// base path(s) — all of terraform/helmfile/packer by default, or just flg.ComponentType when the
// user passed --type explicitly (flg.TypeChanged) — groups the discovered component names by their
// own ComponentType (a repo-wide sweep with no explicit --type can mix terraform/helmfile/packer in
// one run, and ExecuteComponentVendorPullBatch only accepts one componentType per call), and pulls
// each type-group in its own batched call. Mirrors, for "vendor pull", what
// cmd/vendor/update.go's runRepoWideUpdate/runVendorPull already do for "vendor update --pull" in
// the identical repo shape.
func handleVendorPullSweep(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	defer perf.Track(atmosConfig, "exec.handleVendorPullSweep")()

	found, err := vendoring.DiscoverAllComponentManifests(flg.ComponentType, flg.TypeChanged)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return ErrNoVendorSourcesFound
	}

	componentsByType := map[string][]string{}
	for _, rs := range found {
		if rs == nil || rs.Source == nil {
			continue
		}
		componentsByType[rs.ComponentType] = append(componentsByType[rs.ComponentType], rs.Source.Component)
	}

	opts := install.InstallOptions{DryRun: flg.DryRun, RefreshLock: flg.RefreshLock, LockEnforcement: flg.LockEnforcement}
	var errs []error
	for componentType, components := range componentsByType {
		if err := ExecuteComponentVendorPullBatch(atmosConfig, components, componentType, opts); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
