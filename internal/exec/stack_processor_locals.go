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
func ExtractAndResolveLocals(
	atmosConfig *schema.AtmosConfiguration,
	section map[string]any,
	parentLocals map[string]any,
	filePath string,
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
	return resolveLocalsWithDependencies(localsMap, parentLocals, filePath)
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

// resolveLocalsWithDependencies resolves locals using dependency ordering and cycle detection.
func resolveLocalsWithDependencies(localsMap, parentLocals map[string]any, filePath string) (map[string]any, error) {
	resolver := locals.NewResolver(localsMap, filePath)
	resolved, err := resolver.Resolve(parentLocals)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// ProcessStackLocals extracts and resolves all locals from a stack config file.
// Returns a LocalsContext with resolved locals at each scope (global, terraform, helmfile, packer).
// Component-level locals are processed separately during component processing.
func ProcessStackLocals(
	atmosConfig *schema.AtmosConfiguration,
	stackConfigMap map[string]any,
	filePath string,
) (*LocalsContext, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStackLocals")()

	ctx := &LocalsContext{}

	// Extract global locals (available to all sections).
	globalLocals, err := ExtractAndResolveLocals(atmosConfig, stackConfigMap, nil, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve global locals: %w", err)
	}
	ctx.Global = globalLocals

	// Extract terraform section locals (inherit from global).
	if terraformSection, ok := stackConfigMap[cfg.TerraformSectionName].(map[string]any); ok {
		terraformLocals, err := ExtractAndResolveLocals(atmosConfig, terraformSection, ctx.Global, filePath)
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
		helmfileLocals, err := ExtractAndResolveLocals(atmosConfig, helmfileSection, ctx.Global, filePath)
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
		packerLocals, err := ExtractAndResolveLocals(atmosConfig, packerSection, ctx.Global, filePath)
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

// MergeForTemplateContext merges all locals into a single flat map for template processing.
// Global locals are copied first, then section-specific locals override if explicitly defined.
// MergeForComponentType returns the merged locals for a specific component type.
// This is what templates would see for a component of that type.
func (ctx *LocalsContext) MergeForComponentType(componentType string) map[string]any {
	defer perf.Track(nil, "exec.LocalsContext.MergeForComponentType")()

	if ctx == nil {
		return nil
	}

	switch componentType {
	case "terraform":
		return ctx.Terraform
	case "helmfile":
		return ctx.Helmfile
	case "packer":
		return ctx.Packer
	default:
		// For unknown types, return global only.
		return ctx.Global
	}
}

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
func (ctx *LocalsContext) GetForComponentType(componentType string) map[string]any {
	defer perf.Track(nil, "exec.LocalsContext.GetForComponentType")()

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
		return ctx.Global
	}
}

// ResolveComponentLocals resolves locals for a specific component.
// It merges component-level locals with the parent scope (component-type or global).
func ResolveComponentLocals(
	atmosConfig *schema.AtmosConfiguration,
	componentConfig map[string]any,
	parentLocals map[string]any,
	filePath string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ResolveComponentLocals")()

	return ExtractAndResolveLocals(atmosConfig, componentConfig, parentLocals, filePath)
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
