package function

import (
	"context"
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// IncludeFunction implements the include function for including content from files.
type IncludeFunction struct {
	BaseFunction
}

// NewIncludeFunction creates a new include function handler.
func NewIncludeFunction() *IncludeFunction {
	defer perf.Track(nil, "function.NewIncludeFunction")()

	return &IncludeFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagInclude,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the include function.
// Usage:
//
//	!include path/to/file.yaml
//	!include path/to/file.yaml .query.expression
//
// Note: The include function is special - it operates on yaml.Node directly
// and cannot return arbitrary values like other functions. The actual
// implementation remains in pkg/utils/yaml_include_by_extension.go which
// modifies the yaml.Node in-place.
//
// This function serves as a marker for the registry but the actual processing
// is handled specially in the YAML processor.
func (f *IncludeFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.IncludeFunction.Execute")()

	log.Debug("Executing include function", "args", args)

	// The include function requires special handling because it modifies
	// yaml.Node directly. This placeholder returns an error.
	return nil, fmt.Errorf("%w: include", ErrSpecialYAMLHandling)
}

// IncludeRawFunction implements the include.raw function for including raw file content.
type IncludeRawFunction struct {
	BaseFunction
}

// NewIncludeRawFunction creates a new include.raw function handler.
func NewIncludeRawFunction() *IncludeRawFunction {
	defer perf.Track(nil, "function.NewIncludeRawFunction")()

	return &IncludeRawFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagIncludeRaw,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the include.raw function.
// Usage:
//
//	!include.raw path/to/file.txt
//
// Note: Like include, this function operates on yaml.Node directly.
func (f *IncludeRawFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.IncludeRawFunction.Execute")()

	log.Debug("Executing include.raw function", "args", args)

	return nil, fmt.Errorf("%w: include.raw", ErrSpecialYAMLHandling)
}
