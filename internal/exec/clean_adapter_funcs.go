package exec

import (
	"fmt"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfclean "github.com/cloudposse/atmos/pkg/terraform/clean"
)

// ProcessStacksForClean wraps ProcessStacks with the simplified signature for clean.
//
//nolint:gocritic // hugeParam: signature must match StackProcessor interface which uses value type
func ProcessStacksForClean(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStacksForClean")()

	// shouldCheckStack = only require stack if explicitly provided (matching main's behavior).
	shouldCheckStack := info.Stack != ""
	return ProcessStacks(atmosConfig, info, shouldCheckStack, false, false, nil, nil)
}

// ExecuteDescribeStacksForClean wraps ExecuteDescribeStacks with the simplified signature for clean.
func ExecuteDescribeStacksForClean(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeStacksForClean")()

	return ExecuteDescribeStacks(
		atmosConfig,
		filterByStack,
		components,
		nil, nil, false, false, false, false, nil, nil,
	)
}

// CollectComponentsDirectoryObjectsForClean delegates to pkg/terraform/clean.CollectComponentsDirectoryObjects.
func CollectComponentsDirectoryObjectsForClean(basePath string, componentPaths []string, patterns []string) ([]tfclean.Directory, error) {
	defer perf.Track(nil, "exec.CollectComponentsDirectoryObjectsForClean")()

	return tfclean.CollectComponentsDirectoryObjects(basePath, componentPaths, patterns)
}

// ConstructTerraformComponentVarfileNameForClean exports the varfile name constructor for clean.
func ConstructTerraformComponentVarfileNameForClean(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "exec.ConstructTerraformComponentVarfileNameForClean")()

	return constructTerraformComponentVarfileName(info)
}

// ConstructTerraformComponentVarfileName exports the varfile name constructor for use by other packages.
func ConstructTerraformComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "exec.ConstructTerraformComponentVarfileName")()

	return constructTerraformComponentVarfileName(info)
}

// ConstructTerraformComponentVarfilePath exports the varfile path constructor
// (bare name joined with the component's working directory, then resolved to
// an absolute path) for use by other packages that need the varfile to resolve
// correctly regardless of a subprocess's own working directory - e.g. tfmigrate,
// which for `migration "multi_state"` runs its internal convergence-check
// `terraform plan` from a *second* directory (`from_dir`), where a bare
// filename good only in the component's own directory would not exist.
func ConstructTerraformComponentVarfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "exec.ConstructTerraformComponentVarfilePath")()

	path := constructTerraformComponentVarfilePath(atmosConfig, info)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrPathResolution, err)
	}
	return absPath, nil
}

// ComputeTerraformSecretVarEnv partitions the component's variables exactly like
// the terraform execution path (secret-bearing values plus declared-sensitive
// inputs) and returns the TF_VAR_ environment entries for the excluded keys.
// For use by other packages that spawn terraform-adjacent subprocesses (e.g.
// tfmigrate), whose internal terraform runs would otherwise miss every variable
// the generated varfile keeps off disk.
func ComputeTerraformSecretVarEnv(info *schema.ConfigAndStacksInfo) ([]string, error) {
	defer perf.Track(nil, "exec.ComputeTerraformSecretVarEnv")()

	computeTerraformSecretVarKeys(info)
	return secretVarEnv(info)
}

// ConstructTerraformComponentPlanfileNameForClean exports the planfile name constructor for clean.
func ConstructTerraformComponentPlanfileNameForClean(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "exec.ConstructTerraformComponentPlanfileNameForClean")()

	return constructTerraformComponentPlanfileName(info)
}

// ConstructTerraformComponentPlanfilePath exports the planfile path constructor for use by other packages.
func ConstructTerraformComponentPlanfilePath(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "exec.ConstructTerraformComponentPlanfilePath")()

	return constructTerraformComponentPlanfilePath(atmosConfig, info)
}

// GetAllStacksComponentsPathsForClean delegates to pkg/terraform/clean.GetAllStacksComponentsPaths.
func GetAllStacksComponentsPathsForClean(stacksMap map[string]any) []string {
	defer perf.Track(nil, "exec.GetAllStacksComponentsPathsForClean")()

	return tfclean.GetAllStacksComponentsPaths(stacksMap)
}
