package exec

import (
	"fmt"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// BuildTerraformWorkspace builds Terraform workspace
func BuildTerraformWorkspace(
	stack string,
	stackNamePattern string,
	componentMetadata map[any]any,
	context cfg.Context,
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
