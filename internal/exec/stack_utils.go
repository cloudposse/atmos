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
	component string,
	baseComponent string,
	context c.Context,
) (string, error) {

	contextPrefix, err := c.GetContextPrefix(stack, context, stackNamePattern)
	if err != nil {
		return "", err
	}

	var workspace string

	// Terraform workspace can be overridden per component in YAML config `metadata.terraform_workspace`
	if componentTerraformWorkspace, componentTerraformWorkspaceExist := componentMetadata["terraform_workspace"].(string); componentTerraformWorkspaceExist {
		workspace = componentTerraformWorkspace
	} else if baseComponent == "" {
		workspace = contextPrefix
	} else {
		workspace = fmt.Sprintf("%s-%s", contextPrefix, component)
	}

	return strings.Replace(workspace, "/", "-", -1), nil
}
