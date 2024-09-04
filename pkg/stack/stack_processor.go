package stack

import (
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessYAMLConfigFiles takes a list of paths to stack manifests, processes and deep-merges all imports,
// and returns a list of stack configs
func ProcessYAMLConfigFiles(
	cliConfig schema.CliConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	filePaths []string,
	processStackDeps bool,
	processComponentDeps bool,
	ignoreMissingFiles bool,
) (
	[]string,
	map[string]any,
	map[string]map[string]any,
	error,
) {
	return exec.ProcessYAMLConfigFiles(
		cliConfig,
		stacksBasePath,
		terraformComponentsBasePath,
		helmfileComponentsBasePath,
		filePaths,
		processStackDeps,
		processComponentDeps,
		ignoreMissingFiles,
	)
}

func ProcessYAMLConfigFile(
	cliConfig schema.CliConfiguration,
	basePath string,
	filePath string,
	importsConfig map[string]map[string]any,
	context map[string]any,
	ignoreMissingFiles bool,
	skipTemplatesProcessingInImports bool,
	ignoreMissingTemplateValues bool,
	skipIfMissing bool,
	parentTerraformOverrides map[string]any,
	parentHelmfileOverrides map[string]any,
	atmosManifestJsonSchemaFilePath string,
) (
	map[string]any,
	map[string]map[string]any,
	map[string]any,
	error,
) {
	return exec.ProcessYAMLConfigFile(
		cliConfig,
		basePath,
		filePath,
		importsConfig,
		context,
		ignoreMissingFiles,
		skipTemplatesProcessingInImports,
		ignoreMissingTemplateValues,
		skipIfMissing,
		parentTerraformOverrides,
		parentHelmfileOverrides,
		atmosManifestJsonSchemaFilePath,
	)
}

// ProcessStackConfig takes a stack manifest, deep-merges all variables, settings, environments and backends,
// and returns the final stack configuration for all Terraform and helmfile components
func ProcessStackConfig(
	cliConfig schema.CliConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	stack string,
	config map[string]any,
	processStackDeps bool,
	processComponentDeps bool,
	componentTypeFilter string,
	componentStackMap map[string]map[string][]string,
	importsConfig map[string]map[string]any,
	checkBaseComponentExists bool,
) (map[string]any, error) {
	return exec.ProcessStackConfig(
		cliConfig,
		stacksBasePath,
		terraformComponentsBasePath,
		helmfileComponentsBasePath,
		stack,
		config,
		processStackDeps,
		processComponentDeps,
		componentTypeFilter,
		componentStackMap,
		importsConfig,
		checkBaseComponentExists,
	)
}
