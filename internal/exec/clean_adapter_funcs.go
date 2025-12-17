package exec

import (
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
		nil, nil, false, false, false, false, nil, nil)
}

// CollectComponentsDirectoryObjectsForClean wraps CollectComponentsDirectoryObjects.
func CollectComponentsDirectoryObjectsForClean(basePath string, componentPaths []string, patterns []string) ([]tfclean.Directory, error) {
	defer perf.Track(nil, "exec.CollectComponentsDirectoryObjectsForClean")()

	// Convert between the types.
	dirs, err := CollectComponentsDirectoryObjects(basePath, componentPaths, patterns)
	if err != nil {
		return nil, err
	}
	// Convert Directory to tfclean.Directory.
	result := make([]tfclean.Directory, len(dirs))
	for i, d := range dirs {
		files := make([]tfclean.ObjectInfo, len(d.Files))
		for j, f := range d.Files {
			files[j] = tfclean.ObjectInfo{
				FullPath:     f.FullPath,
				RelativePath: f.RelativePath,
				Name:         f.Name,
				IsDir:        f.IsDir,
			}
		}
		result[i] = tfclean.Directory{
			Name:         d.Name,
			FullPath:     d.FullPath,
			RelativePath: d.RelativePath,
			Files:        files,
		}
	}
	return result, nil
}

// ConstructTerraformComponentVarfileNameForClean exports the varfile name constructor for clean.
func ConstructTerraformComponentVarfileNameForClean(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "exec.ConstructTerraformComponentVarfileNameForClean")()

	return constructTerraformComponentVarfileName(info)
}

// ConstructTerraformComponentPlanfileNameForClean exports the planfile name constructor for clean.
func ConstructTerraformComponentPlanfileNameForClean(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "exec.ConstructTerraformComponentPlanfileNameForClean")()

	return constructTerraformComponentPlanfileName(info)
}

// GetAllStacksComponentsPathsForClean exports the stacks component paths getter for clean.
func GetAllStacksComponentsPathsForClean(stacksMap map[string]any) []string {
	defer perf.Track(nil, "exec.GetAllStacksComponentsPathsForClean")()

	return getAllStacksComponentsPaths(stacksMap)
}
