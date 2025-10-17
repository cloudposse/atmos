//nolint:gocognit,revive,nestif // Complex component processing logic requires nested conditionals
package exec

import (
	"reflect"

	"github.com/mitchellh/mapstructure"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Affected reason constants.
const (
	affectedReasonStackMetadata   = "stack.metadata"
	affectedReasonComponent       = "component"
	affectedReasonComponentModule = "component.module"
	affectedReasonStackVars       = "stack.vars"
	affectedReasonStackEnv        = "stack.env"
	affectedReasonStackSettings   = "stack.settings"
)

// Section name constants for isEqual comparisons.
const (
	sectionNameMetadata = "metadata"
	sectionNameVars     = "vars"
	sectionNameEnv      = "env"
)

// shouldSkipComponent determines if a component should be skipped based on metadata.
func shouldSkipComponent(metadataSection map[string]any, componentName string, excludeLocked bool) bool {
	// Skip abstract components.
	if metadataType, ok := metadataSection["type"].(string); ok {
		if metadataType == "abstract" {
			return true
		}
	}

	// Skip disabled components.
	if !isComponentEnabled(metadataSection, componentName) {
		return true
	}

	// Skip locked components if requested.
	if excludeLocked && isComponentLocked(metadataSection) {
		return true
	}

	return false
}

// addAffectedComponent adds an affected component to the list.
// This is a thread-safe helper that doesn't modify shared state.
func addAffectedComponent(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	affectedReason string,
	includeSpaceliftAdminStacks bool,
	currentStacks *map[string]any,
	includeSettings bool,
) error {
	affectedItem := schema.Affected{
		ComponentType: componentType,
		Component:     componentName,
		Stack:         stackName,
		Affected:      affectedReason,
	}

	// Append to the local slice (thread-safe as each goroutine has its own slice).
	return appendToAffected(
		atmosConfig,
		componentName,
		stackName,
		componentSection,
		affected,
		&affectedItem,
		includeSpaceliftAdminStacks,
		currentStacks,
		includeSettings,
	)
}

// processTerraformComponentsIndexed processes Terraform components using the files index.
//
//nolint:cyclop,funlen // Component processing requires checking multiple sections (metadata, vars, env, settings, modules)
func processTerraformComponentsIndexed(
	stackName string,
	terraformSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range terraformSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		// Check metadata section and skip if needed.
		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			// Check metadata changes.
			if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
					&componentSection, affectedReasonStackMetadata, includeSpaceliftAdminStacks, currentStacks, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		// Check component folder and module changes.
		if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
			// Check terraform modules.
			changed, err := areTerraformComponentModulesChangedIndexed(component, atmosConfig, filesIndex, patternCache)
			if err != nil {
				return nil, err
			}
			if changed {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
					&componentSection, affectedReasonComponentModule, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}

			// Check component folder changes.
			changed, err = isComponentFolderChangedIndexed(component, cfg.TerraformComponentType, atmosConfig, filesIndex, patternCache)
			if err != nil {
				return nil, err
			}
			if changed {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
					&componentSection, affectedReasonComponent, includeSpaceliftAdminStacks, currentStacks, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		// Check vars, env, settings sections (same as non-indexed version).
		if varSection, ok := componentSection[sectionNameVars].(map[string]any); ok {
			if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, varSection, sectionNameVars) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
					&componentSection, affectedReasonStackVars, includeSpaceliftAdminStacks, currentStacks, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if envSection, ok := componentSection[sectionNameEnv].(map[string]any); ok {
			if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, envSection, sectionNameEnv) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
					&componentSection, affectedReasonStackEnv, includeSpaceliftAdminStacks, currentStacks, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

// processHelmfileComponentsIndexed processes Helmfile components using the files index.
//
//nolint:cyclop,dupl,funlen // Similar structure to processPackerComponentsIndexed but for different component type
func processHelmfileComponentsIndexed(
	stackName string,
	helmfileSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range helmfileSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
					&componentSection, affectedReasonStackMetadata, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
			changed, err := isComponentFolderChangedIndexed(component, cfg.HelmfileComponentType, atmosConfig, filesIndex, patternCache)
			if err != nil {
				return nil, err
			}
			if changed {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
					&componentSection, affectedReasonComponent, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if varSection, ok := componentSection[sectionNameVars].(map[string]any); ok {
			if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, varSection, sectionNameVars) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
					&componentSection, affectedReasonStackVars, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if envSection, ok := componentSection[sectionNameEnv].(map[string]any); ok {
			if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, envSection, sectionNameEnv) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
					&componentSection, affectedReasonStackEnv, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

// processPackerComponentsIndexed processes Packer components using the files index.
//
//nolint:cyclop,dupl,funlen // Similar structure to processHelmfileComponentsIndexed but for different component type
func processPackerComponentsIndexed(
	stackName string,
	packerSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range packerSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
					&componentSection, affectedReasonStackMetadata, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if component, ok := componentSection[cfg.ComponentSectionName].(string); ok && component != "" {
			changed, err := isComponentFolderChangedIndexed(component, cfg.PackerComponentType, atmosConfig, filesIndex, patternCache)
			if err != nil {
				return nil, err
			}
			if changed {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
					&componentSection, affectedReasonComponent, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if varSection, ok := componentSection[sectionNameVars].(map[string]any); ok {
			if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, varSection, sectionNameVars) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
					&componentSection, affectedReasonStackVars, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if envSection, ok := componentSection[sectionNameEnv].(map[string]any); ok {
			if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, envSection, sectionNameEnv) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
					&componentSection, affectedReasonStackEnv, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

// checkSettingsAndDependenciesIndexed checks settings using indexed files.
func checkSettingsAndDependenciesIndexed(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	settingsSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	filesIndex *changedFilesIndex,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
) error {
	// Check settings section changes.
	if !isEqual(remoteStacks, stackName, componentType, componentName, settingsSection, cfg.SettingsSectionName) {
		err := addAffectedComponent(affected, atmosConfig, componentName, stackName, componentType,
			componentSection, affectedReasonStackSettings, includeSpaceliftAdminStacks, currentStacks, includeSettings)
		if err != nil {
			return err
		}
	}

	// Check settings.depends_on using indexed version.
	return checkDependencyChangesIndexed(
		affected, atmosConfig, componentName, stackName, componentType,
		componentSection, settingsSection, filesIndex,
		includeSpaceliftAdminStacks, currentStacks, includeSettings,
	)
}

// checkDependencyChangesIndexed checks if dependent files or folders have changed.
// This helper reduces cyclomatic complexity of checkSettingsAndDependenciesIndexed.
func checkDependencyChangesIndexed(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	settingsSection map[string]any,
	filesIndex *changedFilesIndex,
	includeSpaceliftAdminStacks bool,
	currentStacks *map[string]any,
	includeSettings bool,
) error {
	var stackComponentSettings schema.Settings
	err := mapstructure.Decode(settingsSection, &stackComponentSettings)
	if err != nil {
		return err
	}

	if reflect.ValueOf(stackComponentSettings).IsZero() ||
		reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() {
		return nil
	}

	isFolderOrFileChanged, changedType, changedFileOrFolder, err := isComponentDependentFolderOrFileChangedIndexed(
		filesIndex,
		stackComponentSettings.DependsOn,
	)
	if err != nil {
		return err
	}

	if !isFolderOrFileChanged {
		return nil
	}

	return addDependencyAffectedItem(
		affected, atmosConfig, componentName, stackName, componentType,
		componentSection, changedType, changedFileOrFolder,
		includeSpaceliftAdminStacks, currentStacks, includeSettings,
	)
}

// addDependencyAffectedItem adds an affected item for a dependency change.
// This helper further reduces complexity by handling the affected item creation.
func addDependencyAffectedItem(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	changedType string,
	changedFileOrFolder string,
	includeSpaceliftAdminStacks bool,
	currentStacks *map[string]any,
	includeSettings bool,
) error {
	changedFile := ""
	if changedType == "file" {
		changedFile = changedFileOrFolder
	}

	changedFolder := ""
	if changedType == "folder" {
		changedFolder = changedFileOrFolder
	}

	affectedItem := schema.Affected{
		ComponentType: componentType,
		Component:     componentName,
		Stack:         stackName,
		Affected:      changedType,
		File:          changedFile,
		Folder:        changedFolder,
	}

	return appendToAffected(
		atmosConfig,
		componentName,
		stackName,
		componentSection,
		affected,
		&affectedItem,
		includeSpaceliftAdminStacks,
		currentStacks,
		includeSettings,
	)
}
