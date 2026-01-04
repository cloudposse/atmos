package clean

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecProcessStacks is a function type for ProcessStacks from internal/exec.
type ExecProcessStacks func(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error)

// ExecDescribeStacks is a function type for ExecuteDescribeStacks from internal/exec.
type ExecDescribeStacks func(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
) (map[string]any, error)

// ExecGetGenerateFilenames is a function type for GetGenerateFilenamesForComponent.
type ExecGetGenerateFilenames func(componentSection map[string]any) []string

// ExecCollectComponentsDirectoryObjects is a function type for CollectComponentsDirectoryObjects.
type ExecCollectComponentsDirectoryObjects func(basePath string, componentPaths []string, patterns []string) ([]Directory, error)

// ExecConstructVarfileName is a function type for constructTerraformComponentVarfileName.
type ExecConstructVarfileName func(info *schema.ConfigAndStacksInfo) string

// ExecConstructPlanfileName is a function type for constructTerraformComponentPlanfileName.
type ExecConstructPlanfileName func(info *schema.ConfigAndStacksInfo) string

// ExecGetAllStacksComponentsPaths is a function type for getAllStacksComponentsPaths.
type ExecGetAllStacksComponentsPaths func(stacksMap map[string]any) []string

// ExecAdapter adapts internal/exec functions to the StackProcessor interface.
type ExecAdapter struct {
	processStacks                    ExecProcessStacks
	executeDescribeStacks            ExecDescribeStacks
	getGenerateFilenamesForComponent ExecGetGenerateFilenames
	collectComponentsDirectoryObjs   ExecCollectComponentsDirectoryObjects
	constructVarfileName             ExecConstructVarfileName
	constructPlanfileName            ExecConstructPlanfileName
	getAllStacksComponentsPaths      ExecGetAllStacksComponentsPaths
}

// NewExecAdapter creates a new adapter that wraps internal/exec functions.
//
//nolint:revive // argument-limit: adapter requires all internal/exec functions for clean operations
func NewExecAdapter(
	processStacks ExecProcessStacks,
	executeDescribeStacks ExecDescribeStacks,
	getGenerateFilenames ExecGetGenerateFilenames,
	collectComponentsDirectoryObjs ExecCollectComponentsDirectoryObjects,
	constructVarfileName ExecConstructVarfileName,
	constructPlanfileName ExecConstructPlanfileName,
	getAllStacksComponentsPaths ExecGetAllStacksComponentsPaths,
) *ExecAdapter {
	defer perf.Track(nil, "clean.NewExecAdapter")()

	return &ExecAdapter{
		processStacks:                    processStacks,
		executeDescribeStacks:            executeDescribeStacks,
		getGenerateFilenamesForComponent: getGenerateFilenames,
		collectComponentsDirectoryObjs:   collectComponentsDirectoryObjs,
		constructVarfileName:             constructVarfileName,
		constructPlanfileName:            constructPlanfileName,
		getAllStacksComponentsPaths:      getAllStacksComponentsPaths,
	}
}

// ProcessStacks implements StackProcessor.ProcessStacks.
//
//nolint:gocritic // hugeParam: interface contract requires value type for ConfigAndStacksInfo
func (a *ExecAdapter) ProcessStacks(atmosConfig *schema.AtmosConfiguration, info schema.ConfigAndStacksInfo) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "clean.ExecAdapter.ProcessStacks")()

	return a.processStacks(atmosConfig, info)
}

// ExecuteDescribeStacks implements StackProcessor.ExecuteDescribeStacks.
func (a *ExecAdapter) ExecuteDescribeStacks(
	atmosConfig *schema.AtmosConfiguration,
	filterByStack string,
	components []string,
) (map[string]any, error) {
	defer perf.Track(atmosConfig, "clean.ExecAdapter.ExecuteDescribeStacks")()

	return a.executeDescribeStacks(atmosConfig, filterByStack, components)
}

// GetGenerateFilenamesForComponent implements StackProcessor.GetGenerateFilenamesForComponent.
func (a *ExecAdapter) GetGenerateFilenamesForComponent(componentSection map[string]any) []string {
	defer perf.Track(nil, "clean.ExecAdapter.GetGenerateFilenamesForComponent")()

	return a.getGenerateFilenamesForComponent(componentSection)
}

// CollectComponentsDirectoryObjects implements StackProcessor.CollectComponentsDirectoryObjects.
func (a *ExecAdapter) CollectComponentsDirectoryObjects(basePath string, componentPaths []string, patterns []string) ([]Directory, error) {
	defer perf.Track(nil, "clean.ExecAdapter.CollectComponentsDirectoryObjects")()

	return a.collectComponentsDirectoryObjs(basePath, componentPaths, patterns)
}

// ConstructTerraformComponentVarfileName implements StackProcessor.ConstructTerraformComponentVarfileName.
func (a *ExecAdapter) ConstructTerraformComponentVarfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "clean.ExecAdapter.ConstructTerraformComponentVarfileName")()

	return a.constructVarfileName(info)
}

// ConstructTerraformComponentPlanfileName implements StackProcessor.ConstructTerraformComponentPlanfileName.
func (a *ExecAdapter) ConstructTerraformComponentPlanfileName(info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "clean.ExecAdapter.ConstructTerraformComponentPlanfileName")()

	return a.constructPlanfileName(info)
}

// GetAllStacksComponentsPaths implements StackProcessor.GetAllStacksComponentsPaths.
func (a *ExecAdapter) GetAllStacksComponentsPaths(stacksMap map[string]any) []string {
	defer perf.Track(nil, "clean.ExecAdapter.GetAllStacksComponentsPaths")()

	return a.getAllStacksComponentsPaths(stacksMap)
}
