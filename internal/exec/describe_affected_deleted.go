package exec

import (
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// detectDeletedComponents detects components and stacks that exist in BASE (remoteStacks)
// but have been deleted in HEAD (currentStacks).
// This enables CI/CD pipelines to identify resources that need terraform destroy.
func detectDeletedComponents(
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	stackToFilter string,
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
			stackDeleted, err := processDeletedStack(
				stackName,
				remoteComponentsSection,
				atmosConfig,
			)
			if err != nil {
				return nil, err
			}
			deleted = append(deleted, stackDeleted...)
		} else {
			// Stack exists but check for deleted components within.
			componentDeleted, err := processDeletedComponentsInStack(
				stackName,
				remoteComponentsSection,
				currentStackSection,
				atmosConfig,
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
) ([]schema.Affected, error) {
	defer perf.Track(atmosConfig, "exec.processDeletedStack")()

	var deleted []schema.Affected

	// Process each component type.
	for _, componentType := range []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType} {
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

			affected := createDeletedAffectedItem(&deletedItemParams{
				componentName:    componentName,
				stackName:        stackName,
				componentType:    componentType,
				componentSection: &componentSection,
				affectedReason:   affectedReasonDeletedStack,
				deletionType:     deletionTypeStack,
				atmosConfig:      atmosConfig,
			})
			deleted = append(deleted, affected)
		}
	}

	return deleted, nil
}

// processDeletedComponentsInStack handles the case where a stack exists but some components were deleted.
// Components that exist in BASE but not in HEAD are marked as deleted with deletion_type: "component".
func processDeletedComponentsInStack(
	stackName string,
	remoteComponentsSection map[string]any,
	currentStackSection any,
	atmosConfig *schema.AtmosConfiguration,
) ([]schema.Affected, error) {
	defer perf.Track(atmosConfig, "exec.processDeletedComponentsInStack")()

	var deleted []schema.Affected

	currentStackMap, ok := currentStackSection.(map[string]any)
	if !ok {
		return deleted, nil
	}

	currentComponentsSection, ok := currentStackMap["components"].(map[string]any)
	if !ok {
		// No components section in HEAD means all BASE components are deleted.
		return processDeletedStack(stackName, remoteComponentsSection, atmosConfig)
	}

	// Process each component type.
	for _, componentType := range []string{cfg.TerraformComponentType, cfg.HelmfileComponentType, cfg.PackerComponentType} {
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

	// Extract vars section and decode to Context for metadata fields.
	varsSection, ok := componentSection[cfg.VarsSectionName].(map[string]any)
	if !ok {
		return
	}

	var context schema.Context
	if err := mapstructure.Decode(varsSection, &context); err != nil {
		// If decoding fails, skip metadata population but continue.
		return
	}

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
