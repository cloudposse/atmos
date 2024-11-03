package exec

import (
	"fmt"
	"path"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// BuildTerraformWorkspace builds Terraform workspace
func BuildTerraformWorkspace(cliConfig schema.CliConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
	var contextPrefix string
	var err error
	var tmpl string

	if cliConfig.Stacks.NameTemplate != "" {
		tmpl, err = ProcessTmpl("terraform-workspace-stacks-name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
		if err != nil {
			return "", err
		}
		contextPrefix = tmpl
	} else if cliConfig.Stacks.NamePattern != "" {
		contextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack, configAndStacksInfo.Context, cliConfig.Stacks.NamePattern, configAndStacksInfo.Stack)
		if err != nil {
			return "", err
		}
	} else {
		contextPrefix = strings.Replace(configAndStacksInfo.Stack, "/", "-", -1)
	}

	var workspace string
	componentMetadata := configAndStacksInfo.ComponentMetadataSection

	// Terraform workspace can be overridden per component using `metadata.terraform_workspace_pattern` or `metadata.terraform_workspace_template` or `metadata.terraform_workspace`
	if terraformWorkspaceTemplate, terraformWorkspaceTemplateExist := componentMetadata["terraform_workspace_template"].(string); terraformWorkspaceTemplateExist {
		tmpl, err = ProcessTmpl("terraform-workspace-template", terraformWorkspaceTemplate, configAndStacksInfo.ComponentSection, false)
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

// ProcessComponentMetadata processes component metadata and returns a base component (if any) and whether the component is real or abstract and whether the component is enabled or not
func ProcessComponentMetadata(
	component string,
	componentSection map[string]any,
) (map[string]any, string, bool, bool) {
	baseComponentName := ""
	componentIsAbstract := false
	componentIsEnabled := true
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
		if componentMetadataEnabled, componentMetadataEnabledExists := componentMetadata["enabled"].(bool); componentMetadataEnabledExists && !componentMetadataEnabled {
			componentIsEnabled = false
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

	return componentMetadata, baseComponentName, componentIsAbstract, componentIsEnabled
}

// BuildDependentStackNameFromDependsOnLegacy builds the dependent stack name from "settings.spacelift.depends_on" config
func BuildDependentStackNameFromDependsOnLegacy(
	dependsOn string,
	allStackNames []string,
	currentStackName string,
	componentNamesInCurrentStack []string,
	currentComponentName string,
) (string, error) {
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

// BuildDependentStackNameFromDependsOn builds the dependent stack name from "settings.depends_on" config
func BuildDependentStackNameFromDependsOn(
	currentComponentName string,
	currentStackName string,
	dependsOnComponentName string,
	dependsOnStackName string,
	allStackNames []string,
) (string, error) {

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

// BuildComponentPath builds component path (path to the component's physical location on disk)
func BuildComponentPath(
	cliConfig schema.CliConfiguration,
	componentSectionMap map[string]any,
	componentType string,
) string {

	var componentPath string

	if stackComponentSection, ok := componentSectionMap[cfg.ComponentSectionName].(string); ok {
		if componentType == "terraform" {
			componentPath = path.Join(cliConfig.BasePath, cliConfig.Components.Terraform.BasePath, stackComponentSection)
		} else if componentType == "helmfile" {
			componentPath = path.Join(cliConfig.BasePath, cliConfig.Components.Helmfile.BasePath, stackComponentSection)
		}
	}

	return componentPath
}

// GetStackNamePattern returns stack name pattern
func GetStackNamePattern(cliConfig schema.CliConfiguration) string {
	return cliConfig.Stacks.NamePattern
}

// IsComponentAbstract returns 'true' if the component is abstract
func IsComponentAbstract(metadataSection map[string]any) bool {
	if metadataType, ok := metadataSection["type"].(string); ok {
		if metadataType == "abstract" {
			return true
		}
	}
	return false
}

// IsComponentEnabled returns 'true' if the component is enabled
func IsComponentEnabled(varsSection map[string]any) bool {
	if enabled, ok := varsSection["enabled"].(bool); ok {
		if enabled == false {
			return false
		}
	}
	return true
}
