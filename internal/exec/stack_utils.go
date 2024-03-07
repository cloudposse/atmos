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
func BuildTerraformWorkspace(
	stack string,
	stackNamePattern string,
	componentMetadata map[any]any,
	context schema.Context,
) (string, error) {

	var contextPrefix string
	var err error

	if stackNamePattern != "" {
		contextPrefix, err = cfg.GetContextPrefix(stack, context, stackNamePattern, stack)
		if err != nil {
			return "", err
		}
	} else {
		contextPrefix = strings.Replace(stack, "/", "-", -1)
	}

	var workspace string

	if terraformWorkspacePattern, terraformWorkspacePatternExist := componentMetadata["terraform_workspace_pattern"].(string); terraformWorkspacePatternExist {
		// Terraform workspace can be overridden per component in YAML config `metadata.terraform_workspace_pattern`
		workspace = cfg.ReplaceContextTokens(context, terraformWorkspacePattern)
	} else if terraformWorkspace, terraformWorkspaceExist := componentMetadata["terraform_workspace"].(string); terraformWorkspaceExist {
		// Terraform workspace can be overridden per component in YAML config `metadata.terraform_workspace`
		workspace = terraformWorkspace
	} else if context.BaseComponent == "" {
		workspace = contextPrefix
	} else {
		workspace = fmt.Sprintf("%s-%s", contextPrefix, context.Component)
	}

	return strings.Replace(workspace, "/", "-", -1), nil
}

// ProcessComponentMetadata processes component metadata and returns a base component (if any) and whether the component is real or abstract
func ProcessComponentMetadata(
	component string,
	componentSection map[string]any,
) (map[any]any, string, bool) {
	baseComponentName := ""
	componentIsAbstract := false
	var componentMetadata map[any]any

	// Find base component in the `component` attribute
	if base, ok := componentSection["component"].(string); ok {
		baseComponentName = base
	}

	if componentMetadataSection, componentMetadataSectionExists := componentSection["metadata"]; componentMetadataSectionExists {
		componentMetadata = componentMetadataSection.(map[any]any)
		if componentMetadataType, componentMetadataTypeAttributeExists := componentMetadata["type"].(string); componentMetadataTypeAttributeExists {
			if componentMetadataType == "abstract" {
				componentIsAbstract = true
			}
		}
		// Find base component in the `metadata.component` attribute
		// `metadata.component` overrides `component`
		if componentMetadataComponent, componentMetadataComponentExists := componentMetadata["component"].(string); componentMetadataComponentExists {
			baseComponentName = componentMetadataComponent
		}
	}

	// If `component` or `metadata.component` is the same as the atmos component, the atmos component does not have a base component
	if component == baseComponentName {
		baseComponentName = ""
	}

	return componentMetadata, baseComponentName, componentIsAbstract
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

	if stackComponentSection, ok := componentSectionMap["component"].(string); ok {
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
