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
	componentMetadata map[interface{}]interface{},
	context c.Context,
) (string, error) {

	contextPrefix, err := c.GetContextPrefix(stack, context, stackNamePattern)
	if err != nil {
		return "", err
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
