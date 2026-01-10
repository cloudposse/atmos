package exec

import (
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
)

// ProcessStacksForGenerate wraps ProcessStacks for use by the generate adapter.
// Signature matches ExecStackProcessor in pkg/terraform/generate/adapter.go.
//
//nolint:revive,gocritic // argument-limit: signature must match internal/exec.ProcessStacks; hugeParam: interface requires value type
func ProcessStacksForGenerate(
	atmosConfig *schema.AtmosConfiguration,
	info schema.ConfigAndStacksInfo,
	checkStack bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	authManager auth.AuthManager,
) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "exec.ProcessStacksForGenerate")()

	return ProcessStacks(atmosConfig, info, checkStack, processTemplates, processYamlFunctions, skip, authManager)
}

// FindStacksMapForGenerate wraps FindStacksMap for use by the generate adapter.
// Signature matches ExecFindStacksMap in pkg/terraform/generate/adapter.go.
func FindStacksMapForGenerate(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.FindStacksMapForGenerate")()

	return FindStacksMap(atmosConfig, ignoreMissingFiles)
}

// generateFilesForComponent generates files from the generate section during terraform execution.
// This is called automatically when auto_generate_files is enabled.
func generateFilesForComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	adapter := tfgenerate.NewExecAdapter(ProcessStacksForGenerate, FindStacksMapForGenerate)
	service := tfgenerate.NewService(adapter)
	return service.GenerateFilesForComponent(atmosConfig, info, workingDir)
}

// GetGenerateFilenamesForComponent returns the list of filenames from the generate section.
// This is used by terraform clean to know which files to delete.
func GetGenerateFilenamesForComponent(componentSection map[string]any) []string {
	defer perf.Track(nil, "exec.GetGenerateFilenamesForComponent")()

	return tfgenerate.GetFilenamesForComponent(componentSection)
}
