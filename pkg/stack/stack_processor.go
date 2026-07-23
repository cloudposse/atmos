package stack

import (
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessYAMLConfigFiles takes a list of paths to stack manifests, processes and deep-merges all imports, and returns a list of stack configs.
func ProcessYAMLConfigFiles(
	atmosConfig *schema.AtmosConfiguration,
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	packerComponentsBasePath string,
	ansibleComponentsBasePath string,
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
	defer perf.Track(atmosConfig, "stack.ProcessYAMLConfigFiles")()

	return exec.ProcessYAMLConfigFiles(
		atmosConfig,
		stacksBasePath,
		terraformComponentsBasePath,
		helmfileComponentsBasePath,
		packerComponentsBasePath,
		ansibleComponentsBasePath,
		filePaths,
		processStackDeps,
		processComponentDeps,
		ignoreMissingFiles,
	)
}

// ProcessYAMLConfigFile takes a path to a YAML stack manifest, recursively
// processes and deep-merges all the imports, and returns the processing result.
func ProcessYAMLConfigFile(
	atmosConfig *schema.AtmosConfiguration,
	basePath string,
	filePath string,
	importsConfig map[string]map[string]any,
	context map[string]any,
	ignoreMissingFiles bool,
	skipTemplatesProcessingInImports bool,
	ignoreMissingTemplateValues bool,
	skipIfMissing bool,
	parentTerraformOverridesInline map[string]any,
	parentTerraformOverridesImports map[string]any,
	parentHelmfileOverridesInline map[string]any,
	parentHelmfileOverridesImports map[string]any,
	atmosManifestJsonSchemaFilePath string,
) (*schema.StackManifestProcessingResult, error) {
	defer perf.Track(atmosConfig, "stack.ProcessYAMLConfigFile")()

	return exec.ProcessYAMLConfigFile(
		atmosConfig,
		basePath,
		filePath,
		importsConfig,
		context,
		ignoreMissingFiles,
		skipTemplatesProcessingInImports,
		ignoreMissingTemplateValues,
		skipIfMissing,
		parentTerraformOverridesInline,
		parentTerraformOverridesImports,
		parentHelmfileOverridesInline,
		parentHelmfileOverridesImports,
		atmosManifestJsonSchemaFilePath,
	)
}
