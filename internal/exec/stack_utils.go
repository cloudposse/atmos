package exec

import (
	"fmt"
	"path/filepath"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// BuildTerraformWorkspace builds Terraform workspace.
func BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "exec.BuildTerraformWorkspace")()

	// Return 'default' workspace if workspaces are disabled
	// Terraform always operates in the `default` workspace when multiple workspaces are unsupported or disabled,
	// preventing switching or creating additional workspaces.
	if !isWorkspacesEnabled(atmosConfig, &configAndStacksInfo) {
		return cfg.TerraformDefaultWorkspace, nil
	}

	var contextPrefix string
	var err error
	var tmpl string

	// Stack name precedence: name (from manifest) > name_template > name_pattern > filename.
	switch {
	case configAndStacksInfo.StackManifestName != "":
		contextPrefix = configAndStacksInfo.StackManifestName
	case atmosConfig.Stacks.NameTemplate != "":
		tmpl, err = ProcessTmpl(atmosConfig, "terraform-workspace-stacks-name-template", atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
		if err != nil {
			return "", err
		}
		contextPrefix = tmpl
	case atmosConfig.Stacks.NamePattern != "":
		contextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack, configAndStacksInfo.Context, atmosConfig.Stacks.NamePattern, configAndStacksInfo.Stack)
		if err != nil {
			return "", err
		}
	default:
		contextPrefix = strings.Replace(configAndStacksInfo.Stack, "/", "-", -1)
	}

	var workspace string
	componentMetadata := configAndStacksInfo.ComponentMetadataSection

	// Terraform workspace can be overridden per component using `metadata.terraform_workspace_pattern` or `metadata.terraform_workspace_template` or `metadata.terraform_workspace`
	if terraformWorkspaceTemplate, terraformWorkspaceTemplateExist := componentMetadata["terraform_workspace_template"].(string); terraformWorkspaceTemplateExist {
		tmpl, err = ProcessTmpl(atmosConfig, "terraform-workspace-template", terraformWorkspaceTemplate, configAndStacksInfo.ComponentSection, false)
		if err != nil {
			return "", err
		}
		workspace = tmpl
	} else if terraformWorkspacePattern, terraformWorkspacePatternExist := componentMetadata["terraform_workspace_pattern"].(string); terraformWorkspacePatternExist {
		workspace = cfg.ReplaceContextTokens(configAndStacksInfo.Context, terraformWorkspacePattern)
	} else if terraformWorkspace, terraformWorkspaceExist := componentMetadata["terraform_workspace"].(string); terraformWorkspaceExist {
		workspace = terraformWorkspace
	} else if configAndStacksInfo.Context.BaseComponent == "" {
		workspace = contextPrefix
	} else {
		workspace = fmt.Sprintf("%s-%s", contextPrefix, configAndStacksInfo.Context.Component)
	}

	return strings.Replace(workspace, "/", "-", -1), nil
}

// ProcessComponentMetadata processes component metadata and returns a base component (if any) and whether
// the component is real or abstract and whether the component is disabled or not and whether the component is locked.
func ProcessComponentMetadata(
	component string,
	componentSection map[string]any,
) (map[string]any, string, bool, bool, bool) {
	defer perf.Track(nil, "exec.ProcessComponentMetadata")()

	baseComponentName := ""
	componentIsAbstract := false
	componentIsEnabled := true
	componentIsLocked := false
	var componentMetadata map[string]any

	// Find base component in the `component` attribute
	if base, ok := componentSection[cfg.ComponentSectionName].(string); ok {
		baseComponentName = base
	}

	if componentMetadataSection, componentMetadataSectionExists := componentSection["metadata"]; componentMetadataSectionExists {
		componentMetadata = componentMetadataSection.(map[string]any)
		if componentMetadataType, componentMetadataTypeAttributeExists := componentMetadata["type"].(string); componentMetadataTypeAttributeExists {
			if componentMetadataType == "abstract" {
				componentIsAbstract = true
			}
		}
		if enabledValue, exists := componentMetadata["enabled"]; exists {
			if enabled, ok := enabledValue.(bool); ok && !enabled {
				componentIsEnabled = false
			}
		}
		if lockedValue, exists := componentMetadata["locked"]; exists {
			if locked, ok := lockedValue.(bool); ok && locked {
				componentIsLocked = true
			}
		}
		// Find base component in the `metadata.component` attribute
		// `metadata.component` overrides `component`
		if componentMetadataComponent, componentMetadataComponentExists := componentMetadata[cfg.ComponentSectionName].(string); componentMetadataComponentExists {
			baseComponentName = componentMetadataComponent
		}
	}

	// If `component` or `metadata.component` is the same as the atmos component, the atmos component does not have a base component
	if component == baseComponentName {
		baseComponentName = ""
	}

	return componentMetadata, baseComponentName, componentIsAbstract, componentIsEnabled, componentIsLocked
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

	var dependentStackName string

	dep := strings.Replace(dependsOn, "/", "-", -1)

	if u.SliceContainsString(allStackNames, dep) {
		dependentStackName = dep
	} else if u.SliceContainsString(componentNamesInCurrentStack, dep) {
		dependentStackName = fmt.Sprintf("%s-%s", currentStackName, dep)
	} else {
		errorMessage := fmt.Errorf("the component '%[1]s' in the stack '%[2]s' specifies 'depends_on' dependency '%[3]s', "+
			"but '%[3]s' is not a stack and not a component in the '%[2]s' stack",
			currentComponentName,
			currentStackName,
			dependsOn,
		)

		return "", errorMessage
	}

	return dependentStackName, nil
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

	if u.SliceContainsString(allStackNames, dep) {
		return dep, nil
	}

	errorMessage := fmt.Errorf("the component '%[1]s' in the stack '%[2]s' specifies 'settings.depends_on' dependency "+
		"on the component '%[3]s' in the stack '%[4]s', but '%[3]s' is not defined in the '%[4]s' stack, or the component and stack names are not correct",
		currentComponentName,
		currentStackName,
		dependsOnComponentName,
		dependsOnStackName,
	)

	return "", errorMessage
}

// BuildComponentPath builds component path (path to the component's physical location on disk).
func BuildComponentPath(
	atmosConfig *schema.AtmosConfiguration,
	componentSectionMap *map[string]any,
	componentType string,
) string {
	defer perf.Track(atmosConfig, "exec.BuildComponentPath")()

	var componentPath string

	if stackComponentSection, ok := (*componentSectionMap)[cfg.ComponentSectionName].(string); ok {
		switch componentType {
		case cfg.TerraformComponentType:
			componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, stackComponentSection)
		case cfg.HelmfileComponentType:
			componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath, stackComponentSection)
		case cfg.PackerComponentType:
			componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath, stackComponentSection)
		}
	}

	return componentPath
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

// IsComponentEnabled returns 'true' if the component is enabled.
func IsComponentEnabled(varsSection map[string]any) bool {
	defer perf.Track(nil, "exec.IsComponentEnabled")()

	if enabled, ok := varsSection["enabled"].(bool); ok {
		if enabled == false {
			return false
		}
	}
	return true
}
