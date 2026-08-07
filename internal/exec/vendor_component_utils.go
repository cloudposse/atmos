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

// filterStackComponentsByTags narrows a --stack/--labels-resolved, type-grouped component set to
// just the ones whose vendor.yaml source declares one of the given tags -- --tags is an independent
// filter that composes with --stack/--labels (see handleStackVendor's own doc comment), not a
// rejected combination. A component with no vendor.yaml entry at all (the common case for --stack,
// which bypasses vendor.yaml for installation -- see handleVendorConfig) has no declared tags and is
// naturally excluded by a non-empty tags filter, the same way any filter excludes an entity missing
// the filtered attribute.
func filterStackComponentsByTags(componentsByType map[string][]string, tags []string) (map[string][]string, error) {
	filtered := make(map[string][]string, len(componentsByType))
	for componentType, names := range componentsByType {
		kept, err := vendoring.FilterComponentsByDeclaredTags("", names, tags)
		if err != nil {
			return nil, err
		}
		if len(kept) > 0 {
			filtered[componentType] = kept
		}
	}
	return filtered, nil
}

// resolveAndFilterStackComponents resolves --stack/--labels' type-grouped component set (via
// resolveStackVendorComponents) and, when tags is non-empty, narrows it further via
// filterStackComponentsByTags -- the combined "resolve then optionally filter" step
// handleStackVendor delegates to, kept separate to stay within this repo's cyclomatic-complexity
// budget (CLAUDE.md: extract into a named helper, keep the caller a flat pipeline). A nil, nil
// result means there's nothing to vendor for reasons unrelated to tags (the stack's components have
// no component.yaml at all); the caller treats that as success, not an error. A tags filter that
// empties an otherwise non-empty set is reported as an explicit "matched nothing" error instead.
func resolveAndFilterStackComponents(
	atmosConfig *schema.AtmosConfiguration,
	stacksMap map[string]any,
	componentTypes []string,
	tags []string,
) (map[string][]string, error) {
	componentsByType, err := resolveStackVendorComponents(atmosConfig, stacksMap, componentTypes)
	if err != nil {
		return nil, err
	}
	if len(componentsByType) == 0 || len(tags) == 0 {
		return componentsByType, nil
	}

	filtered, err := filterStackComponentsByTags(componentsByType, tags)
	if err != nil {
		return nil, err
	}
	if len(filtered) == 0 {
		return nil, errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation("No components matched the given selector.").
			Err()
	}
	return filtered, nil
}

// handleStackVendor implements "atmos vendor pull --stack <stack>" and/or "atmos vendor pull
// --labels <labels>": vendors every component declared in the stack (or, with --labels and no
// --stack, across all stacks) that has its own component.yaml, regardless of whether the repo also
// has a vendor.yaml (this path bypasses vendor.yaml entirely for installation -- see
// handleVendorConfig). --labels composes with --stack as a further narrowing (both resolve the same
// stack-declared component set; --labels just filters it by metadata.labels), and --tags composes
// with either (or both) as yet another independent filter, this time against vendor.yaml's declared
// source tags (see filterStackComponentsByTags) -- installation still happens via each component's
// own component.yaml regardless of whether --tags is used, since --tags only narrows the candidate
// set here, it never changes how a selected component is installed. Many repos vendor purely
// through per-component component.yaml manifests declared under the components a stack references,
// with no repo-wide vendor.yaml at all.
func handleStackVendor(atmosConfig *schema.AtmosConfiguration, flg *VendorFlags) error {
	defer perf.Track(atmosConfig, "exec.handleStackVendor")()

	componentTypes := []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType}
	if flg.TypeChanged {
		componentTypes = []string{flg.ComponentType}
	}

	// Labels scope the describe pass itself (the early-skip perf optimization
	// ExecuteDescribeStacksScoped shares with `list components --labels`), so an out-of-scope
	// stack never evaluates just to be filtered out afterward.
	stacksMap, err := ExecuteDescribeStacksScoped(
		atmosConfig, flg.Stack, nil, componentTypes, nil,
		false, // ignoreMissingFiles
		false, // processTemplates
		false, // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
		true,  // authDisabled
		nil,   // tagsFilter -- vendor.yaml source tags are a separate concept, resolved elsewhere.
		flg.Labels,
		DescribeStacksErrorOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to describe stack %q: %w", flg.Stack, err)
	}
	if len(stacksMap) == 0 {
		// Matches cmd/vendor/update.go's identical zero-match wording: pull and update share the
		// same stack/labels-only selector vocabulary, so both report "no match" with the same
		// sentinel and text (unlike diff/clean/verify's shared resolver, which also covers
		// --component/--tags and so uses its own, broader wording).
		return errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation("No components matched the given --stack/--labels selector.").
			Err()
	}

	componentsByType, err := resolveAndFilterStackComponents(atmosConfig, stacksMap, componentTypes, flg.Tags)
	if err != nil {
		return err
	}
	if len(componentsByType) == 0 {
		// The stack resolved components, but none has its own component.yaml -- nothing to do here,
		// unrelated to --tags (there's no candidate set left for it to narrow).
		return nil
	}

	return pullStackComponentsByType(atmosConfig, componentsByType, install.InstallOptions{
		DryRun:          flg.DryRun,
		RefreshLock:     flg.RefreshLock,
		LockEnforcement: flg.LockEnforcement,
	})
}

// pullStackComponentsByType vendors componentsByType (already resolved and, when applicable,
// tags-filtered), one ExecuteComponentVendorPullBatch call per component type, sorted for stable
// pull order/progress output/joined-error text across runs (map iteration order is
// nondeterministic). Per-type failures are joined and returned together rather than aborting on
// the first one, so every type still gets attempted.
func pullStackComponentsByType(atmosConfig *schema.AtmosConfiguration, componentsByType map[string][]string, opts install.InstallOptions) error {
	types := make([]string, 0, len(componentsByType))
	for componentType := range componentsByType {
		types = append(types, componentType)
	}
	sort.Strings(types)

	var errs []error
	for _, componentType := range types {
		if err := ExecuteComponentVendorPullBatch(atmosConfig, componentsByType[componentType], componentType, opts); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// stackVendorComponent is one (componentType, resolved name) pair discovered by
// walkStackVendorComponents, deduplicated within a single walk by componentType+name.
type stackVendorComponent struct {
	ComponentType string
	Name          string
}

// walkStackVendorComponents is the single stack-walk shared by resolveStackVendorComponents (pull's
// --stack path, which further filters to components with a component.yaml, grouped by type) and
// ResolveVendorComponentSelector (update/diff/clean/verify's shared selector, which further
// flattens and dedups by name alone, ignoring type). Previously each function hand-rolled its own,
// near-identical copy of this walk; this is the one place they now diverge only in what they do with
// each resolved (type, name) pair, not in how components/stacks are traversed.
//
// Walks stacksMap's terraform/helmfile/packer components (already scoped to a single stack, or all
// stacks, by ExecuteDescribeStacks*'s filterByStack), skips abstract components
// (FilterAbstractComponents), resolves each name's metadata.component override, and dedupes by
// componentType+name (the same name can independently appear under two different component types).
func walkStackVendorComponents(stacksMap map[string]any, componentTypes []string) []stackVendorComponent {
	seen := make(map[string]bool)
	var result []stackVendorComponent

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
			for _, name := range FilterAbstractComponents(typeComponents) {
				resolved := resolveVendorComponentName(name, typeComponents[name])
				key := componentType + "/" + resolved
				if seen[key] {
					continue
				}
				seen[key] = true
				result = append(result, stackVendorComponent{ComponentType: componentType, Name: resolved})
			}
		}
	}

	return result
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

// resolveStackVendorComponents walks stacksMap via walkStackVendorComponents and groups the ones
// with a component.yaml/component.yml manifest by component type, ready for
// ExecuteComponentVendorPullBatch. A component without a manifest is silently skipped, since not
// every stack component vendors this way -- --stack pulls whichever ones do.
func resolveStackVendorComponents(
	atmosConfig *schema.AtmosConfiguration,
	stacksMap map[string]any,
	componentTypes []string,
) (map[string][]string, error) {
	defer perf.Track(atmosConfig, "exec.resolveStackVendorComponents")()

	result := make(map[string][]string)
	for _, c := range walkStackVendorComponents(stacksMap, componentTypes) {
		hasManifest, err := componentHasVendorManifest(atmosConfig, c.Name, c.ComponentType)
		if err != nil {
			return nil, err
		}
		if !hasManifest {
			continue
		}
		result[c.ComponentType] = append(result[c.ComponentType], c.Name)
	}

	return result, nil
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

// ResolveVendorComponentSelector resolves --stack/--labels into a flat, deduped, sorted list of
// component names across componentTypes, via the same walkStackVendorComponents walk
// resolveStackVendorComponents (pull's --stack path) uses. An empty stack scopes across all stacks.
// Reuses ExecuteDescribeStacksScoped's own labelsFilter scoping (the same mechanism `list components
// --labels` already uses) so a --labels filter never forces evaluating out-of-scope stacks.
//
// Unlike resolveStackVendorComponents, this flattens across component type (deduping by name alone,
// where walkStackVendorComponents itself dedupes by type+name) and does not require a component.yaml
// to exist -- `vendor update`, `vendor diff`, `vendor clean`, and `vendor verify` all key off
// vendor.yaml Sources[].Component by name, not by component type or per-component manifest presence.
func ResolveVendorComponentSelector(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	labels map[string]string,
	componentTypes []string,
) ([]string, error) {
	defer perf.Track(atmosConfig, "exec.ResolveVendorComponentSelector")()

	stacksMap, err := ExecuteDescribeStacksScoped(
		atmosConfig, stack, nil, componentTypes, nil,
		false, // ignoreMissingFiles
		false, // processTemplates
		false, // processYamlFunctions
		false, // includeEmptyStacks
		nil,   // skip
		nil,   // authManager
		true,  // authDisabled
		nil,   // tagsFilter -- vendor.yaml source tags are a separate concept, resolved elsewhere.
		labels,
		DescribeStacksErrorOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to describe stacks: %w", err)
	}

	seen := make(map[string]bool)
	var names []string

	for _, c := range walkStackVendorComponents(stacksMap, componentTypes) {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		names = append(names, c.Name)
	}

	sort.Strings(names)
	return names, nil
}
