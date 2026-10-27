package terraform

import (
	"fmt"
	"path/filepath"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ConstructTerraformComponentWorkingDir constructs the working dir for a terraform component in a stack.
func ConstructTerraformComponentWorkingDir(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentWorkingDir")()

	// Check if a provisioner (source or workdir) set a workdir path.
	// This takes precedence over the standard component path.
	// Use filepath.FromSlash to normalize path separators for cross-platform compatibility.
	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		return filepath.FromSlash(workdirPath)
	}

	// If we have a resolved absolute path, use GetComponentPath.
	// Otherwise, just construct the path directly (for tests and simple cases).
	if atmosConfig.TerraformDirAbsolutePath != "" {
		path, err := u.GetComponentPath(atmosConfig, "terraform", info.ComponentFolderPrefix, info.FinalComponent)
		if err == nil {
			return path
		}
		// Log error but still try to return something.
		log.Debug("Failed to resolve component path, falling back to construction", "error", err)
	}

	// Direct path construction without converting to absolute.
	// This maintains compatibility with tests that use simple paths.
	return filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		info.ComponentFolderPrefix,
		info.FinalComponent,
	)
}

// ConstructTerraformComponentPlanfileName constructs the planfile name for a terraform component in a stack.
func ConstructTerraformComponentPlanfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "terraform.ConstructTerraformComponentPlanfileName")()

	var planFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		planFile = fmt.Sprintf("%s-%s.planfile", info.ContextPrefix, info.Component)
	} else {
		planFile = fmt.Sprintf("%s-%s-%s.planfile", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return planFile
}

// ConstructTerraformComponentVarfileName constructs the varfile name for a terraform component in a stack.
func ConstructTerraformComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "terraform.ConstructTerraformComponentVarfileName")()

	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.terraform.tfvars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.terraform.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return varFile
}

// ConstructTerraformComponentTestVarfileName constructs the test varfile name for a terraform component in a stack.
func ConstructTerraformComponentTestVarfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "terraform.ConstructTerraformComponentTestVarfileName")()

	var varFile string
	if len(info.ComponentFolderPrefixReplaced) == 0 {
		varFile = fmt.Sprintf("%s-%s.terraform.test.tfvars.json", info.ContextPrefix, info.Component)
	} else {
		varFile = fmt.Sprintf("%s-%s-%s.terraform.test.tfvars.json", info.ContextPrefix, info.ComponentFolderPrefixReplaced, info.Component)
	}

	return varFile
}

// ConstructTerraformComponentVarfilePath constructs the varfile path for a terraform component in a stack.
func ConstructTerraformComponentVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentVarfilePath")()

	return filepath.Join(
		ConstructTerraformComponentWorkingDir(atmosConfig, info),
		ConstructTerraformComponentVarfileName(info),
	)
}

// ConstructTerraformComponentTestVarfilePath constructs the test varfile path for a terraform component in a stack.
func ConstructTerraformComponentTestVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentTestVarfilePath")()

	return filepath.Join(
		ConstructTerraformComponentWorkingDir(atmosConfig, info),
		ConstructTerraformComponentTestVarfileName(info),
	)
}

// ConstructTerraformComponentPlanfilePath constructs the planfile path for a terraform component in a stack.
func ConstructTerraformComponentPlanfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "terraform.ConstructTerraformComponentPlanfilePath")()

	return filepath.Join(
		ConstructTerraformComponentWorkingDir(atmosConfig, info),
		ConstructTerraformComponentPlanfileName(info),
	)
}
