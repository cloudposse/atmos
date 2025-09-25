package exec

import (
	"fmt"
	"path/filepath"

	log "github.com/charmbracelet/log"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// constructTerraformComponentWorkingDir constructs the working dir for a terraform component in a stack.
func constructTerraformComponentWorkingDir(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	// Use the central function that respects all overrides and custom paths.
	path, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		// Log error but still try to return something.
		log.Debug("Failed to resolve component path, falling back to construction", "error", err)
		// Fallback that still respects the configured path.
		return filepath.Join(
			atmosConfig.BasePath,
			atmosConfig.Components.Terraform.BasePath,
			info.ComponentFolderPrefix,
			info.FinalComponent,
		)
	}
	return path
}

// constructTerraformComponentPlanfileName constructs the planfile name for a terraform component in a stack.
func constructTerraformComponentPlanfileName(info *schema.ConfigAndStacksInfo) string {
	var planFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		planFile = fmt.Sprintf("%s-%s.planfile", info.ContextPrefix, info.Component)
	} else {
		planFile = fmt.Sprintf("%s-%s-%s.planfile", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return planFile
}

// constructTerraformComponentVarfileName constructs the varfile name for a terraform component in a stack.
func constructTerraformComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.terraform.tfvars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return varFile
}

// constructTerraformComponentVarfilePath constructs the varfile path for a terraform component in a stack.
func constructTerraformComponentVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		constructTerraformComponentWorkingDir(atmosConfig, info),
		constructTerraformComponentVarfileName(info),
	)
}

// constructTerraformComponentPlanfilePath constructs the planfile path for a terraform component in a stack.
func constructTerraformComponentPlanfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		constructTerraformComponentWorkingDir(atmosConfig, info),
		constructTerraformComponentPlanfileName(info),
	)
}

// constructHelmfileComponentWorkingDir constructs the working dir for a helmfile component in a stack.
func constructHelmfileComponentWorkingDir(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	// Use the central function that respects all overrides and custom paths.
	path, err := u.GetComponentPath(atmosConfig, "helmfile", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		// Log error but still try to return something.
		log.Debug("Failed to resolve component path, falling back to construction", "error", err)
		// Fallback that still respects the configured path.
		return filepath.Join(
			atmosConfig.BasePath,
			atmosConfig.Components.Helmfile.BasePath,
			info.ComponentFolderPrefix,
			info.FinalComponent,
		)
	}
	return path
}

// constructHelmfileComponentVarfileName constructs the varfile name for a helmfile component in a stack.
func constructHelmfileComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.helmfile.vars.yaml", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.helmfile.vars.yaml", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}
	return varFile
}

// constructHelmfileComponentVarfilePath constructs the varfile path for a helmfile component in a stack.
func constructHelmfileComponentVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		constructHelmfileComponentWorkingDir(atmosConfig, info),
		constructHelmfileComponentVarfileName(info),
	)
}

// constructPackerComponentVarfileName constructs the varfile name for a Packer component in a stack.
func constructPackerComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.packer.vars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.packer.vars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}
	return varFile
}

// constructPackerComponentVarfilePath constructs the varfile path for a Packer component in a stack.
func constructPackerComponentVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	return filepath.Join(
		constructPackerComponentWorkingDir(atmosConfig, info),
		constructPackerComponentVarfileName(info),
	)
}

// constructPackerComponentWorkingDir constructs the working dir for a Packer component in a stack.
func constructPackerComponentWorkingDir(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	// Use the central function that respects all overrides and custom paths.
	path, err := u.GetComponentPath(atmosConfig, "packer", info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		// Log error but still try to return something.
		log.Debug("Failed to resolve component path, falling back to construction", "error", err)
		// Fallback that still respects the configured path.
		return filepath.Join(
			atmosConfig.BasePath,
			atmosConfig.Components.Packer.BasePath,
			info.ComponentFolderPrefix,
			info.FinalComponent,
		)
	}
	return path
}
