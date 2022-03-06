package exec

import (
	"fmt"
	c "github.com/cloudposse/atmos/pkg/config"
	"path"
)

// constructTerraformComponentWorkingDir constructs the working dir for a terraform component in a stack
func constructTerraformComponentWorkingDir(info c.ConfigAndStacksInfo) string {
	return path.Join(
		c.Config.BasePath,
		c.Config.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)
}

// constructTerraformComponentPlanfileName constructs the planfile name for a terraform component in a stack
func constructTerraformComponentPlanfileName(info c.ConfigAndStacksInfo) string {
	var planFile string
	if len(info.ComponentFolderPrefix) == 0 {
		planFile = fmt.Sprintf("%s-%s.planfile", info.ContextPrefix, info.Component)
	} else {
		planFile = fmt.Sprintf("%s-%s-%s.planfile", info.ContextPrefix, info.ComponentFolderPrefix, info.Component)
	}

	return planFile
}

// constructTerraformComponentVarfileName constructs the varfile name for a terraform component in a stack
func constructTerraformComponentVarfileName(info c.ConfigAndStacksInfo) string {
	var varFile string
	if len(info.ComponentFolderPrefix) == 0 {
		varFile = fmt.Sprintf("%s-%s.terraform.tfvars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefix, info.Component)
	}

	return varFile
}

// constructTerraformComponentVarfilePath constructs the varfile path for a terraform component in a stack
func constructTerraformComponentVarfilePath(info c.ConfigAndStacksInfo) string {
	return path.Join(
		constructTerraformComponentWorkingDir(info),
		constructTerraformComponentVarfileName(info),
	)
}

// constructTerraformComponentPlanfilePath constructs the planfile path for a terraform component in a stack
func constructTerraformComponentPlanfilePath(info c.ConfigAndStacksInfo) string {
	return path.Join(
		constructTerraformComponentWorkingDir(info),
		constructTerraformComponentPlanfileName(info),
	)
}

// constructHelmfileComponentWorkingDir constructs the working dir for a helmfile component in a stack
func constructHelmfileComponentWorkingDir(info c.ConfigAndStacksInfo) string {
	return path.Join(
		c.Config.BasePath,
		c.Config.Components.Helmfile.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)
}

// constructHelmfileComponentVarfileName constructs the varfile name for a helmfile component in a stack
func constructHelmfileComponentVarfileName(info c.ConfigAndStacksInfo) string {
	var varFile string
	if len(info.ComponentFolderPrefix) == 0 {
		varFile = fmt.Sprintf("%s-%s.helmfile.vars.yaml", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.helmfile.vars.yaml", info.ContextPrefix, info.ComponentFolderPrefix, info.Component)
	}
	return varFile
}

// constructHelmfileComponentVarfilePath constructs the varfile path for a helmfile component in a stack
func constructHelmfileComponentVarfilePath(info c.ConfigAndStacksInfo) string {
	return path.Join(
		constructHelmfileComponentWorkingDir(info),
		constructHelmfileComponentVarfileName(info),
	)
}
