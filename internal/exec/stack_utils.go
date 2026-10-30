package exec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// BuildTerraformWorkspace builds Terraform workspace.
func BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
	return NamingResolver().BuildTerraformWorkspace(atmosConfig, &configAndStacksInfo)
}

// ProcessComponentMetadata processes component metadata and returns a base component (if any) and whether
// the component is real or abstract and whether the component is disabled or not and whether the component is locked.
func ProcessComponentMetadata(
	componentName string,
	componentSection map[string]any,
) (map[string]any, string, bool, bool, bool) {
	result := component.ProcessMetadata(componentName, componentSection)
	return result.Fields, result.BaseComponent, result.Abstract, result.Enabled, result.Locked
}

// BuildDependentStackNameFromDependsOnLegacy builds the dependent stack name from "settings.spacelift.depends_on" config.
func BuildDependentStackNameFromDependsOnLegacy(
	dependsOn string,
	allStackNames []string,
	currentStackName string,
	componentNamesInCurrentStack []string,
	currentComponentName string,
) (string, error) {
	defer perf.Track(nil, "exec.BuildDependentStackNameFromDependsOnLegacy")()

	dep := strings.Replace(dependsOn, "/", "-", -1)

	if slices.Contains(allStackNames, dep) {
		return dep, nil
	}

	if slices.Contains(componentNamesInCurrentStack, dep) {
		return fmt.Sprintf("%s-%s", currentStackName, dep), nil
	}

	return "", fmt.Errorf(
		"%w: the component '%[2]s' in the stack '%[3]s' specifies 'depends_on' dependency '%[4]s', "+
			"but '%[4]s' is not a stack and not a component in the '%[3]s' stack",
		errUtils.ErrInvalidDependsOn,
		currentComponentName,
		currentStackName,
		dependsOn,
	)
}

// BuildDependentStackNameFromDependsOn builds the dependent stack name from "settings.depends_on" config.
func BuildDependentStackNameFromDependsOn(
	currentComponentName string,
	currentStackName string,
	dependsOnComponentName string,
	dependsOnStackName string,
	allStackNames []string,
) (string, error) {
	defer perf.Track(nil, "exec.BuildDependentStackNameFromDependsOn")()

	dep := strings.Replace(fmt.Sprintf("%s-%s", dependsOnStackName, dependsOnComponentName), "/", "-", -1)

	if slices.Contains(allStackNames, dep) {
		return dep, nil
	}

	return "", fmt.Errorf(
		"%w: the component '%s' in the stack '%s' specifies 'settings.depends_on' dependency "+
			"on the component '%s' in the stack '%s', but '%s' is not defined in the '%s' stack, or the component and stack names are not correct",
		errUtils.ErrInvalidSettingsDependsOn,
		currentComponentName,
		currentStackName,
		dependsOnComponentName,
		dependsOnStackName,
		dependsOnComponentName,
		dependsOnStackName,
	)
}

// BuildComponentPath builds component path (path to the component's physical location on disk).
// The optional componentNameFallback parameter is used when the "component" field is not set
// in the componentSectionMap (e.g., for JIT vendored components with "source" attribute).
func BuildComponentPath(
	atmosConfig *schema.AtmosConfiguration,
	componentSectionMap *map[string]any,
	componentType string,
	componentNameFallback ...string,
) string {
	defer perf.Track(atmosConfig, "exec.BuildComponentPath")()

	// Determine the component folder name.
	componentFolder := GetComponentFolder(componentSectionMap, componentNameFallback...)
	if componentFolder == "" {
		return ""
	}

	// Build the full path based on component type.
	switch componentType {
	case cfg.TerraformComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, componentFolder)
	case cfg.HelmfileComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath, componentFolder)
	case cfg.PackerComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath, componentFolder)
	case cfg.AnsibleComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Ansible.BasePath, componentFolder)
	case cfg.ContainerComponentType:
		return filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Container.BasePath, componentFolder)
	default:
		// Emulator components (and any type with no filesystem source tree) have no component
		// path. See getComponentBasePath in describe_stacks.go.
		return ""
	}
}

// GetComponentFolder extracts the component folder name from a component section.
// It checks for an explicit "component" field, falling back to componentNameFallback if provided.
// This ensures components with "source" (JIT vendored) or without explicit "component" field
// still have their folder resolved correctly.
func GetComponentFolder(componentSectionMap *map[string]any, componentNameFallback ...string) string {
	defer perf.Track(nil, "exec.GetComponentFolder")()

	// Check for explicit "component" field.
	if stackComponentSection, ok := (*componentSectionMap)[cfg.ComponentSectionName].(string); ok && stackComponentSection != "" {
		return stackComponentSection
	}

	// Use fallback if provided.
	if len(componentNameFallback) > 0 && componentNameFallback[0] != "" {
		return componentNameFallback[0]
	}

	return ""
}

// GetStackNamePattern returns the stack name pattern.
func GetStackNamePattern(atmosConfig *schema.AtmosConfiguration) string {
	return atmosConfig.Stacks.NamePattern
}

// GetStackNameTemplate returns the stack name template.
func GetStackNameTemplate(atmosConfig *schema.AtmosConfiguration) string {
	defer perf.Track(atmosConfig, "exec.GetStackNameTemplate")()

	return atmosConfig.Stacks.NameTemplate
}

// IsComponentAbstract returns 'true' if the component is abstract.
func IsComponentAbstract(metadataSection map[string]any) bool {
	defer perf.Track(nil, "exec.IsComponentAbstract")()

	if metadataType, ok := metadataSection["type"].(string); ok {
		if metadataType == "abstract" {
			return true
		}
	}
	return false
}
