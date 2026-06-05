package function

import (
	"context"
	"fmt"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
)

// terraformArgs holds parsed terraform function arguments.
type terraformArgs struct {
	component string
	stack     string
	output    string
}

// parseTerraformArgs parses terraform function arguments (component, stack, output).
// Arguments can be either 2 or 3 parts:
//   - 2 parts: component output_name (stack from context)
//   - 3 parts: component stack output_name
func parseTerraformArgs(args string, execCtx *ExecutionContext) (*terraformArgs, error) {
	parts, err := utils.SplitStringByDelimiter(args, ' ')
	if err != nil {
		return nil, err
	}

	var component, stack, output string

	switch len(parts) {
	case 3:
		component = strings.TrimSpace(parts[0])
		stack = strings.TrimSpace(parts[1])
		output = strings.TrimSpace(parts[2])
	case 2:
		component = strings.TrimSpace(parts[0])
		stack = execCtx.Stack
		output = strings.TrimSpace(parts[1])
	default:
		return nil, fmt.Errorf("%w: terraform function requires 2 or 3 arguments, got %d", ErrInvalidArguments, len(parts))
	}

	return &terraformArgs{component: component, stack: stack, output: output}, nil
}

// TerraformOutputFunction implements the terraform.output function.
type TerraformOutputFunction struct {
	BaseFunction
}

// NewTerraformOutputFunction creates a new terraform.output function handler.
func NewTerraformOutputFunction() *TerraformOutputFunction {
	defer perf.Track(nil, "function.NewTerraformOutputFunction")()

	return &TerraformOutputFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagTerraformOutput,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the terraform.output function.
// Usage:
//
//	!terraform.output component output_name
//	!terraform.output component stack output_name
//
// Note: This is a placeholder that parses args. The actual terraform output
// retrieval is handled by internal/exec which has the full implementation.
func (f *TerraformOutputFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TerraformOutputFunction.Execute")()

	log.Debug("Executing terraform.output function", "args", args)

	if execCtx == nil || execCtx.AtmosConfig == nil {
		return nil, fmt.Errorf("%w: terraform.output function requires AtmosConfig", ErrExecutionFailed)
	}

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: terraform.output function requires arguments", ErrInvalidArguments)
	}

	// Parse arguments.
	parsed, err := parseTerraformArgs(args, execCtx)
	if err != nil {
		return nil, err
	}

	log.Debug("Parsed terraform.output args", "component", parsed.component, "stack", parsed.stack, "output", parsed.output)

	// TODO: The actual implementation requires outputGetter.GetOutput and other
	// helpers from internal/exec. For now, return a placeholder error.
	// The migration will update internal/exec to call this function.
	return nil, fmt.Errorf("%w: terraform.output not yet fully migrated: component=%s stack=%s output=%s", ErrExecutionFailed, parsed.component, parsed.stack, parsed.output)
}

// TerraformStateFunction implements the terraform.state function.
type TerraformStateFunction struct {
	BaseFunction
}

// NewTerraformStateFunction creates a new terraform.state function handler.
func NewTerraformStateFunction() *TerraformStateFunction {
	defer perf.Track(nil, "function.NewTerraformStateFunction")()

	return &TerraformStateFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagTerraformState,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the terraform.state function.
// Usage:
//
//	!terraform.state component output_name
//	!terraform.state component stack output_name
//
// Note: This is a placeholder that parses args. The actual terraform state
// retrieval is handled by internal/exec which has the full implementation.
func (f *TerraformStateFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TerraformStateFunction.Execute")()

	log.Debug("Executing terraform.state function", "args", args)

	if execCtx == nil || execCtx.AtmosConfig == nil {
		return nil, fmt.Errorf("%w: terraform.state function requires AtmosConfig", ErrExecutionFailed)
	}

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: terraform.state function requires arguments", ErrInvalidArguments)
	}

	// Parse arguments.
	parsed, err := parseTerraformArgs(args, execCtx)
	if err != nil {
		return nil, err
	}

	log.Debug("Parsed terraform.state args", "component", parsed.component, "stack", parsed.stack, "output", parsed.output)

	// TODO: The actual implementation requires state retrieval helpers from internal/exec.
	// For now, return a placeholder error.
	return nil, fmt.Errorf("%w: terraform.state not yet fully migrated: component=%s stack=%s output=%s", ErrExecutionFailed, parsed.component, parsed.stack, parsed.output)
}
