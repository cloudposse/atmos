//nolint:revive // file-length-limit: 570 lines, slightly above 500. Will split in follow-up.
package exec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
	includeEmptyStacks   bool
	skip                 []string
	authManager          auth.AuthManager
	finalStacksMap       map[string]any
	// componentAuthResolver builds a per-component AuthManager; defaults to
	// createComponentAuthManager and is overridable in tests.
	componentAuthResolver componentAuthManagerResolver
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
	return &describeStacksProcessor{
		atmosConfig:           atmosConfig,
		filterByStack:         filterByStack,
		components:            components,
		sections:              sections,
		componentTypes:        componentTypes,
		processTemplates:      processTemplates,
		processYamlFunctions:  processYamlFunctions,
		includeEmptyStacks:    includeEmptyStacks,
		skip:                  skip,
		authManager:           authManager,
		finalStacksMap:        make(map[string]any),
		componentAuthResolver: createComponentAuthManager,
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
// declare its own default identity in its auth section. Any error from the
// resolver is swallowed and the parent AuthManager is used — this preserves the
// original swallow-on-error behavior of the inline code that was refactored.
func (p *describeStacksProcessor) resolveComponentAuthManager(
	componentSection map[string]any,
	componentName, stackName string,
) auth.AuthManager {
	componentAuthManager := p.authManager
	if !shouldResolvePerComponentAuth(p.processTemplates, p.processYamlFunctions) {
		return componentAuthManager
	}
	authSection, hasAuth := componentSection[cfg.AuthSectionName].(map[string]any)
	if !hasAuth || !hasDefaultIdentity(authSection) {
		return componentAuthManager
	}
	resolver := p.componentAuthResolver
	if resolver == nil {
		resolver = createComponentAuthManager
	}
	resolved, createErr := resolver(p.atmosConfig, componentSection, componentName, stackName, p.authManager)
	if createErr == nil && resolved != nil {
		componentAuthManager = resolved
	}
	return componentAuthManager
}

// processStackFile processes one stack file, iterating over all requested component types.
func (p *describeStacksProcessor) processStackFile(stackFileName string, stackMap map[string]any) error { //nolint:revive // cyclomatic: pre-creation guard adds unavoidable branches.
	defer perf.Track(p.atmosConfig, "exec.describeStacksProcessor.processStackFile")()

	// Read manifest name before deleting imports — getStackManifestName reads "name",
	// not "imports", but keeping reads before mutations avoids implicit ordering assumptions.
	stackManifestName := getStackManifestName(stackMap)

	// Delete the stack-wide imports section (not needed in output).
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
	}

	for _, te := range typeEntries {
		if len(p.componentTypes) > 0 && !u.SliceContainsString(p.componentTypes, te.name) {
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

	// Resolve the per-component auth manager (may fall back to the parent).
	componentAuthManager := p.resolveComponentAuthManager(componentSection, componentName, stackName)
	propagateAuth(&info, componentAuthManager)

	// Filter: skip this component if it does not belong to the requested stack.
	if shouldFilterByStack(p.filterByStack, stackFileName, stackName) {
		return nil
	}

	if stackName == "" {
		stackName = stackFileName
	}

	// Filter: skip this component if it does not match the requested component list.
	// This check is performed before any mutations to componentSection so that
	// the live stacksMap data is not modified for filtered-out components.
	componentIncluded := len(p.components) == 0 ||
		u.SliceContainsString(p.components, componentName) ||
		u.SliceContainsString(derivedComponents, componentName)
	if !componentIncluded {
		return nil
	}

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
		componentSection, err = processComponentSectionYAMLFunctions(p.atmosConfig, &info, componentSection, p.skip)
		if err != nil {
			return err
		}
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

// shouldFilterByStack returns true when the component should be skipped because it
// does not belong to the requested stack filter.  An empty filterByStack means no filtering.
func shouldFilterByStack(filterByStack, stackFileName, stackName string) bool {
	return filterByStack != "" && filterByStack != stackFileName && filterByStack != stackName
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
		if len(sections) == 0 || u.SliceContainsString(sections, sectionName) {
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
	componentSectionStr, err := atmosYaml.ConvertToYAMLPreservingDelimiters(
		componentSection,
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

	processed, err := ProcessTmplWithDatasources(
		atmosConfig,
		info,
		settingsSectionStruct,
		"describe-stacks-all-sections",
		componentSectionStr,
		info.ComponentSection,
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
	return converted, nil
}

// processComponentSectionYAMLFunctions applies YAML function processing to a component section.
func processComponentSectionYAMLFunctions(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	componentSection map[string]any,
	skip []string,
) (map[string]any, error) {
	converted, err := ProcessCustomYamlTags(
		atmosConfig,
		componentSection,
		info.Stack,
		skip,
		info,
	)
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
