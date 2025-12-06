package exec

import (
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentYAMLProcessor processes YAML functions for a specific component.
// It implements the merge.YAMLFunctionProcessor interface.
type ComponentYAMLProcessor struct {
	atmosConfig   *schema.AtmosConfiguration
	currentStack  string
	skip          []string
	resolutionCtx *ResolutionContext
	stackInfo     *schema.ConfigAndStacksInfo
}

// NewComponentYAMLProcessor creates a new YAML processor for component merging.
func NewComponentYAMLProcessor(
	atmosConfig *schema.AtmosConfiguration,
	currentStack string,
	skip []string,
	resolutionCtx *ResolutionContext,
	stackInfo *schema.ConfigAndStacksInfo,
) m.YAMLFunctionProcessor {
	defer perf.Track(atmosConfig, "exec.NewComponentYAMLProcessor")()

	return &ComponentYAMLProcessor{
		atmosConfig:   atmosConfig,
		currentStack:  currentStack,
		skip:          skip,
		resolutionCtx: resolutionCtx,
		stackInfo:     stackInfo,
	}
}

// ProcessYAMLFunctionString processes a YAML function string and returns the processed value.
// This method recursively processes the string to handle all YAML function types:
// - !template: Go template rendering
// - !terraform.output: Terraform output from other components
// - !terraform.state: Terraform state queries
// - !store.get, !store: Store lookups
// - !exec: Command execution
// - !env: Environment variable expansion.
func (p *ComponentYAMLProcessor) ProcessYAMLFunctionString(value string) (any, error) {
	defer perf.Track(p.atmosConfig, "exec.ComponentYAMLProcessor.ProcessYAMLFunctionString")()

	// Process the YAML function using existing function processors.
	// processCustomTagsWithContext handles all YAML function types and skip list.
	return processCustomTagsWithContext(
		p.atmosConfig,
		value,
		p.currentStack,
		p.skip,
		p.resolutionCtx,
		p.stackInfo,
	), nil
}
