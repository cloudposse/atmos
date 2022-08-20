package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"strings"
)

// BuildTerraformWorkspace builds Terraform workspace
func BuildTerraformWorkspace(
	stack string,
	stackNamePattern string,
	componentMetadata map[any]any,
	context c.Context,
) (string, error) {

	var contextPrefix string
	var err error

	if stackNamePattern != "" {
		contextPrefix, err = c.GetContextPrefix(stack, context, stackNamePattern, stack)
		if err != nil {
			return "", err
		}
	} else {
		contextPrefix = strings.Replace(stack, "/", "-", -1)
	}

	var workspace string

	if terraformWorkspacePattern, terraformWorkspacePatternExist := componentMetadata["terraform_workspace_pattern"].(string); terraformWorkspacePatternExist {
		// Terraform workspace can be overridden per component in YAML config `metadata.terraform_workspace_pattern`
		workspace = c.ReplaceContextTokens(context, terraformWorkspacePattern)
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
