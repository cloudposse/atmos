package exec

import (
	"errors"
	"fmt"
	"os"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
	"github.com/cloudposse/atmos/pkg/vendoring"
	vendorcomponent "github.com/cloudposse/atmos/pkg/vendoring/component"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
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

	packages, err := vendorcomponent.BuildVendorPackages(vendorcomponent.BuildPackagesOptions{
		AtmosConfig:         atmosConfig,
		VendorComponentSpec: vendorComponentSpec,
		Component:           component,
		ComponentPath:       componentPath,
		RefreshLock:         opts.RefreshLock,
		TemplateFunc:        ProcessTmpl,
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
		packages, err := vendorcomponent.BuildVendorPackages(vendorcomponent.BuildPackagesOptions{
			AtmosConfig:         atmosConfig,
			VendorComponentSpec: &config.Spec,
			Component:           component,
			ComponentPath:       componentPath,
			RefreshLock:         opts.RefreshLock,
			TemplateFunc:        ProcessTmpl,
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

	// Sort the type keys so pull order, progress output, and any joined error text are stable
	// across runs (map iteration order is nondeterministic).
	componentTypes := make([]string, 0, len(componentsByType))
	for componentType := range componentsByType {
		componentTypes = append(componentTypes, componentType)
	}
	sort.Strings(componentTypes)

	opts := install.InstallOptions{DryRun: flg.DryRun, RefreshLock: flg.RefreshLock, LockEnforcement: flg.LockEnforcement}
	var errs []error
	for _, componentType := range componentTypes {
		if err := ExecuteComponentVendorPullBatch(atmosConfig, componentsByType[componentType], componentType, opts); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// handleStackVendor implements "atmos vendor pull --stack <stack>": vendors every component
// declared in the stack that has its own component.yaml, regardless of whether the repo also has
// a vendor.yaml (--stack bypasses vendor.yaml entirely -- see handleVendorConfig). Many repos
// vendor purely through per-component component.yaml manifests declared under the components a
// stack references, with no repo-wide vendor.yaml at all.
func handleStackVendor(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	defer perf.Track(atmosConfig, "exec.handleStackVendor")()

	componentTypes := []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType}
	if flg.TypeChanged {
		componentTypes = []string{flg.ComponentType}
	}

	stacksMap, err := ExecuteDescribeStacks(atmosConfig, flg.Stack, nil, componentTypes, nil, false, false, false, false, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to describe stack %q: %w", flg.Stack, err)
	}
	if len(stacksMap) == 0 {
		return fmt.Errorf("%w: stack %q not found or has no components", errUtils.ErrStackNotFound, flg.Stack)
	}

	componentsByType, err := resolveStackVendorComponents(atmosConfig, stacksMap, componentTypes)
	if err != nil {
		return err
	}
	if len(componentsByType) == 0 {
		return nil
	}

	// Sort the type keys so pull order, progress output, and any joined error text are stable
	// across runs (map iteration order is nondeterministic).
	types := make([]string, 0, len(componentsByType))
	for componentType := range componentsByType {
		types = append(types, componentType)
	}
	sort.Strings(types)

	opts := install.InstallOptions{DryRun: flg.DryRun, RefreshLock: flg.RefreshLock, LockEnforcement: flg.LockEnforcement}
	var errs []error
	for _, componentType := range types {
		if err := ExecuteComponentVendorPullBatch(atmosConfig, componentsByType[componentType], componentType, opts); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// resolveStackVendorComponents walks stacksMap's terraform/helmfile/packer components (already
// scoped to a single stack by ExecuteDescribeStacks' filterByStack) and groups the ones with a
// component.yaml/component.yml manifest by component type, ready for
// ExecuteComponentVendorPullBatch. Abstract components are skipped (FilterAbstractComponents --
// they have no component.yaml of their own); a component without a manifest is silently skipped
// too, since not every stack component vendors this way -- --stack pulls whichever ones do. A
// resolved component name is deduped across component types (metadata.component can make two
// stack-declared names resolve to the same underlying component directory).
func resolveStackVendorComponents(
	atmosConfig *schema.AtmosConfiguration,
	stacksMap map[string]any,
	componentTypes []string,
) (map[string][]string, error) {
	defer perf.Track(atmosConfig, "exec.resolveStackVendorComponents")()

	result := make(map[string][]string)
	seen := make(map[string]bool)

	for _, stackSection := range stacksMap {
		componentsSection, ok := stackComponentsSection(stackSection)
		if !ok {
			continue
		}

		for _, componentType := range componentTypes {
			typeComponents, ok := componentsSection[componentType].(map[string]any)
			if !ok {
				continue
			}
			if err := appendStackTypeComponents(atmosConfig, typeComponents, componentType, seen, result); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// stackComponentsSection extracts one stack's "components" map[string]any section from an
// ExecuteDescribeStacks result entry, reporting false when the shape doesn't match.
func stackComponentsSection(stackSection any) (map[string]any, bool) {
	stackMap, ok := stackSection.(map[string]any)
	if !ok {
		return nil, false
	}
	componentsSection, ok := stackMap["components"].(map[string]any)
	return componentsSection, ok
}

// appendStackTypeComponents resolves one component type's components (already scoped to a single
// stack) into result[componentType] -- the per-type body of resolveStackVendorComponents' walk.
// Deduplicates against seen (shared across all types/stacks in one resolveStackVendorComponents
// call) and skips components with no component.yaml/component.yml.
func appendStackTypeComponents(
	atmosConfig *schema.AtmosConfiguration,
	typeComponents map[string]any,
	componentType string,
	seen map[string]bool,
	result map[string][]string,
) error {
	for _, name := range FilterAbstractComponents(typeComponents) {
		resolved := resolveVendorComponentName(name, typeComponents[name])
		key := componentType + "/" + resolved
		if seen[key] {
			continue
		}
		seen[key] = true

		hasManifest, err := componentHasVendorManifest(atmosConfig, resolved, componentType)
		if err != nil {
			return err
		}
		if !hasManifest {
			continue
		}
		result[componentType] = append(result[componentType], resolved)
	}
	return nil
}

// resolveVendorComponentName returns the component name to vendor for a stack-declared component,
// honoring a metadata.component override (e.g. multiple stack instances of the same underlying
// component vendor from the same directory).
func resolveVendorComponentName(name string, data any) string {
	compMap, ok := data.(map[string]any)
	if !ok {
		return name
	}
	metadata, ok := compMap["metadata"].(map[string]any)
	if !ok {
		return name
	}
	if component, ok := metadata["component"].(string); ok && component != "" {
		return component
	}
	return name
}

// componentHasVendorManifest reports whether component has a component.yaml/component.yml,
// treating a missing component directory or manifest as "no" rather than an error -- --stack
// vendors whichever of the stack's components declare one, silently skipping the rest -- and
// propagating any other error (e.g. an unsupported componentType).
func componentHasVendorManifest(atmosConfig *schema.AtmosConfiguration, component, componentType string) (bool, error) {
	componentPath, err := vendoring.ResolveComponentPath(atmosConfig, component, componentType)
	if err != nil {
		if errors.Is(err, errUtils.ErrComponentDirNotFound) {
			return false, nil
		}
		return false, err
	}
	if _, err := vendoring.FindComponentManifestFile(componentPath); err != nil {
		if errors.Is(err, errUtils.ErrComponentManifestNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
