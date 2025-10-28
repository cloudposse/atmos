package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// appendToAffected adds an item to the affected list, and adds the Spacelift stack and Atlantis project (if configured).
func appendToAffected(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentSection *map[string]any,
	affectedList *[]schema.Affected,
	affected *schema.Affected,
	includeSpaceliftAdminStacks bool,
	stacks *map[string]any,
	includeSettings bool,
) error {
	// Append the affected section to the `affected_all` slice.
	affected.AffectedAll = append(affected.AffectedAll, affected.Affected)

	// If the affected component in the stack was already added to the result, don't add it again
	for i := range *affectedList {
		v := &(*affectedList)[i]
		if v.Component == affected.Component && v.Stack == affected.Stack && v.ComponentType == affected.ComponentType {
			// For the found item in the list, append the affected section to the `affected_all` slice.
			v.AffectedAll = append(v.AffectedAll, affected.Affected)
			return nil
		}
	}

	settingsSection := map[string]any{}

	if i, ok2 := (*componentSection)[cfg.SettingsSectionName]; ok2 {
		settingsSection = i.(map[string]any)

		if includeSettings {
			affected.Settings = settingsSection
		}
	}

	if affected.ComponentType == cfg.TerraformComponentType {
		varSection := map[string]any{}

		if i, ok2 := (*componentSection)[cfg.VarsSectionName]; ok2 {
			varSection = i.(map[string]any)
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:         componentName,
			Stack:                    stackName,
			ComponentVarsSection:     varSection,
			ComponentSettingsSection: settingsSection,
			ComponentSection: map[string]any{
				cfg.VarsSectionName:     varSection,
				cfg.SettingsSectionName: settingsSection,
			},
		}

		// Affected Spacelift stack
		spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
		if err != nil {
			return err
		}
		affected.SpaceliftStack = spaceliftStackName

		// Affected Atlantis project
		atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
		if err != nil {
			return err
		}
		affected.AtlantisProject = atlantisProjectName

		if includeSpaceliftAdminStacks {
			affectedList, err = addAffectedSpaceliftAdminStack(
				atmosConfig,
				affectedList,
				&settingsSection,
				stacks,
				stackName,
				componentName,
				&configAndStacksInfo,
				includeSettings,
			)
			if err != nil {
				return err
			}
		}
	}

	// Check the `component` section and add `ComponentPath` to the output.
	affected.ComponentPath = BuildComponentPath(atmosConfig, componentSection, affected.ComponentType)
	affected.StackSlug = fmt.Sprintf("%s-%s", stackName, strings.Replace(componentName, "/", "-", -1))

	*affectedList = append(*affectedList, *affected)
	return nil
}

// isEqual compares a section of a component from the remote stacks with a section of a local component.
// Uses optimized custom deep comparison instead of reflect.DeepEqual for 15-25% improvement.
func isEqual(
	remoteStacks *map[string]any,
	localStackName string,
	componentType string,
	localComponentName string,
	localSection map[string]any,
	sectionName string,
) bool {
	if remoteStackSection, ok := (*remoteStacks)[localStackName].(map[string]any); ok {
		if remoteComponentsSection, ok := remoteStackSection["components"].(map[string]any); ok {
			if remoteComponentTypeSection, ok := remoteComponentsSection[componentType].(map[string]any); ok {
				if remoteComponentSection, ok := remoteComponentTypeSection[localComponentName].(map[string]any); ok {
					if remoteSection, ok := remoteComponentSection[sectionName].(map[string]any); ok {
						return deepEqualMaps(localSection, remoteSection)
					}
				}
			}
		}
	}
	return false
}

// deepEqualMaps performs optimized deep comparison of two maps.
// This avoids the overhead of reflect.DeepEqual by using type assertions.
// Correctly distinguishes between nil and empty maps to match reflect.DeepEqual behavior.
func deepEqualMaps(a, b map[string]any) bool {
	// Check if exactly one is nil (XOR).
	// This preserves reflect.DeepEqual behavior where nil != empty map.
	if (a == nil) != (b == nil) {
		return false
	}

	// Quick length check.
	if len(a) != len(b) {
		return false
	}

	// Compare all keys and values.
	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists {
			return false
		}

		if !deepEqualValues(valueA, valueB) {
			return false
		}
	}

	return true
}

// deepEqualValues recursively compares two values of any type.
// Optimized for common types in Atmos configurations.
//
//nolint:cyclop,funlen,revive // Type switch for deep comparison requires explicit handling of all common types
func deepEqualValues(a, b any) bool {
	// Handle nil cases.
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Type assertions for common types to avoid reflection.
	switch aTyped := a.(type) {
	case map[string]any:
		bTyped, ok := b.(map[string]any)
		if !ok {
			return false
		}
		return deepEqualMaps(aTyped, bTyped)

	case []any:
		bTyped, ok := b.([]any)
		if !ok {
			return false
		}
		return deepEqualSlices(aTyped, bTyped)

	case string:
		bTyped, ok := b.(string)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case int:
		bTyped, ok := b.(int)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case int64:
		bTyped, ok := b.(int64)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case float64:
		bTyped, ok := b.(float64)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case bool:
		bTyped, ok := b.(bool)
		if !ok {
			return false
		}
		return aTyped == bTyped

	default:
		// Fallback to reflect.DeepEqual for uncommon types.
		return reflect.DeepEqual(a, b)
	}
}

// deepEqualSlices compares two slices recursively.
// Correctly distinguishes between nil and empty slices to match reflect.DeepEqual behavior.
func deepEqualSlices(a, b []any) bool {
	// Check if exactly one is nil (XOR).
	// This preserves reflect.DeepEqual behavior where nil != empty slice.
	if (a == nil) != (b == nil) {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !deepEqualValues(a[i], b[i]) {
			return false
		}
	}

	return true
}

// isComponentFolderChanged checks if the component folder changed (has changed files in the folder or its subfolders).
func isComponentFolderChanged(
	component string,
	componentType string,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
) (bool, error) {
	var componentPath string

	switch componentType {
	case cfg.TerraformComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)
	case cfg.HelmfileComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath, component)
	case cfg.PackerComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath, component)
	default:
		return false, fmt.Errorf("%w: %s", errUtils.ErrUnsupportedComponentType, componentType)
	}

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return false, err
	}

	componentPathPattern := filepath.Join(componentPathAbs, "**")

	for _, changedFile := range changedFiles {
		changedFileAbs, err := filepath.Abs(changedFile)
		if err != nil {
			return false, err
		}

		match, err := u.PathMatch(componentPathPattern, changedFileAbs)
		if err != nil {
			return false, err
		}

		if match {
			return true, nil
		}
	}

	return false, nil
}

// areTerraformComponentModulesChanged checks if any of the external Terraform modules (but on the local filesystem) that the component uses have changed.
func areTerraformComponentModulesChanged(
	component string,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
) (bool, error) {
	componentPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return false, err
	}

	terraformConfiguration, diags := tfconfig.LoadModule(componentPathAbs)
	if diags.HasErrors() {
		diagErr := diags.Err()

		// Try structured error detection first (most robust).
		if errors.Is(diagErr, os.ErrNotExist) || errors.Is(diagErr, fs.ErrNotExist) {
			return false, nil
		}

		// Fallback to error message inspection for cases where tfconfig doesn't wrap errors properly.
		// This handles missing subdirectory modules (e.g., ./modules/security-group referenced in main.tf
		// but the directory doesn't exist). Such missing paths are valid in affected detection—components
		// or their modules may be deleted or not yet created when tracking changes over time.
		errMsg := diagErr.Error()
		if strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "Failed to read directory") {
			return false, nil
		}

		// For other errors (syntax errors, permission issues, etc.), return error.
		return false, errors.Join(errUtils.ErrFailedToLoadTerraformModule, diagErr)
	}

	// If no configuration, there are no modules to check.
	if terraformConfiguration == nil {
		return false, nil
	}

	for _, changedFile := range changedFiles {
		changedFileAbs, err := filepath.Abs(changedFile)
		if err != nil {
			return false, err
		}

		for _, moduleConfig := range terraformConfiguration.ModuleCalls {
			// We are processing the local modules only (not from terraform registry), they will have `Version` as an empty string
			if moduleConfig.Version != "" {
				continue
			}

			modulePath := filepath.Join(filepath.Dir(moduleConfig.Pos.Filename), moduleConfig.Source)

			modulePathAbs, err := filepath.Abs(modulePath)
			if err != nil {
				return false, err
			}

			modulePathPattern := filepath.Join(modulePathAbs, "**")

			match, err := u.PathMatch(modulePathPattern, changedFileAbs)
			if err != nil {
				return false, err
			}

			if match {
				return true, nil
			}
		}
	}

	return false, nil
}

// addAffectedSpaceliftAdminStack adds the affected Spacelift admin stack that manages the affected child stack.
func addAffectedSpaceliftAdminStack(
	atmosConfig *schema.AtmosConfiguration,
	affectedList *[]schema.Affected,
	settingsSection *map[string]any,
	stacks *map[string]any,
	currentStackName string,
	currentComponentName string,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	includeSettings bool,
) (*[]schema.Affected, error) {
	// Convert the `settings` section to the `Settings` structure
	var componentSettings schema.Settings
	err := mapstructure.Decode(settingsSection, &componentSettings)
	if err != nil {
		return nil, err
	}

	// Skip if the component has an empty `settings.spacelift` section
	if reflect.ValueOf(componentSettings).IsZero() ||
		reflect.ValueOf(componentSettings.Spacelift).IsZero() {
		return affectedList, nil
	}

	// Find and process `settings.spacelift.admin_stack_config` section
	var adminStackContextSection any
	var adminStackContext schema.Context
	var ok bool

	if adminStackContextSection, ok = componentSettings.Spacelift["admin_stack_selector"]; !ok {
		return affectedList, nil
	}

	err = mapstructure.Decode(adminStackContextSection, &adminStackContext)
	if err != nil {
		return nil, err
	}

	// Skip if the component has an empty `settings.spacelift.admin_stack_selector` section
	if reflect.ValueOf(adminStackContext).IsZero() {
		return affectedList, nil
	}

	var adminStackContextPrefix string

	if atmosConfig.Stacks.NameTemplate != "" {
		adminStackContextPrefix, err = ProcessTmpl(atmosConfig, "spacelift-admin-stack-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
		if err != nil {
			return nil, err
		}
	} else {
		adminStackContextPrefix, err = cfg.GetContextPrefix(currentStackName, adminStackContext, GetStackNamePattern(atmosConfig), currentStackName)
		if err != nil {
			return nil, err
		}
	}

	var componentVarsSection map[string]any
	var componentSettingsSection map[string]any
	var componentSettingsSpaceliftSection map[string]any

	// Find the Spacelift admin stack that manages the current stack
	if stacks == nil {
		return affectedList, nil
	}
	for stackName, stackSection := range *stacks {
		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {
				if terraformSection, ok := componentsSection[cfg.TerraformComponentType].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {

							if componentVarsSection, ok = componentSection["vars"].(map[string]any); !ok {
								return affectedList, nil
							}

							var context schema.Context
							err = mapstructure.Decode(componentVarsSection, &context)
							if err != nil {
								return nil, err
							}

							var contextPrefix string

							if atmosConfig.Stacks.NameTemplate != "" {
								contextPrefix, err = ProcessTmpl(atmosConfig, "spacelift-stack-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
								if err != nil {
									return nil, err
								}
							} else {
								contextPrefix, err = cfg.GetContextPrefix(stackName, context, GetStackNamePattern(atmosConfig), stackName)
								if err != nil {
									return nil, err
								}
							}

							if adminStackContext.Component == componentName && adminStackContextPrefix == contextPrefix {
								if componentSettingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
									return affectedList, nil
								}

								if componentSettingsSpaceliftSection, ok = componentSettingsSection["spacelift"].(map[string]any); !ok {
									return affectedList, nil
								}

								if spaceliftWorkspaceEnabled, ok := componentSettingsSpaceliftSection["workspace_enabled"].(bool); !ok || !spaceliftWorkspaceEnabled {
									return nil, errUtils.Build(errUtils.ErrSpaceliftAdminStackWorkspaceNotEnabled).
										WithContext("component", currentComponentName).
										WithContext("stack", currentStackName).
										WithContext("admin_component", componentName).
										WithContext("admin_stack", stackName).
										WithHintf(
											"Component '%s' in stack '%s' references Spacelift admin component '%s' in stack '%s' via 'settings.spacelift.admin_stack_selector', "+
												"but the admin component has workspace disabled in 'settings.spacelift.workspace_enabled'",
											currentComponentName,
											currentStackName,
											componentName,
											stackName,
										).
										WithHint("Enable Spacelift workspace for the admin component by setting 'settings.spacelift.workspace_enabled: true'").
										Err()
								}

								affectedSpaceliftAdminStack := schema.Affected{
									ComponentType: cfg.TerraformComponentType,
									Component:     componentName,
									Stack:         stackName,
									Affected:      "stack.settings.spacelift.admin_stack_selector",
								}

								err = appendToAffected(
									atmosConfig,
									componentName,
									stackName,
									&componentSection,
									affectedList,
									&affectedSpaceliftAdminStack,
									false,
									nil,
									includeSettings,
								)
								if err != nil {
									return nil, err
								}
							}
						}
					}
				}
			}
		}
	}

	return affectedList, nil
}

// addDependentsToAffected adds dependent components and stacks to each affected component.
func addDependentsToAffected(
	atmosConfig *schema.AtmosConfiguration,
	affected *[]schema.Affected,
	includeSettings bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	onlyInStack string,
) error {
	for i := 0; i < len(*affected); i++ {
		a := &(*affected)[i]

		// Skip if `onlyInStack` is specified and the affected component is not in the specified stack.
		if onlyInStack != "" && a.Stack != onlyInStack {
			continue
		}

		deps, err := ExecuteDescribeDependents(
			atmosConfig,
			&DescribeDependentsArgs{
				Component:            a.Component,
				Stack:                a.Stack,
				IncludeSettings:      includeSettings,
				ProcessTemplates:     processTemplates,
				ProcessYamlFunctions: processYamlFunctions,
				Skip:                 skip,
				OnlyInStack:          onlyInStack,
			},
		)
		if err != nil {
			return err
		}

		if len(deps) > 0 {
			a.Dependents = deps
			err = addDependentsToDependents(
				atmosConfig,
				&deps,
				includeSettings,
				processTemplates,
				processYamlFunctions,
				skip,
				onlyInStack,
			)
			if err != nil {
				return err
			}
		} else {
			a.Dependents = []schema.Dependent{}
		}
	}

	processIncludedInDependencies(affected)
	return nil
}

// addDependentsToDependents recursively adds dependent components and stacks to each dependent component.
func addDependentsToDependents(
	atmosConfig *schema.AtmosConfiguration,
	dependents *[]schema.Dependent,
	includeSettings bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	onlyInStack string,
) error {
	for i := 0; i < len(*dependents); i++ {
		d := &(*dependents)[i]

		deps, err := ExecuteDescribeDependents(
			atmosConfig,
			&DescribeDependentsArgs{
				Component:            d.Component,
				Stack:                d.Stack,
				IncludeSettings:      includeSettings,
				ProcessTemplates:     processTemplates,
				ProcessYamlFunctions: processYamlFunctions,
				Skip:                 skip,
				OnlyInStack:          onlyInStack,
			},
		)
		if err != nil {
			return err
		}

		if len(deps) > 0 {
			d.Dependents = deps
			err = addDependentsToDependents(
				atmosConfig,
				&deps,
				includeSettings,
				processTemplates,
				processYamlFunctions,
				skip,
				onlyInStack,
			)
			if err != nil {
				return err
			}
		} else {
			d.Dependents = []schema.Dependent{}
		}
	}

	return nil
}

func processIncludedInDependencies(affected *[]schema.Affected) {
	for i := 0; i < len(*affected); i++ {
		a := &((*affected)[i])
		a.IncludedInDependents = processIncludedInDependenciesForAffected(affected, a.StackSlug, i)
		if !a.IncludedInDependents {
			processPeerDependencies(&a.Dependents)
		}
	}
}

func processIncludedInDependenciesForAffected(affected *[]schema.Affected, stackSlug string, affectedIndex int) bool {
	for i := 0; i < len(*affected); i++ {
		if i == affectedIndex {
			continue
		}

		a := &((*affected)[i])

		if len(a.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForDependents(&a.Dependents, stackSlug)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}

func processIncludedInDependenciesForDependents(dependents *[]schema.Dependent, stackSlug string) bool {
	for i := 0; i < len(*dependents); i++ {
		d := &((*dependents)[i])

		if d.StackSlug == stackSlug {
			return true
		}

		if len(d.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForDependents(&d.Dependents, stackSlug)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}

func processPeerDependencies(dependents *[]schema.Dependent) {
	for i := 0; i < len(*dependents); i++ {
		d := &((*dependents)[i])
		d.IncludedInDependents = processIncludedInDependenciesForPeerDependencies(dependents, d.StackSlug, i)
		processPeerDependencies(&d.Dependents)
	}
}

func processIncludedInDependenciesForPeerDependencies(dependents *[]schema.Dependent, stackSlug string, depIndex int) bool {
	for i := 0; i < len(*dependents); i++ {
		if i == depIndex {
			continue
		}

		d := &((*dependents)[i])

		if d.StackSlug == stackSlug {
			return true
		}

		if len(d.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForPeerDependencies(&d.Dependents, stackSlug, -1)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}

// isComponentInStackAffected checks if a component in a stack is in the affected list, recursively.
func isComponentInStackAffected(affectedList []schema.Affected, stackSlug string) bool {
	for i := range affectedList {
		if affectedList[i].StackSlug == stackSlug {
			return true
		}
	}
	return false
}
