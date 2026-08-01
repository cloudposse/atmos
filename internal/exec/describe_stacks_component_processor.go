//nolint:revive // file-length-limit: 570 lines, slightly above 500. Will split in follow-up.
package exec

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version/manager"
	atmosYaml "github.com/cloudposse/atmos/pkg/yaml"
)

// componentSections holds all standard sections extracted from a component map.
type componentSections struct {
	vars        map[string]any
	metadata    map[string]any
	settings    map[string]any
	env         map[string]any
	auth        map[string]any
	providers   map[string]any
	hooks       map[string]any
	test        map[string]any
	secrets     map[string]any
	overrides   map[string]any
	backend     map[string]any
	backendType string
}

// processComponentTypeOpts configures component-type-specific behaviour.
// Only terraform uses workspace and metadata inheritance; other types leave these false.
type processComponentTypeOpts struct {
	// buildWorkspace instructs the processor to resolve the Terraform workspace and attach it to the component section.
	buildWorkspace bool
	// applyMetadataInheritance instructs the processor to resolve metadata from inherited base components.
	applyMetadataInheritance bool
	// checkIncludeEmpty instructs the processor to filter out empty sections according to AtmosConfiguration.Describe.Settings.IncludeEmpty.
	checkIncludeEmpty bool
}

// componentAuthManagerResolver builds a per-component AuthManager for the given
// component section. It mirrors the signature of createComponentAuthManager so
// that describeStacksProcessor can inject a test double. See
// docs/fixes/2026-04-24-list-instances-per-component-auth.md for context.
type componentAuthManagerResolver func(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	component string,
	stack string,
	parentAuthManager auth.AuthManager,
) (auth.AuthManager, error)

// describeStacksProcessor holds the immutable configuration and the mutable result map
// for a single call to ExecuteDescribeStacks.  All processing methods are attached to
// this struct so that they share configuration without requiring long argument lists.
type describeStacksProcessor struct {
	atmosConfig          *schema.AtmosConfiguration
	filterByStack        string
	components           []string
	sections             []string
	componentTypes       []string
	processTemplates     bool
	processYamlFunctions bool
	authDisabled         bool
	useMocks             bool
	// tagsFilter/labelsFilter narrow the result to components matching info.Tags
	// (any-match) / info.Labels (all-match), mirroring
	// pkg/scheduler/adapters/terraform.go's matchesTerraformTagsAndLabels post-filter.
	// When non-empty, components that can be cheaply proven out-of-scope from
	// already-inherited metadata.tags/metadata.labels are skipped before auth/template/
	// YAML-function evaluation (see scopeDecision). The post-filter remains the
	// authoritative answer for every component this early gate cannot decide.
	tagsFilter   []string
	labelsFilter map[string]string
	// resolveSecrets makes this processor retrieve !secret values while it resolves
	// YAML functions. It is used by Terraform graph execution preflight; inspection
	// commands retain their mask-only behavior even when output masking is enabled.
	resolveSecrets     bool
	includeEmptyStacks bool
	skip               []string
	authManager        auth.AuthManager
	finalStacksMap     map[string]any
	// componentAuthResolver builds a per-component AuthManager; defaults to
	// createComponentAuthManager and is overridable in tests.
	componentAuthResolver componentAuthManagerResolver
	// authManagerCache memoizes per-component AuthManagers within one describe-stacks pass, keyed by
	// auth section + parent chain, so components sharing an auth section reuse one manager instead of
	// re-running the full auth cycle (credential writes, file locks, keyring rebuilds). The pass is
	// single-threaded, so a plain map needs no locking.
	// See docs/fixes/2026-06-22-describe-stacks-scope-and-cache-per-component-auth.md.
	authManagerCache map[string]auth.AuthManager
	// onWarning, when non-nil, switches YAML-function processing to lenient mode: a
	// recoverable per-value error (e.g. backend not yet provisioned) is substituted with
	// nil and reported here instead of aborting the whole describe-stacks call. See
	// ProcessCustomYamlTagsLenient and ExecuteDescribeStacksWithOptions.
	onWarning func(DegradationWarning)
}

// withDegradation switches the processor to lenient YAML-function processing: recoverable
// per-value errors are substituted with nil and reported via onWarning instead of failing
// the whole describe-stacks call. Passing a nil onWarning restores the default strict
// behavior (equivalent to not calling withDegradation at all).
func (p *describeStacksProcessor) withDegradation(onWarning func(DegradationWarning)) *describeStacksProcessor {
	p.onWarning = onWarning
	return p
}

// newDescribeStacksProcessor creates a processor with an empty result map.
func newDescribeStacksProcessor( //nolint:revive // argument-limit: constructor needs all config params.
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components, componentTypes, sections []string,
	processTemplates, processYamlFunctions, includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
) *describeStacksProcessor {
	return newDescribeStacksProcessorWithAuthDisabled(
		atmosConfig,
		filterByStack,
		components, componentTypes, sections,
		processTemplates, processYamlFunctions, includeEmptyStacks,
		skip,
		authManager,
		false,
	)
}

func newDescribeStacksProcessorWithAuthDisabled( //nolint:revive // argument-limit: constructor needs all config params.
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components, componentTypes, sections []string,
	processTemplates, processYamlFunctions, includeEmptyStacks bool,
	skip []string,
	authManager auth.AuthManager,
	authDisabled bool,
) *describeStacksProcessor {
	return &describeStacksProcessor{
		atmosConfig:           atmosConfig,
		filterByStack:         filterByStack,
		components:            components,
		sections:              sections,
		componentTypes:        componentTypes,
		processTemplates:      processTemplates,
		processYamlFunctions:  processYamlFunctions,
		authDisabled:          authDisabled,
		includeEmptyStacks:    includeEmptyStacks,
		skip:                  skip,
		authManager:           authManager,
		finalStacksMap:        make(map[string]any),
		componentAuthResolver: createComponentAuthManager,
		authManagerCache:      make(map[string]auth.AuthManager),
	}
}

// shouldResolvePerComponentAuth reports whether the per-component AuthManager
// resolver should run for this processor configuration. Per-component auth is
// needed whenever the component will be processed by either YAML functions
// (e.g. !terraform.state, !terraform.output) or Go templates (e.g.
// atmos.Component), because both paths consume info.AuthContext to authenticate
// terraform subprocesses against remote backends.
//
// When both flags are false, no template or YAML-function evaluation will
// occur on this component, so the authManager is not consulted downstream and
// resolution can be skipped.
//
// See docs/fixes/2026-04-24-list-instances-per-component-auth.md for the fix
// that widened this condition from processYamlFunctions-only to include
// templates (the atmos.Component path).
func shouldResolvePerComponentAuth(processTemplates, processYamlFunctions bool) bool {
	return processTemplates || processYamlFunctions
}

// resolveComponentAuthManager returns the AuthManager to use for this component.
// It returns the parent AuthManager unchanged when per-component resolution is
// disabled (see shouldResolvePerComponentAuth) or when the component does not
// declare its own default identity in its auth section. When the component
// declares a default identity, resolver errors are fatal by default: falling
// back to the parent manager could silently read the wrong backend/account
// for a manager that *did* resolve, just not to what was declared. In warn/silent
// mode (p.onWarning set), a construction FAILURE — no manager was built at all,
// as opposed to one resolving to an unexpected identity — is instead reported as
// a degradation warning and this component falls back to the parent manager,
// matching the same "recoverable, substitute, continue" pattern YAML-function
// processing already uses for e.g. a not-yet-provisioned backend.
func (p *describeStacksProcessor) resolveComponentAuthManager(
	componentSection map[string]any,
	componentName, stackName string,
) (auth.AuthManager, error) {
	componentAuthManager := p.authManager
	if p.authDisabled || !shouldResolvePerComponentAuth(p.processTemplates, p.processYamlFunctions) {
		return componentAuthManager, nil
	}
	authSection, hasAuth := componentSection[cfg.AuthSectionName].(map[string]any)
	if !hasAuth || !hasDefaultIdentity(authSection) {
		return componentAuthManager, nil
	}

	// Reuse a manager already resolved for the same auth section this pass. The resolver derives its
	// result only from the auth section, the (constant) global config, and the parent manager
	// (componentName/stackName are logging only), so an identical key yields an equivalent manager.
	cacheKey, cacheable := p.componentAuthCacheKey(authSection)
	if cacheable {
		if cached, ok := p.authManagerCache[cacheKey]; ok {
			return cached, nil
		}
	}

	resolver := p.componentAuthResolver
	if resolver == nil {
		resolver = createComponentAuthManager
	}
	resolved, createErr := resolver(p.atmosConfig, componentSection, componentName, stackName, p.authManager)
	if createErr != nil {
		if p.onWarning != nil {
			p.onWarning(DegradationWarning{
				Stack:     stackName,
				Component: componentName,
				Function:  "auth",
				Reason:    createErr.Error(),
			})
			return componentAuthManager, nil
		}
		return componentAuthManager, fmt.Errorf("%w: failed to resolve auth for component %q in stack %q: %w", errUtils.ErrAuthManager, componentName, stackName, createErr)
	}
	result := componentAuthManager
	if resolved != nil {
		result = resolved
	}
	if cacheable {
		p.cacheComponentAuthManager(cacheKey, result)
	}
	return result, nil
}

// componentAuthCacheKey delegates to the shared buildComponentAuthCacheKey so the describe-stacks
// processor and the nested terraform.state path key per-component AuthManagers identically and cannot
// drift. See buildComponentAuthCacheKey in terraform_nested_auth_helper.go.
func (p *describeStacksProcessor) componentAuthCacheKey(authSection map[string]any) (string, bool) {
	return buildComponentAuthCacheKey(p.authManager, authSection)
}

// cacheComponentAuthManager stores a resolved manager, lazily creating the map so struct-literal
// processors (e.g. in tests) also memoize.
func (p *describeStacksProcessor) cacheComponentAuthManager(key string, manager auth.AuthManager) {
	if p.authManagerCache == nil {
		p.authManagerCache = make(map[string]auth.AuthManager)
	}
	p.authManagerCache[key] = manager
}

// processStackFile processes one stack file, iterating over all requested component types.
func (p *describeStacksProcessor) processStackFile(stackFileName string, stackMap map[string]any) error { //nolint:revive // cyclomatic: pre-creation guard adds unavoidable branches.
	defer perf.Track(p.atmosConfig, "exec.describeStacksProcessor.processStackFile")()

	// Read manifest name before deleting imports — getStackManifestName reads "name",
	// not "imports", but keeping reads before mutations avoids implicit ordering assumptions.
	stackManifestName := getStackManifestName(stackMap)

	// Delete the stack-wide imports section (not needed in output).
	// Shallow-clone first: stackMap is owned by the shared FindStacksMap cache, and
	// deleting from it in place would strip `imports` (and thus `deps`) from every
	// subsequent ProcessStacks call in the same process (e.g., DAG-scheduled bulk
	// commands, which run ExecuteDescribeStacks before executing components).
	stackMap = maps.Clone(stackMap)
	delete(stackMap, "imports")

	// When includeEmptyStacks is true, pre-create an entry in the result map so that
	// stacks without components (e.g., import-only stacks) are still present in the output.
	// Only pre-create when the stack name can be resolved without per-component context:
	// - manifest name is set (explicit name: field), OR
	// - neither NameTemplate nor NamePattern is configured (raw file name is the final name).
	// When NameTemplate or NamePattern is active, the real name is resolved per-component
	// and pre-creating under stackFileName would leave a ghost entry.
	canResolveNameEarly := stackManifestName != "" ||
		(p.atmosConfig.Stacks.NameTemplate == "" && GetStackNamePattern(p.atmosConfig) == "")

	if p.includeEmptyStacks && canResolveNameEarly {
		initialName := stackFileName
		if stackManifestName != "" {
			initialName = stackManifestName
		}
		// Skip pre-creation if filterByStack is active and this stack doesn't match.
		if shouldFilterByStack(p.filterByStack, stackFileName, initialName) {
			return nil
		}
		if !u.MapKeyExists(p.finalStacksMap, initialName) {
			entry := make(map[string]any)
			entry[cfg.ComponentsSectionName] = make(map[string]any)
			p.finalStacksMap[initialName] = entry
		}
	}

	componentsSection, ok := stackMap[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return nil
	}

	type typeEntry struct {
		name string
		opts processComponentTypeOpts
	}
	typeEntries := []typeEntry{
		{cfg.TerraformSectionName, processComponentTypeOpts{
			buildWorkspace:           true,
			applyMetadataInheritance: true,
			checkIncludeEmpty:        true,
		}},
		{cfg.HelmfileSectionName, processComponentTypeOpts{}},
		{cfg.PackerSectionName, processComponentTypeOpts{}},
		{cfg.AnsibleSectionName, processComponentTypeOpts{}},
		{cfg.KubernetesSectionName, processComponentTypeOpts{applyMetadataInheritance: true}},
		{cfg.HelmSectionName, processComponentTypeOpts{applyMetadataInheritance: true}},
		{cfg.ContainerSectionName, processComponentTypeOpts{}},
		{cfg.EmulatorSectionName, processComponentTypeOpts{}},
	}

	for _, te := range typeEntries {
		if len(p.componentTypes) > 0 && !slices.Contains(p.componentTypes, te.name) {
			continue
		}
		typeSection, ok := componentsSection[te.name].(map[string]any)
		if !ok {
			continue
		}
		if err := p.processComponentTypeSection(stackFileName, stackManifestName, te.name, typeSection, te.opts); err != nil {
			return err
		}
	}

	return nil
}

// processComponentTypeSection iterates over every component within a component type section
// (e.g., all Terraform components in a stack file) and processes each one.
func (p *describeStacksProcessor) processComponentTypeSection(
	stackFileName, stackManifestName, typeName string,
	typeSection map[string]any,
	opts processComponentTypeOpts,
) error {
	defer perf.Track(p.atmosConfig, "exec.describeStacksProcessor.processComponentTypeSection")()

	for componentName, compSection := range typeSection {
		origSection, ok := compSection.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid 'components.%s.%s' section in the file '%s'", //nolint:err113 // Dynamic context needed for debugging.
				typeName, componentName, stackFileName)
		}

		// Shallow-clone the component section so mutations (setting defaults,
		// metadata inheritance) don't modify the shared FindStacksMap cache.
		componentSection := make(map[string]any, len(origSection))
		for k, v := range origSection {
			componentSection[k] = v
		}

		// Ensure the `component` key is set (defaults to the component name).
		if comp, ok := componentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
			componentSection[cfg.ComponentSectionName] = componentName
		}

		if err := p.processComponentEntry(
			stackFileName, stackManifestName, typeName,
			componentName, componentSection, typeSection, opts,
		); err != nil {
			return err
		}
	}
	return nil
}

// processComponentEntry processes a single component: resolves the stack name,
// filters, builds the ConfigAndStacksInfo, processes templates, and writes to the result map.
func (p *describeStacksProcessor) processComponentEntry( //nolint:gocognit,revive,cyclop,funlen // Orchestrator function with unavoidable branching.
	stackFileName, stackManifestName, typeName,
	componentName string,
	componentSection, allTypeComponents map[string]any,
	opts processComponentTypeOpts,
) error {
	defer perf.Track(p.atmosConfig, "exec.describeStacksProcessor.processComponentEntry")()

	// Find derived components to include even when the component filter is active.
	derivedComponents, err := FindComponentsDerivedFromBaseComponents(stackFileName, allTypeComponents, p.components)
	if err != nil {
		return err
	}

	// Extract all standard sections with empty-map defaults.
	secs := extractDescribeComponentSections(componentSection)

	// Terraform-only: resolve inherited metadata from base components.
	if opts.applyMetadataInheritance && p.atmosConfig.Stacks.Inherit.IsMetadataInheritanceEnabled() {
		secs.metadata, err = applyTerraformMetadataInheritance(
			p.atmosConfig, allTypeComponents, componentName, stackFileName, secs.metadata,
		)
		if err != nil {
			return err
		}
	}

	// Enforce the selector purity contract on metadata.tags/metadata.labels —
	// whether or not a filter is active. Selectors drive scoping decisions
	// before evaluation, so by design they may not contain constructs
	// requiring authentication or process execution; this errors early with a
	// by-design explanation instead of deferring to a later evaluation failure.
	// describe.settings.eager_evaluation is the escape hatch: it forces full
	// evaluation (no pre-evaluation scoping), under which selectors are
	// evaluated like any other value and the purity contract is moot.
	if err := p.validateSelectorMetadata(secs.metadata); err != nil {
		return fmt.Errorf("component %q in stack manifest %q: %w", componentName, stackFileName, err)
	}

	info := buildConfigAndStacksInfo(componentName, stackFileName, stackManifestName, secs)

	// Ensure the component key is present in the info's ComponentSection.
	if comp, ok := info.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
		info.ComponentSection[cfg.ComponentSectionName] = componentName
	}

	// Resolve the logical stack name.  When the name_pattern path is taken, resolveStackName
	// also returns the populated Context so that BuildTerraformWorkspace and template functions
	// that read info.Context see non-zero values (matching the original monolith's behaviour).
	stackName, resolvedContext, err := resolveStackName(p.atmosConfig, stackFileName, stackManifestName, info, secs.vars)
	if err != nil {
		return err
	}
	info.Context = resolvedContext
	info.AuthDisabled = p.authDisabled
	info.UseMocks = p.useMocks

	// Filter: skip this component if it does not belong to the requested stack.
	// Done before resolveComponentAuthManager (below) so out-of-scope components don't trigger a
	// full auth cycle. See docs/fixes/2026-06-22-describe-stacks-scope-and-cache-per-component-auth.md.
	if shouldFilterByStack(p.filterByStack, stackFileName, stackName) {
		return nil
	}

	// Filter: skip this component if requested tags/labels don't match, using only
	// already-inherited metadata (no auth/template/YAML-function evaluation triggered).
	// Generalizes the -s early-skip above to --tags/--labels. See
	// docs/fixes/2026-07-25-scope-before-evaluate-labels-tags-list-dependencies.md.
	if inScope, decidable := p.scopeDecision(secs.metadata, componentSection); decidable && !inScope {
		return nil
	}

	if stackName == "" {
		stackName = stackFileName
	}

	// Filter: skip this component if it does not match the requested component list.
	// This check is performed before any mutations to componentSection so that
	// the live stacksMap data is not modified for filtered-out components.
	componentIncluded := len(p.components) == 0 ||
		slices.Contains(p.components, componentName) ||
		slices.Contains(derivedComponents, componentName)
	if !componentIncluded {
		return nil
	}

	// Resolve the per-component auth manager (may fall back to the parent). Must run before the
	// template and YAML-function processing below, which read info.AuthContext.
	componentAuthManager, err := p.resolveComponentAuthManager(componentSection, componentName, stackName)
	if err != nil {
		return err
	}
	propagateAuth(&info, componentAuthManager)

	// Ensure the stack-level entry exists (only for included components).
	if !u.MapKeyExists(p.finalStacksMap, stackName) {
		p.finalStacksMap[stackName] = make(map[string]any)
	}

	info.Stack = stackName
	setAtmosComponentMetadata(componentSection, componentName, stackName, stackFileName)
	setAtmosComponentMetadata(info.ComponentSection, componentName, stackName, stackFileName)

	ensureComponentEntryInMap(p.finalStacksMap, stackName, typeName, componentName)

	// Terraform-only: build and attach the Terraform workspace.
	if opts.buildWorkspace {
		workspace, wsErr := BuildTerraformWorkspace(p.atmosConfig, info)
		if wsErr != nil {
			return wsErr
		}
		componentSection["workspace"] = workspace
		info.ComponentSection["workspace"] = workspace
	}

	// Add component_info with component_path.
	componentInfo := buildComponentInfo(p.atmosConfig, componentSection, typeName)
	componentSection[componentInfoKey] = componentInfo
	info.ComponentSection[componentInfoKey] = componentInfo

	// Mocks are literal fixture data. Do not render templates or resolve YAML
	// functions within them; doing so could reach the real dependency that the
	// mock is intended to replace.
	literalMocks, hasLiteralMocks := componentSection[cfg.MocksSectionName]
	if hasLiteralMocks {
		delete(componentSection, cfg.MocksSectionName)
	}

	// `sources` is generated provenance for describe output. It can retain overridden
	// parent values, so it must not be treated as executable component configuration.
	sources, hasSources := componentSection[sourcesSectionName]
	if hasSources {
		componentSection = maps.Clone(componentSection)
		delete(componentSection, sourcesSectionName)
		info.ComponentSection = componentSection
	}

	// Process Go templates.
	if p.processTemplates {
		componentSection, err = processComponentSectionTemplates(p.atmosConfig, &info, componentSection, secs.settings)
		if err != nil {
			return err
		}
		// Sync info.ComponentSection so YAML functions see rendered values
		// instead of raw template strings like "{{ .vars.region }}".
		info.ComponentSection = componentSection
	}

	// Process YAML functions.
	if p.processYamlFunctions {
		// A component disabled via metadata.enabled has no deployed state, so its
		// !terraform.state / !terraform.output must not be resolved — the backend read would
		// fail with "state not provisioned". Gate on metadata.enabled only, independent of
		// vars.enabled. See docs/fixes/2026-06-22-describe-respect-metadata-enabled.md.
		skip := p.skip
		if !isComponentEnabled(secs.metadata, componentName) {
			skip = disabledComponentTerraformSkip(p.skip)
		}
		componentSection, err = processComponentSectionYAMLFunctions(
			p.atmosConfig,
			&info,
			componentSection,
			skip,
			p.onWarning,
			!p.resolveSecrets && iolib.MaskingEnabled(),
		)
		if err != nil {
			return err
		}
	}
	if hasLiteralMocks {
		componentSection[cfg.MocksSectionName] = literalMocks
		info.ComponentSection[cfg.MocksSectionName] = literalMocks
	}
	if hasSources {
		componentSection[sourcesSectionName] = sources
		info.ComponentSection[sourcesSectionName] = sources
	}

	// Write the (optionally filtered) sections into the result map.
	includeEmpty := resolveIncludeEmpty(p.atmosConfig, opts.checkIncludeEmpty)
	destMap, ok := getComponentDestMap(p.finalStacksMap, stackName, typeName, componentName)
	if !ok {
		return fmt.Errorf("internal error: component entry not found for %s/%s/%s", stackName, typeName, componentName) //nolint:err113 // Dynamic context for debugging.
	}
	addSectionsToComponentEntry(destMap, componentSection, p.sections, includeEmpty)

	return nil
}

// ---------------------------------------------------------------------------
// Pure helper functions – independently unit-testable
// ---------------------------------------------------------------------------

// disabledComponentTerraformSkip returns baseSkip plus the terraform state/output YAML functions so a
// component disabled via metadata.enabled keeps its !terraform.* values unresolved (no backend read).
// The names are bare (no leading "!") to match skipFunc, which trims the tag prefix before comparing.
// baseSkip is cloned so the processor's shared skip slice is never mutated.
func disabledComponentTerraformSkip(baseSkip []string) []string {
	return append(
		slices.Clone(baseSkip),
		strings.TrimPrefix(u.AtmosYamlFuncTerraformState, "!"),
		strings.TrimPrefix(u.AtmosYamlFuncTerraformOutput, "!"),
	)
}

// extractDescribeComponentSections returns all standard Atmos sections from a component map,
// using empty maps (or empty string) as defaults when a section is absent.
// This is used by the describe stacks processor; for the full stack processor, see extractComponentSections.
func extractDescribeComponentSections(componentSection map[string]any) componentSections { //nolint:revive,funlen // 10 section extractions, each trivial.
	s := componentSections{}

	if v, ok := componentSection[cfg.VarsSectionName].(map[string]any); ok {
		s.vars = v
	} else {
		s.vars = map[string]any{}
	}

	if v, ok := componentSection[cfg.MetadataSectionName].(map[string]any); ok {
		s.metadata = v
	} else {
		s.metadata = map[string]any{}
	}

	if v, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
		s.settings = v
	} else {
		s.settings = map[string]any{}
	}

	if v, ok := componentSection[cfg.EnvSectionName].(map[string]any); ok {
		s.env = v
	} else {
		s.env = map[string]any{}
	}

	if v, ok := componentSection[cfg.AuthSectionName].(map[string]any); ok {
		s.auth = v
	} else {
		s.auth = map[string]any{}
	}

	if v, ok := componentSection[cfg.ProvidersSectionName].(map[string]any); ok {
		s.providers = v
	} else {
		s.providers = map[string]any{}
	}

	if v, ok := componentSection[cfg.HooksSectionName].(map[string]any); ok {
		s.hooks = v
	} else {
		s.hooks = map[string]any{}
	}

	if v, ok := componentSection[cfg.TestSectionName].(map[string]any); ok {
		s.test = v
	} else {
		s.test = map[string]any{}
	}

	if v, ok := componentSection[cfg.SecretsSectionName].(map[string]any); ok {
		s.secrets = v
	} else {
		s.secrets = map[string]any{}
	}

	if v, ok := componentSection[cfg.OverridesSectionName].(map[string]any); ok {
		s.overrides = v
	} else {
		s.overrides = map[string]any{}
	}

	if v, ok := componentSection[cfg.BackendSectionName].(map[string]any); ok {
		s.backend = v
	} else {
		s.backend = map[string]any{}
	}

	if v, ok := componentSection[cfg.BackendTypeSectionName].(string); ok {
		s.backendType = v
	}

	return s
}

// buildConfigAndStacksInfo constructs a schema.ConfigAndStacksInfo from the extracted sections.
func buildConfigAndStacksInfo(
	componentName, stackFileName, stackManifestName string,
	secs componentSections, //nolint:gocritic // hugeParam: value type by design (read-only snapshot).
) schema.ConfigAndStacksInfo {
	return schema.ConfigAndStacksInfo{
		ComponentFromArg:          componentName,
		Stack:                     stackFileName,
		StackManifestName:         stackManifestName,
		ComponentMetadataSection:  secs.metadata,
		ComponentVarsSection:      secs.vars,
		ComponentSettingsSection:  secs.settings,
		ComponentEnvSection:       secs.env,
		ComponentAuthSection:      secs.auth,
		ComponentProvidersSection: secs.providers,
		ComponentHooksSection:     secs.hooks,
		ComponentOverridesSection: secs.overrides,
		ComponentBackendSection:   secs.backend,
		ComponentBackendType:      secs.backendType,
		ComponentSection: map[string]any{
			cfg.VarsSectionName:        secs.vars,
			cfg.MetadataSectionName:    secs.metadata,
			cfg.SettingsSectionName:    secs.settings,
			cfg.EnvSectionName:         secs.env,
			cfg.AuthSectionName:        secs.auth,
			cfg.ProvidersSectionName:   secs.providers,
			cfg.HooksSectionName:       secs.hooks,
			cfg.TestSectionName:        secs.test,
			cfg.SecretsSectionName:     secs.secrets,
			cfg.OverridesSectionName:   secs.overrides,
			cfg.BackendSectionName:     secs.backend,
			cfg.BackendTypeSectionName: secs.backendType,
		},
	}
}

// resolveStackName determines the final logical stack name to use when writing to the result map.
// Precedence: manifest name > name_template > name_pattern > filename.
// It also returns the schema.Context populated when a name_pattern is used; callers should
// set info.Context from the returned value so that BuildTerraformWorkspace and template
// functions have access to the correct context fields.
func resolveStackName(
	atmosConfig *schema.AtmosConfiguration,
	stackFileName, stackManifestName string,
	info schema.ConfigAndStacksInfo, //nolint:gocritic // hugeParam: read-only, passed by value intentionally.
	varsSection map[string]any,
) (string, schema.Context, error) {
	switch {
	case stackManifestName != "":
		return stackManifestName, schema.Context{}, nil

	case atmosConfig.Stacks.NameTemplate != "":
		name, err := ProcessTmpl(atmosConfig, "describe-stacks-name-template",
			atmosConfig.Stacks.NameTemplate, info.ComponentSection, false)
		if err != nil {
			return "", schema.Context{}, err
		}
		return name, schema.Context{}, nil

	case GetStackNamePattern(atmosConfig) != "":
		context := cfg.GetContextFromVars(varsSection)
		name, err := cfg.GetContextPrefix(stackFileName, context, GetStackNamePattern(atmosConfig), stackFileName)
		if err != nil {
			// Fall back to filename when pattern validation fails.
			log.Debug("Pattern validation failed, using filename as stack name",
				logFieldStack, stackFileName, "error", err)
			return stackFileName, context, nil
		}
		return name, context, nil

	default:
		return stackFileName, schema.Context{}, nil
	}
}

// Metadata subsection keys read by the tags/labels scope gate and the
// selector purity validation.
const (
	metadataTagsKey   = "tags"
	metadataLabelsKey = "labels"
)

// shouldFilterByStack returns true when the component should be skipped because it
// does not belong to the requested stack filter.  An empty filterByStack means no filtering.
func shouldFilterByStack(filterByStack, stackFileName, stackName string) bool {
	return filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName
}

// scopeDecision reports whether a component is in scope for this processor's
// tags/labels filter, using only cheaply-available metadata (no auth/template/
// YAML-function evaluation). Decidable is false when selector metadata cannot
// be safely resolved, or when eager evaluation is forced via
// describe.settings.eager_evaluation: in both cases the
// caller must fall through to full evaluation, leaving
// pkg/scheduler/adapters/terraform.go's post-filter as the authoritative answer.
func (p *describeStacksProcessor) scopeDecision(metadata, componentSection map[string]any) (inScope bool, decidable bool) {
	if len(p.tagsFilter) == 0 && len(p.labelsFilter) == 0 {
		return true, true
	}
	if GetEagerEvaluationSetting(p.atmosConfig) {
		return true, false
	}
	leftDelim, rightDelim := p.templateDelims()
	selectorData := maps.Clone(componentSection)
	selectorData[cfg.MetadataSectionName] = metadata
	return inScopeByTagsAndLabelsWithContext(metadata, selectorData, p.tagsFilter, p.labelsFilter, leftDelim, rightDelim)
}

// templateDelims returns the effective template delimiters for this processor's
// configuration ("{{"/"}}" unless 'templates.settings.delimiters' overrides them).
func (p *describeStacksProcessor) templateDelims() (string, string) {
	return tags.TemplateDelims(p.atmosConfig.Templates.Settings.Delimiters)
}

// validateSelectorMetadata enforces the selector purity contract on this
// component's metadata.tags and metadata.labels (see tags.ValidateSelectorValue).
// When describe.settings.eager_evaluation forces full evaluation, the contract
// is not enforced: no scoping decision is made before evaluation, selectors are
// evaluated like any other value, and rejecting previously-working manifests
// would leave users without a rollback path.
func (p *describeStacksProcessor) validateSelectorMetadata(metadata map[string]any) error {
	if GetEagerEvaluationSetting(p.atmosConfig) {
		return nil
	}
	rawTags, hasTags := metadata[metadataTagsKey]
	rawLabels, hasLabels := metadata[metadataLabelsKey]
	if !hasTags && !hasLabels {
		return nil
	}
	leftDelim, rightDelim := p.templateDelims()
	if err := tags.ValidateSelectorValue("metadata.tags", rawTags, leftDelim, rightDelim); err != nil {
		return err
	}
	return tags.ValidateSelectorValue("metadata.labels", rawLabels, leftDelim, rightDelim)
}

// inScopeByTagsAndLabels mirrors matchesTerraformTagsAndLabels in
// pkg/scheduler/adapters/terraform.go (tags: any-match, labels: all-match) so the
// early-skip gate here and that later post-filter can never disagree. Returns
// decidable=false when metadata.tags/metadata.labels contain an unresolved Go
// template or Atmos YAML-function marker (see tags.SelectorUnresolved), since
// their real value cannot be determined without the full template/YAML-function
// evaluation this gate exists to avoid for out-of-scope components.
func inScopeByTagsAndLabels(metadata map[string]any, filterTags []string, filterLabels map[string]string, leftDelim string) (inScope bool, decidable bool) {
	if len(filterTags) == 0 && len(filterLabels) == 0 {
		return true, true
	}

	rawTags := metadata[metadataTagsKey]
	rawLabels := metadata[metadataLabelsKey]

	if tags.SelectorUnresolved(rawTags, leftDelim) || tags.SelectorUnresolved(rawLabels, leftDelim) {
		return true, false
	}

	if len(filterTags) > 0 && !tags.MatchesTags(tags.ToStringSlice(rawTags), filterTags, tags.TagModeAny) {
		return false, true
	}
	if len(filterLabels) > 0 && !tags.MatchesLabels(tags.ToStringMap(rawLabels), filterLabels) {
		return false, true
	}
	return true, true
}

// inScopeByTagsAndLabelsWithContext resolves simple selector templates against
// the merged component configuration before applying the normal scope gate.
// Templates that cannot be resolved stay undecidable so the caller preserves
// the full evaluation path rather than incorrectly excluding a component.
func inScopeByTagsAndLabelsWithContext(metadata, data map[string]any, filterTags []string, filterLabels map[string]string, leftDelim, rightDelim string) (inScope bool, decidable bool) {
	selectors := make(map[string]any, 2)
	if len(filterTags) > 0 {
		rawTags := metadata[metadataTagsKey]
		if tags.SelectorUnresolved(rawTags, leftDelim) {
			resolvedTags, ok := tags.ResolveSelectorValue(rawTags, data, leftDelim, rightDelim)
			if !ok {
				return true, false
			}
			rawTags = resolvedTags
		}
		selectors[metadataTagsKey] = rawTags
	}

	if len(filterLabels) > 0 {
		// Narrow BEFORE the unresolved check only when narrowing is safe:
		// a non-map labels value (templated scalar) or a templated map key
		// makes the selector undecidable — narrowing first would collapse the
		// unresolved marker into a decidable non-match and wrongly exclude
		// the component.
		narrowed, safe := tags.RequestedLabels(metadata[metadataLabelsKey], filterLabels, leftDelim)
		if !safe {
			return true, false
		}
		rawLabels := any(narrowed)
		if tags.SelectorUnresolved(rawLabels, leftDelim) {
			resolvedLabels, ok := tags.ResolveSelectorValue(rawLabels, data, leftDelim, rightDelim)
			if !ok {
				return true, false
			}
			rawLabels = resolvedLabels
		}
		selectors[metadataLabelsKey] = rawLabels
	}

	return inScopeByTagsAndLabels(selectors, filterTags, filterLabels, leftDelim)
}

// ensureComponentEntryInMap creates all intermediate maps in finalStacksMap so that
// finalStacksMap[stackName]["components"][typeName][componentName] exists as a map[string]any.
func ensureComponentEntryInMap(finalStacksMap map[string]any, stackName, typeName, componentName string) {
	stackEntry, ok := finalStacksMap[stackName].(map[string]any)
	if !ok {
		return
	}

	if !u.MapKeyExists(stackEntry, cfg.ComponentsSectionName) {
		stackEntry[cfg.ComponentsSectionName] = make(map[string]any)
	}
	comps, ok := stackEntry[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return
	}

	if !u.MapKeyExists(comps, typeName) {
		comps[typeName] = make(map[string]any)
	}
	typeMap, ok := comps[typeName].(map[string]any)
	if !ok {
		return
	}

	if !u.MapKeyExists(typeMap, componentName) {
		typeMap[componentName] = make(map[string]any)
	}
}

// getComponentDestMap safely traverses finalStacksMap to the component-level map.
// Returns (nil, false) if any level is missing or has an unexpected type.
func getComponentDestMap(finalStacksMap map[string]any, stackName, typeName, componentName string) (map[string]any, bool) {
	stackEntry, ok := finalStacksMap[stackName].(map[string]any)
	if !ok {
		return nil, false
	}
	comps, ok := stackEntry[cfg.ComponentsSectionName].(map[string]any)
	if !ok {
		return nil, false
	}
	typeMap, ok := comps[typeName].(map[string]any)
	if !ok {
		return nil, false
	}
	destMap, ok := typeMap[componentName].(map[string]any)
	return destMap, ok
}

// setAtmosComponentMetadata adds the five standard Atmos metadata keys to a section map.
func setAtmosComponentMetadata(section map[string]any, componentName, stackName, stackFileName string) {
	section["atmos_component"] = componentName
	section["atmos_stack"] = stackName
	section["stack"] = stackName
	section["atmos_stack_file"] = stackFileName
	section["atmos_manifest"] = stackFileName
}

// resolveIncludeEmpty reads the AtmosConfiguration to determine whether empty sections
// should be included in the output.  When checkIncludeEmpty is false (non-terraform types),
// it always returns true so that all sections are emitted.
func resolveIncludeEmpty(atmosConfig *schema.AtmosConfiguration, checkIncludeEmpty bool) bool {
	if !checkIncludeEmpty {
		return true
	}
	if atmosConfig.Describe.Settings.IncludeEmpty != nil {
		return *atmosConfig.Describe.Settings.IncludeEmpty
	}
	return true // default: include empty sections
}

// addSectionsToComponentEntry copies sections from componentSection into destMap,
// applying the optional section name filter and the includeEmpty rule.
func addSectionsToComponentEntry(
	destMap map[string]any,
	componentSection map[string]any,
	sections []string,
	includeEmpty bool,
) {
	for sectionName, section := range componentSection {
		if !includeEmpty {
			if sectionMap, ok := section.(map[string]any); ok && len(sectionMap) == 0 {
				continue
			}
		}
		if len(sections) == 0 || slices.Contains(sections, sectionName) {
			destMap[sectionName] = section
		}
	}
}

// processComponentSectionTemplates applies Go template processing to a component section
// and returns the rendered section as a map.
func processComponentSectionTemplates(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentSection map[string]any,
	settingsSection map[string]any,
) (map[string]any, error) {
	// Sections computed from Terraform source code (`component_info`) are not Atmos
	// configuration and must not be rendered as `Go` templates. They stay in the template
	// context below, only the rendered input excludes them. See #2145.
	templateInput, nonTemplatedSections := splitNonTemplatedSections(componentSection)

	componentSectionStr, err := atmosYaml.ConvertToYAMLPreservingDelimiters(
		templateInput,
		atmosConfig.Templates.Settings.Delimiters,
	)
	if err != nil {
		return nil, err
	}

	var settingsSectionStruct schema.Settings
	if err = mapstructure.Decode(settingsSection, &settingsSectionStruct); err != nil {
		return nil, err
	}

	// Restore env vars dropped by mapstructure's "-" tag.
	if envMap := extractEnvFromRawMap(settingsSection); len(envMap) > 0 {
		settingsSectionStruct.Templates.Settings.Env = envMap
	}

	componentTemplateContext := make(map[string]any, len(componentSection))
	for k, v := range componentSection {
		componentTemplateContext[k] = v
	}
	componentTemplateContext, err = manager.AddTemplateContext(
		atmosConfig,
		componentSectionStr,
		componentTemplateContext,
		manager.EffectiveTrackFromStack(atmosConfig, info),
	)
	if err != nil {
		return nil, err
	}

	processed, err := ProcessTmplWithDatasources(
		atmosConfig,
		info,
		settingsSectionStruct,
		"describe-stacks-all-sections",
		componentSectionStr,
		componentTemplateContext,
		true,
	)
	if err != nil {
		return nil, err
	}

	converted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](processed)
	if err != nil {
		if !atmosConfig.Templates.Settings.Enabled {
			if strings.Contains(componentSectionStr, "{{") || strings.Contains(componentSectionStr, "}}") {
				templateErr := errors.New( //nolint:err113 // User-facing hint with URL.
					"the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
						"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templates",
				)
				err = errors.Join(err, templateErr)
			}
		}
		return nil, err
	}

	restoreNonTemplatedSections(converted, nonTemplatedSections)

	return converted, nil
}

// processComponentSectionYAMLFunctions applies YAML function processing to a component section.
// When onWarning is non-nil, recoverable per-value errors are tolerated — see
// ProcessCustomYamlTagsLenient. For example, a Terraform backend might not yet be provisioned.
// The secretsMaskOnly parameter selects the inspection behavior that replaces !secret values without a backend lookup.
func processComponentSectionYAMLFunctions(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentSection map[string]any,
	skip []string,
	onWarning func(DegradationWarning),
	secretsMaskOnly bool,
) (map[string]any, error) {
	info.SecretsMaskOnly = secretsMaskOnly
	var converted map[string]any
	var err error
	if onWarning != nil {
		converted, err = ProcessCustomYamlTagsLenient(
			atmosConfig,
			componentSection,
			info.Stack,
			skip,
			info,
			onWarning,
		)
	} else {
		converted, err = ProcessCustomYamlTags(
			atmosConfig,
			componentSection,
			info.Stack,
			skip,
			info,
		)
	}
	if err != nil {
		return nil, err
	}
	return converted, nil
}

// applyTerraformMetadataInheritance resolves metadata from inherited base components
// and merges it into the component's own metadata.  This is terraform-specific behaviour
// triggered when atmos.yaml has stacks.inherit.metadata enabled.
//
// The workspace pattern/template cleanup always runs (regardless of whether an inherit
// list is present) so that any component with an explicit terraform_workspace consistently
// has pattern/template removed — matching the original behaviour of the old monolithic code.
// This prevents workspace derivation conflicts: without cleanup, both an explicit workspace
// AND a pattern/template would coexist and BuildTerraformWorkspace would use the pattern
// (checked first) instead of the explicit value.
func applyTerraformMetadataInheritance(
	atmosConfig *schema.AtmosConfiguration,
	allTerraformComponents map[string]any,
	componentName, stackFileName string,
	metadataSection map[string]any,
) (map[string]any, error) {
	inheritList, hasInherits := metadataSection[cfg.InheritsSectionName].([]any)

	if hasInherits && len(inheritList) > 0 { //nolint:nestif // Inheritance processing requires nested branching.
		baseComponentConfig := &schema.BaseComponentConfig{
			BaseComponentVars:      make(map[string]any),
			BaseComponentSettings:  make(map[string]any),
			BaseComponentEnv:       make(map[string]any),
			BaseComponentAuth:      make(map[string]any),
			BaseComponentSecrets:   make(map[string]any),
			BaseComponentMetadata:  make(map[string]any),
			BaseComponentProviders: make(map[string]any),
			BaseComponentHooks:     make(map[string]any),
		}
		baseComponents := []string{}

		for _, inheritValue := range inheritList {
			inheritFrom, ok := inheritValue.(string)
			if !ok {
				continue
			}
			if err := ProcessBaseComponentConfig(
				atmosConfig,
				atmosConfig,
				baseComponentConfig,
				allTerraformComponents,
				componentName,
				stackFileName,
				inheritFrom,
				"",
				false,
				&baseComponents,
			); err != nil {
				return nil, err
			}
		}

		if len(baseComponentConfig.BaseComponentMetadata) > 0 {
			merged, err := m.Merge(atmosConfig, []map[string]any{
				baseComponentConfig.BaseComponentMetadata, // base (lower priority)
				metadataSection, // component (higher priority)
			})
			if err != nil {
				return nil, err
			}
			metadataSection = merged
		}
	}

	// Always remove pattern/template when the component has an explicit terraform_workspace,
	// regardless of whether an inherit list is present.  This matches the original behaviour:
	// the cleanup ran unconditionally (outside the inheritList guard) in the old monolith.
	if _, hasExplicitWorkspace := metadataSection["terraform_workspace"].(string); hasExplicitWorkspace {
		// Shallow-clone first: unless the inheritance merge above already produced a
		// fresh map, metadataSection aliases the nested metadata map owned by the
		// shared FindStacksMap cache, and deleting from it in place would corrupt the
		// cache for every subsequent caller in the same process.
		metadataSection = maps.Clone(metadataSection)
		delete(metadataSection, "terraform_workspace_pattern")
		delete(metadataSection, "terraform_workspace_template")
	}

	return metadataSection, nil
}

// hasStackExplicitComponents reports whether a stack section contains any component
// entries under components.terraform, components.helmfile, components.packer, or
// components.ansible.
func hasStackExplicitComponents(stackSection map[string]any) bool {
	componentsSection, ok := stackSection[cfg.ComponentsSectionName]
	if !ok || componentsSection == nil {
		return false
	}
	comps, ok := componentsSection.(map[string]any)
	if !ok {
		return false
	}
	for _, typeName := range []string{
		cfg.TerraformSectionName,
		cfg.HelmfileSectionName,
		cfg.PackerSectionName,
		cfg.AnsibleSectionName,
		cfg.KubernetesSectionName,
		cfg.HelmSectionName,
		cfg.ContainerSectionName,
		cfg.EmulatorSectionName,
	} {
		if typeMap, ok := comps[typeName].(map[string]any); ok && len(typeMap) > 0 {
			return true
		}
	}
	return false
}

// hasStackImports reports whether a stack section has a non-empty "import" list.
func hasStackImports(stackSection map[string]any) bool {
	importsSection, ok := stackSection["import"].([]any)
	return ok && len(importsSection) > 0
}

// filterEmptyFinalStacks removes stacks from finalStacksMap that have no meaningful
// component content (respects includeEmptyStacks flag).
func filterEmptyFinalStacks(finalStacksMap map[string]any, includeEmptyStacks bool) error {
	if includeEmptyStacks {
		return nil
	}

	for stackName := range finalStacksMap {
		if stackName == "" {
			delete(finalStacksMap, stackName)
			continue
		}

		stackEntry, ok := finalStacksMap[stackName].(map[string]any)
		if !ok {
			return fmt.Errorf("invalid stack entry type for stack %s", stackName) //nolint:err113 // Dynamic context needed.
		}

		componentsSection, hasComponents := stackEntry[cfg.ComponentsSectionName].(map[string]any)
		if !hasComponents {
			delete(finalStacksMap, stackName)
			continue
		}

		if !stackHasNonEmptyComponents(componentsSection) {
			delete(finalStacksMap, stackName)
		}
	}
	return nil
}

// stackHasNonEmptyComponents returns true if any component within the componentsSection
// has at least one key in its content map. This avoids a section-name whitelist that
// could miss valid sections like backend, providers, hooks, overrides, or auth.
func stackHasNonEmptyComponents(componentsSection map[string]any) bool {
	for _, components := range componentsSection {
		compTypeMap, ok := components.(map[string]any)
		if !ok {
			continue
		}
		for _, comp := range compTypeMap {
			compContent, ok := comp.(map[string]any)
			if !ok {
				continue
			}
			if len(compContent) > 0 {
				return true
			}
		}
	}
	return false
}
