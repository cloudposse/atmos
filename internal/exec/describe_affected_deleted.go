package exec

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	comp "github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// deletableComponentTypes is the set of component types describe-affected evaluates for deletion
// detection. It must mirror the component types handled in the added/modified path
// (processStackAffected in describe_affected_utils_parallel.go); otherwise a deleted component of a
// type present there but missing here goes unreported. Native `helm` and `kubernetes` were
// originally omitted (#3199), and `ansible`, `container`, and `emulator` were added alongside the
// added/modified path in #3203. CloudFormation joins that canonical set alongside its dedicated
// added/modified processor; dependency lookup also includes these types.
var deletableComponentTypes = []string{
	cfg.TerraformComponentType,
	cfg.HelmfileComponentType,
	cfg.PackerComponentType,
	cfg.AnsibleComponentType,
	cfg.ContainerComponentType,
	cfg.EmulatorComponentType,
	cfg.KubernetesComponentType,
	cfg.HelmComponentType,
	cfg.CloudFormationComponentType,
}

// customAffectedComponentTypes returns, in sorted order, the component types present in a stack's
// components section that are not canonical (see deletableComponentTypes) but are registered with
// the component registry, such as custom kinds declared by custom commands. Sections of
// unregistered types are ignored, the same as describe stacks does. Both the added/modified path
// (processStackAffected) and the deleted path use this function, so they stay in sync. See #3208.
func customAffectedComponentTypes(componentsSection map[string]any) []string {
	var types []string
	for componentType, section := range componentsSection {
		if slices.Contains(deletableComponentTypes, componentType) {
			continue
		}
		if _, ok := section.(map[string]any); !ok {
			continue
		}
		if _, ok := comp.GetProvider(componentType); !ok {
			continue
		}
		types = append(types, componentType)
	}
	sort.Strings(types)
	return types
}

// customComponentBasePath returns the base path of a registered custom component type (a type
// not in deletableComponentTypes), as its provider reports it. The second result is false for
// canonical and unregistered types. The base path is empty when the provider has no source tree.
// Folder-change detection and component_path both use it, so they resolve the same folder. See #3208.
func customComponentBasePath(atmosConfig *schema.AtmosConfiguration, componentType string) (string, bool) {
	if slices.Contains(deletableComponentTypes, componentType) {
		return "", false
	}
	provider, ok := comp.GetProvider(componentType)
	if !ok {
		return "", false
	}
	return provider.GetBasePath(atmosConfig), true
}

// deletedComponentTypes returns the component types to evaluate for deletion in a BASE components
// section: the canonical types, then the registered custom types present in that section.
func deletedComponentTypes(remoteComponentsSection map[string]any) []string {
	return append(slices.Clone(deletableComponentTypes), customAffectedComponentTypes(remoteComponentsSection)...)
}

// detectDeletedComponents detects components and stacks that exist in BASE (remoteStacks)
// but have been deleted in HEAD (currentStacks).
// This enables CI/CD pipelines to identify resources that need terraform destroy.
//
// The `--tags` / `--labels` selectors in the filter are evaluated against the deleted component's
// metadata as it was in BASE, because a deleted component no longer has any metadata in HEAD.
// `ExcludeLocked` is applied to deleted components too, using their BASE metadata (`metadata.locked`),
// so a component that was locked when it was deleted is not reported.
func detectDeletedComponents(
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	stackToFilter string,
	filter AffectedFilter,
) ([]schema.Affected, error) {
	defer perf.Track(atmosConfig, "exec.detectDeletedComponents")()

	var deleted []schema.Affected

	// Iterate over BASE stacks to find deletions.
	for stackName, remoteStackSection := range *remoteStacks {
		// If --stack filter is provided, skip other stacks.
		if stackToFilter != "" && stackToFilter != stackName {
			continue
		}

		remoteStackMap, ok := remoteStackSection.(map[string]any)
		if !ok {
			continue
		}

		remoteComponentsSection, ok := remoteStackMap["components"].(map[string]any)
		if !ok {
			continue
		}

		// Check if the stack exists in HEAD.
		currentStackSection, stackExistsInHead := (*currentStacks)[stackName]

		if !stackExistsInHead {
			// Entire stack was deleted - add all non-abstract components.
			stackDeleted := processDeletedStack(
				stackName,
				remoteComponentsSection,
				atmosConfig,
				filter,
			)
			deleted = append(deleted, stackDeleted...)
		} else {
			// Stack exists but check for deleted components within.
			componentDeleted, err := processDeletedComponentsInStack(
				stackName,
				remoteComponentsSection,
				currentStackSection,
				atmosConfig,
				filter,
			)
			if err != nil {
				return nil, err
			}
			deleted = append(deleted, componentDeleted...)
		}
	}

	return deleted, nil
}

// processDeletedStack handles the case where an entire stack was deleted.
// All non-abstract components in the stack are marked as deleted with deletion_type: "stack".
func processDeletedStack(
	stackName string,
	remoteComponentsSection map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filter AffectedFilter,
) []schema.Affected {
	defer perf.Track(atmosConfig, "exec.processDeletedStack")()

	return processAllComponentsAsDeleted(
		stackName,
		remoteComponentsSection,
		atmosConfig,
		affectedReasonDeletedStack,
		deletionTypeStack,
		filter,
	)
}

// processAllComponentsAsDeleted marks all non-abstract components in a stack as deleted.
// This is used both when an entire stack is deleted and when a stack exists but has no components section.
func processAllComponentsAsDeleted(
	stackName string,
	remoteComponentsSection map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	affectedReason string,
	deletionType string,
	filter AffectedFilter,
) []schema.Affected {
	var deleted []schema.Affected

	// Process each component type.
	for _, componentType := range deletedComponentTypes(remoteComponentsSection) {
		componentTypeSection, ok := remoteComponentsSection[componentType].(map[string]any)
		if !ok {
			continue
		}

		for componentName, compSection := range componentTypeSection {
			componentSection, ok := compSection.(map[string]any)
			if !ok {
				continue
			}

			// Skip abstract components - they are not provisioned.
			if isAbstractComponent(componentSection) {
				continue
			}

			// Skip components that were locked in BASE when `--exclude-locked` is set.
			if deletedComponentExcludedAsLocked(componentSection, filter) {
				continue
			}

			// Skip components that do not match the `--tags` / `--labels` selectors (using their BASE metadata).
			if !deletedComponentMatchesSelectors(componentSection, filter) {
				continue
			}

			affected := createDeletedAffectedItem(&deletedItemParams{
				componentName:    componentName,
				stackName:        stackName,
				componentType:    componentType,
				componentSection: &componentSection,
				affectedReason:   affectedReason,
				deletionType:     deletionType,
				atmosConfig:      atmosConfig,
			})
			deleted = append(deleted, affected)
		}
	}

	return deleted
}

// processDeletedComponentsInStack handles the case where a stack exists but some components were deleted.
// Components that exist in BASE but not in HEAD are marked as deleted with deletion_type: "component".
//
//nolint:funlen,revive // function-length: logic is straightforward, splitting would reduce readability.
func processDeletedComponentsInStack(
	stackName string,
	remoteComponentsSection map[string]any,
	currentStackSection any,
	atmosConfig *schema.AtmosConfiguration,
	filter AffectedFilter,
) ([]schema.Affected, error) {
	defer perf.Track(atmosConfig, "exec.processDeletedComponentsInStack")()

	currentStackMap, ok := currentStackSection.(map[string]any)
	if !ok {
		return nil, nil
	}

	currentComponentsSection, ok := currentStackMap["components"].(map[string]any)
	if !ok {
		// Stack exists in HEAD but has no components section - all BASE components are deleted.
		// Use deletion_type: "component" (not "stack") since the stack itself still exists.
		return processAllComponentsAsDeleted(
			stackName,
			remoteComponentsSection,
			atmosConfig,
			affectedReasonDeleted,
			deletionTypeComponent,
			filter,
		), nil
	}

	var deleted []schema.Affected

	// Process each component type.
	for _, componentType := range deletedComponentTypes(remoteComponentsSection) {
		remoteTypeSection, ok := remoteComponentsSection[componentType].(map[string]any)
		if !ok {
			continue
		}

		currentTypeSection, _ := currentComponentsSection[componentType].(map[string]any)

		for componentName, compSection := range remoteTypeSection {
			componentSection, ok := compSection.(map[string]any)
			if !ok {
				continue
			}

			// Skip abstract components - they are not provisioned.
			if isAbstractComponent(componentSection) {
				continue
			}

			// Check if component exists in HEAD.
			if currentTypeSection != nil {
				if _, existsInHead := currentTypeSection[componentName]; existsInHead {
					// Component exists in HEAD - not deleted.
					continue
				}
			}

			// Skip components that were locked in BASE when `--exclude-locked` is set.
			if deletedComponentExcludedAsLocked(componentSection, filter) {
				continue
			}

			// Skip components that do not match the `--tags` / `--labels` selectors (using their BASE metadata).
			if !deletedComponentMatchesSelectors(componentSection, filter) {
				continue
			}

			// Component was deleted.
			affected := createDeletedAffectedItem(&deletedItemParams{
				componentName:    componentName,
				stackName:        stackName,
				componentType:    componentType,
				componentSection: &componentSection,
				affectedReason:   affectedReasonDeleted,
				deletionType:     deletionTypeComponent,
				atmosConfig:      atmosConfig,
			})
			deleted = append(deleted, affected)
		}
	}

	return deleted, nil
}

// deletedComponentExcludedAsLocked reports whether a deleted component must be skipped because
// `--exclude-locked` is set and the component was locked (`metadata.locked: true`) in BASE. The component
// section is the one from BASE, since the component no longer exists in HEAD. A component without a
// metadata section is never considered locked.
func deletedComponentExcludedAsLocked(componentSection map[string]any, filter AffectedFilter) bool {
	if !filter.ExcludeLocked {
		return false
	}

	metadataSection, ok := componentSection[sectionNameMetadata].(map[string]any)
	if !ok {
		return false
	}

	return isComponentLocked(metadataSection)
}

// deletedComponentMatchesSelectors reports whether a deleted component satisfies the `--tags` / `--labels`
// selectors of the filter. The component section is the one from BASE, since the component no longer
// exists in HEAD. Without selectors every deleted component matches; with selectors a component that has
// no metadata section cannot match (the same rule the added/modified path applies).
//
// It checks hasSelectors rather than selectorsApplyNow on purpose: DeferSelectors only defers the live
// components, because a deleted component has no HEAD metadata and no dependents to resolve later.
func deletedComponentMatchesSelectors(componentSection map[string]any, filter AffectedFilter) bool {
	if !filter.hasSelectors() {
		return true
	}

	metadataSection, ok := componentSection[sectionNameMetadata].(map[string]any)
	if !ok {
		return false
	}

	return filter.matchesSelectors(metadataSection)
}

// isAbstractComponent checks if a component has metadata.type = "abstract".
func isAbstractComponent(componentSection map[string]any) bool {
	metadataSection, ok := componentSection[sectionNameMetadata].(map[string]any)
	if !ok {
		return false
	}

	metadataType, ok := metadataSection["type"].(string)
	if !ok {
		return false
	}

	return metadataType == "abstract"
}

// deletedItemParams holds parameters for creating a deleted affected item.
type deletedItemParams struct {
	componentName    string
	stackName        string
	componentType    string
	componentSection *map[string]any
	affectedReason   string
	deletionType     string
	atmosConfig      *schema.AtmosConfiguration
}

// createDeletedAffectedItem creates an Affected item for a deleted component.
func createDeletedAffectedItem(params *deletedItemParams) schema.Affected {
	affected := schema.Affected{
		Component:     params.componentName,
		ComponentType: params.componentType,
		Stack:         params.stackName,
		Affected:      params.affectedReason,
		AffectedAll:   []string{params.affectedReason},
		Deleted:       true,
		DeletionType:  params.deletionType,
	}

	// Build component path from the BASE component section.
	affected.ComponentPath = BuildComponentPath(params.atmosConfig, params.componentSection, params.componentType, params.componentName)
	affected.StackSlug = fmt.Sprintf("%s-%s", params.stackName, strings.ReplaceAll(params.componentName, "/", "-"))

	// Extract metadata from the component's vars section (same as non-deleted items).
	if params.componentSection != nil {
		populateDeletedItemMetadata(&affected, params)
	}

	return affected
}

// populateDeletedItemMetadata extracts and populates metadata fields from the component section.
func populateDeletedItemMetadata(affected *schema.Affected, params *deletedItemParams) {
	componentSection := *params.componentSection

	// Extract vars section and use GetContextFromVars for consistency with the rest of the codebase.
	varsSection, ok := componentSection[cfg.VarsSectionName].(map[string]any)
	if !ok {
		return
	}

	// Use cfg.GetContextFromVars for consistency with spacelift_utils.go and atlantis_utils.go.
	context := cfg.GetContextFromVars(varsSection)

	// Populate context metadata fields.
	affected.Namespace = context.Namespace
	affected.Tenant = context.Tenant
	affected.Environment = context.Environment
	affected.Stage = context.Stage

	// For Terraform components, also populate Spacelift stack and Atlantis project names.
	if params.componentType == cfg.TerraformComponentType {
		populateDeletedItemIntegrations(affected, params, varsSection, componentSection)
	}
}

// populateDeletedItemIntegrations populates Spacelift and Atlantis names for deleted Terraform components.
func populateDeletedItemIntegrations(
	affected *schema.Affected,
	params *deletedItemParams,
	varsSection map[string]any,
	componentSection map[string]any,
) {
	settingsSection, _ := componentSection[cfg.SettingsSectionName].(map[string]any)

	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg:         params.componentName,
		Stack:                    params.stackName,
		ComponentVarsSection:     varsSection,
		ComponentSettingsSection: settingsSection,
		ComponentSection: map[string]any{
			cfg.VarsSectionName:     varsSection,
			cfg.SettingsSectionName: settingsSection,
		},
	}

	// Build Spacelift stack name (ignore errors - field is optional).
	if spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(params.atmosConfig, configAndStacksInfo); err == nil {
		affected.SpaceliftStack = spaceliftStackName
	}

	// Build Atlantis project name (ignore errors - field is optional).
	if atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(params.atmosConfig, configAndStacksInfo); err == nil {
		affected.AtlantisProject = atlantisProjectName
	}
}
