package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/locals"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExtractAndResolveLocals extracts and resolves locals from a section.
// Returns resolved locals map (merged with parent locals) or nil if no locals section.
// If there's an error resolving locals, it returns the error.
// The templateContext parameter provides additional context (like settings, vars) available during template resolution.
// The currentStack parameter is used for YAML function processing in locals.
func ExtractAndResolveLocals(
	atmosConfig *schema.AtmosConfiguration,
	section map[string]any,
	parentLocals map[string]any,
	filePath string,
	templateContext map[string]any,
	currentStack string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExtractAndResolveLocals")()

	if section == nil {
		return copyParentLocals(parentLocals), nil
	}

	// Check for locals section.
	localsRaw, exists := section[cfg.LocalsSectionName]
	if !exists {
		return copyParentLocals(parentLocals), nil
	}

	// Locals must be a map.
	localsMap, ok := localsRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w in %s", errUtils.ErrLocalsInvalidType, filePath)
	}

	// Handle empty locals section.
	if len(localsMap) == 0 {
		return copyOrCreateParentLocals(parentLocals), nil
	}

	// Resolve locals with dependency ordering and cycle detection.
	opts := localsResolveOptions{
		atmosConfig:     atmosConfig,
		currentStack:    currentStack,
		templateContext: templateContext,
	}
	return resolveLocalsWithDependencies(localsMap, parentLocals, filePath, opts)
}

// copyParentLocals creates a copy of parent locals or returns nil if no parent locals.
func copyParentLocals(parentLocals map[string]any) map[string]any {
	if parentLocals == nil {
		return nil
	}
	result := make(map[string]any, len(parentLocals))
	for k, v := range parentLocals {
		result[k] = v
	}
	return result
}

// copyOrCreateParentLocals creates a copy of parent locals or an empty map if nil.
func copyOrCreateParentLocals(parentLocals map[string]any) map[string]any {
	if parentLocals == nil {
		return make(map[string]any)
	}
	result := make(map[string]any, len(parentLocals))
	for k, v := range parentLocals {
		result[k] = v
	}
	return result
}

// localsResolveOptions contains options for resolving locals with dependencies.
type localsResolveOptions struct {
	atmosConfig     *schema.AtmosConfiguration
	currentStack    string
	templateContext map[string]any
}

// resolveLocalsWithDependencies resolves locals using dependency ordering and cycle detection.
// The options parameter provides additional context for YAML function and template processing.
func resolveLocalsWithDependencies(
	localsMap, parentLocals map[string]any,
	filePath string,
	opts localsResolveOptions,
) (map[string]any, error) {
	resolver := locals.NewResolver(localsMap, filePath).WithTemplateContext(opts.templateContext)

	// Add YAML function processor when atmosConfig is available.
	// This enables YAML functions like !terraform.state and !terraform.output in locals.
	if opts.atmosConfig != nil {
		yamlFuncProcessor := createYamlFunctionProcessor(opts.atmosConfig, opts.currentStack)
		resolver = resolver.WithYamlFunctionProcessor(yamlFuncProcessor)
	}

	resolved, err := resolver.Resolve(parentLocals)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// createYamlFunctionProcessor creates a callback for processing YAML function tags in locals.
func createYamlFunctionProcessor(atmosConfig *schema.AtmosConfiguration, currentStack string) locals.YamlFunctionProcessor {
	return func(value string) (any, error) {
		// Process YAML functions using the existing ProcessCustomYamlTags infrastructure.
		// We wrap the value in a map and process it.
		input := map[string]any{"_value": value}
		result, err := ProcessCustomYamlTags(atmosConfig, input, currentStack, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrLocalsYamlFunctionFailed, err)
		}
		// Extract the processed value.
		if processedValue, ok := result["_value"]; ok {
			return processedValue, nil
		}
		return nil, fmt.Errorf("%w: processed value missing", errUtils.ErrLocalsYamlFunctionFailed)
	}
}

// ProcessStackLocals extracts and resolves all locals from a stack config file.
// Returns a LocalsContext with resolved locals at each scope (global, terraform, helmfile, packer).
// Locals can reference .settings and .vars from the same file during template resolution.
// The currentStack parameter is used for YAML function processing (e.g., !terraform.state).
func ProcessStackLocals(
	atmosConfig *schema.AtmosConfiguration,
	stackConfigMap map[string]any,
	filePath string,
	currentStack string,
) (*LocalsContext, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStackLocals")()

	ctx := &LocalsContext{}

	// Build template context from the stack config map.
	// This allows locals to reference .settings, .vars, and other top-level sections from the same file.
	templateContext := buildTemplateContextFromConfig(stackConfigMap)

	// Extract global locals (available to all sections).
	globalLocals, err := ExtractAndResolveLocals(atmosConfig, stackConfigMap, nil, filePath, templateContext, currentStack)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve global locals: %w", err)
	}
	ctx.Global = globalLocals

	// Extract terraform section locals (inherit from global).
	if terraformSection, ok := stackConfigMap[cfg.TerraformSectionName].(map[string]any); ok {
		// Build section-specific template context (merge global with section-level settings/vars).
		sectionContext := buildSectionTemplateContext(templateContext, terraformSection)
		terraformLocals, err := ExtractAndResolveLocals(atmosConfig, terraformSection, ctx.Global, filePath, sectionContext, currentStack)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve terraform locals: %w", err)
		}
		ctx.Terraform = terraformLocals
		// Check if terraform section has its own locals key.
		if _, hasLocals := terraformSection[cfg.LocalsSectionName]; hasLocals {
			ctx.HasTerraformLocals = true
		}
	} else {
		ctx.Terraform = ctx.Global
	}

	// Extract helmfile section locals (inherit from global).
	if helmfileSection, ok := stackConfigMap[cfg.HelmfileSectionName].(map[string]any); ok {
		// Build section-specific template context (merge global with section-level settings/vars).
		sectionContext := buildSectionTemplateContext(templateContext, helmfileSection)
		helmfileLocals, err := ExtractAndResolveLocals(atmosConfig, helmfileSection, ctx.Global, filePath, sectionContext, currentStack)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve helmfile locals: %w", err)
		}
		ctx.Helmfile = helmfileLocals
		// Check if helmfile section has its own locals key.
		if _, hasLocals := helmfileSection[cfg.LocalsSectionName]; hasLocals {
			ctx.HasHelmfileLocals = true
		}
	} else {
		ctx.Helmfile = ctx.Global
	}

	// Extract packer section locals (inherit from global).
	if packerSection, ok := stackConfigMap[cfg.PackerSectionName].(map[string]any); ok {
		// Build section-specific template context (merge global with section-level settings/vars).
		sectionContext := buildSectionTemplateContext(templateContext, packerSection)
		packerLocals, err := ExtractAndResolveLocals(atmosConfig, packerSection, ctx.Global, filePath, sectionContext, currentStack)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve packer locals: %w", err)
		}
		ctx.Packer = packerLocals
		// Check if packer section has its own locals key.
		if _, hasLocals := packerSection[cfg.LocalsSectionName]; hasLocals {
			ctx.HasPackerLocals = true
		}
	} else {
		ctx.Packer = ctx.Global
	}

	return ctx, nil
}

// buildTemplateContextFromConfig extracts settings, vars, and other relevant sections
// from the stack config map to make them available during locals template resolution.
func buildTemplateContextFromConfig(stackConfigMap map[string]any) map[string]any {
	context := make(map[string]any)

	// Extract settings section if present.
	if settings, ok := stackConfigMap[cfg.SettingsSectionName].(map[string]any); ok {
		context[cfg.SettingsSectionName] = settings
	}

	// Extract vars section if present.
	if vars, ok := stackConfigMap[cfg.VarsSectionName].(map[string]any); ok {
		context[cfg.VarsSectionName] = vars
	}

	// Extract env section if present.
	if env, ok := stackConfigMap[cfg.EnvSectionName].(map[string]any); ok {
		context[cfg.EnvSectionName] = env
	}

	return context
}

// buildSectionTemplateContext merges global template context with section-specific settings/vars.
// Section-level values are merged with global values (not replaced), so global keys are preserved
// unless explicitly overridden by section-specific values.
func buildSectionTemplateContext(globalContext map[string]any, sectionConfig map[string]any) map[string]any {
	// Start with a copy of global context.
	context := make(map[string]any, len(globalContext))
	for k, v := range globalContext {
		context[k] = v
	}

	// Merge section-specific settings with global settings if present.
	if settings, ok := sectionConfig[cfg.SettingsSectionName].(map[string]any); ok {
		globalSettings, _ := globalContext[cfg.SettingsSectionName].(map[string]any)
		context[cfg.SettingsSectionName] = mergeStringAnyMaps(globalSettings, settings)
	}

	// Merge section-specific vars with global vars if present.
	if vars, ok := sectionConfig[cfg.VarsSectionName].(map[string]any); ok {
		globalVars, _ := globalContext[cfg.VarsSectionName].(map[string]any)
		context[cfg.VarsSectionName] = mergeStringAnyMaps(globalVars, vars)
	}

	// Merge section-specific env with global env if present.
	if env, ok := sectionConfig[cfg.EnvSectionName].(map[string]any); ok {
		globalEnv, _ := globalContext[cfg.EnvSectionName].(map[string]any)
		context[cfg.EnvSectionName] = mergeStringAnyMaps(globalEnv, env)
	}

	return context
}

// mergeStringAnyMaps performs a shallow merge of two maps, with overlay values taking precedence.
// Returns nil if both inputs are nil.
func mergeStringAnyMaps(base, overlay map[string]any) map[string]any {
	if base == nil && overlay == nil {
		return nil
	}
	result := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overlay {
		result[k] = v
	}
	return result
}

// LocalsContext holds resolved locals at different scopes within a stack file.
// This is used to pass locals context during template processing.
type LocalsContext struct {
	// Global holds locals defined at the stack file root level.
	Global map[string]any

	// Terraform holds locals from the terraform section (merged with global).
	Terraform map[string]any

	// Helmfile holds locals from the helmfile section (merged with global).
	Helmfile map[string]any

	// Packer holds locals from the packer section (merged with global).
	Packer map[string]any

	// HasTerraformLocals indicates the terraform section has its own locals defined.
	HasTerraformLocals bool

	// HasHelmfileLocals indicates the helmfile section has its own locals defined.
	HasHelmfileLocals bool

	// HasPackerLocals indicates the packer section has its own locals defined.
	HasPackerLocals bool
}

// MergeForComponentType returns the merged locals for a specific component type.
// This is what templates would see for a component of that type.
func (ctx *LocalsContext) MergeForComponentType(componentType string) map[string]any {
	defer perf.Track(nil, "exec.LocalsContext.MergeForComponentType")()

	if ctx == nil {
		return nil
	}

	switch componentType {
	case cfg.TerraformSectionName:
		return ctx.Terraform
	case cfg.HelmfileSectionName:
		return ctx.Helmfile
	case cfg.PackerSectionName:
		return ctx.Packer
	default:
		// For unknown types, return global only.
		return ctx.Global
	}
}

// MergeForTemplateContext merges all locals into a single flat map for template processing.
// Global locals are copied first, then section-specific locals override if explicitly defined.
//
// Precedence (later overrides earlier): Global → Terraform → Helmfile → Packer.
//
// Note: In practice, overlapping keys across sections is uncommon because components are
// single-typed (a component is either terraform, helmfile, or packer, not multiple).
// For component-specific processing, use MergeForComponentType instead, which only merges
// the relevant section for the component's type.
func (ctx *LocalsContext) MergeForTemplateContext() map[string]any {
	defer perf.Track(nil, "exec.LocalsContext.MergeForTemplateContext")()

	if ctx == nil {
		return nil
	}

	result := make(map[string]any)

	// Copy global locals first.
	for k, v := range ctx.Global {
		result[k] = v
	}

	// Merge section-specific locals only if explicitly defined.
	// Precedence: terraform → helmfile → packer (last wins for overlapping keys).
	ctx.mergeSectionLocals(result, ctx.Terraform, ctx.HasTerraformLocals)
	ctx.mergeSectionLocals(result, ctx.Helmfile, ctx.HasHelmfileLocals)
	ctx.mergeSectionLocals(result, ctx.Packer, ctx.HasPackerLocals)

	return result
}

// mergeSectionLocals merges section locals into the result map if hasLocals is true.
func (ctx *LocalsContext) mergeSectionLocals(result, sectionLocals map[string]any, hasLocals bool) {
	if !hasLocals {
		return
	}
	for k, v := range sectionLocals {
		result[k] = v
	}
}

// GetForComponentType returns the appropriate locals for a given component type.
// This is an alias for MergeForComponentType for API compatibility.
func (ctx *LocalsContext) GetForComponentType(componentType string) map[string]any {
	defer perf.Track(nil, "exec.LocalsContext.GetForComponentType")()

	return ctx.MergeForComponentType(componentType)
}

// ResolveComponentLocals resolves locals from a config section and merges with parent locals.
// This is used for component-level locals which inherit from stack-level locals and base components.
// The templateContext parameter provides additional context (like settings, vars) available during template resolution.
// The currentStack parameter is used for YAML function processing (e.g., !terraform.state).
func ResolveComponentLocals(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	parentLocals map[string]any,
	filePath string,
	templateContext map[string]any,
	currentStack string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ResolveComponentLocals")()

	return ExtractAndResolveLocals(atmosConfig, componentConfig, parentLocals, filePath, templateContext, currentStack)
}

// StripLocalsFromSection removes the locals section from a map.
// This is used to prevent locals from being merged across file boundaries
// and from appearing in the final component output.
func StripLocalsFromSection(section map[string]any) map[string]any {
	defer perf.Track(nil, "exec.StripLocalsFromSection")()

	if section == nil {
		return nil
	}

	// If no locals section, return as-is.
	if _, exists := section[cfg.LocalsSectionName]; !exists {
		return section
	}

	// Create a copy without locals.
	result := make(map[string]any, len(section)-1)
	for k, v := range section {
		if k != cfg.LocalsSectionName {
			result[k] = v
		}
	}
	return result
}
