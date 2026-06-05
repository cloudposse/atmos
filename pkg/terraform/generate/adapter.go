package generate

import (
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecStackProcessor is a function type that matches the ProcessStacks signature in internal/exec.
type ExecStackProcessor func(
	atmosConfig *schema.AtmosConfiguration,
	info schema.ConfigAndStacksInfo,
	checkStack bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	authManager auth.AuthManager,
) (schema.ConfigAndStacksInfo, error)

// ExecFindStacksMap is a function type that matches the FindStacksMap signature in internal/exec.
type ExecFindStacksMap func(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error)

// ExecAdapter adapts internal/exec functions to the StackProcessor interface.
type ExecAdapter struct {
	processStacks ExecStackProcessor
	findStacksMap ExecFindStacksMap
}

// NewExecAdapter creates a new adapter that wraps internal/exec functions.
func NewExecAdapter(processStacks ExecStackProcessor, findStacksMap ExecFindStacksMap) *ExecAdapter {
	defer perf.Track(nil, "generate.NewExecAdapter")()

	return &ExecAdapter{
		processStacks: processStacks,
		findStacksMap: findStacksMap,
	}
}

// ProcessStacks implements StackProcessor.ProcessStacks by delegating to internal/exec.ProcessStacks.
//
//nolint:revive,gocritic // argument-limit: signature must match internal/exec.ProcessStacks; hugeParam: interface requires value type
func (a *ExecAdapter) ProcessStacks(
	atmosConfig *schema.AtmosConfiguration,
	info schema.ConfigAndStacksInfo,
	checkStack bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	authManager auth.AuthManager,
) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "generate.ExecAdapter.ProcessStacks")()

	return a.processStacks(atmosConfig, info, checkStack, processTemplates, processYamlFunctions, skip, authManager)
}

// FindStacksMap implements StackProcessor.FindStacksMap by delegating to internal/exec.FindStacksMap.
func (a *ExecAdapter) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, error) {
	defer perf.Track(atmosConfig, "generate.ExecAdapter.FindStacksMap")()

	return a.findStacksMap(atmosConfig, ignoreMissingFiles)
}
