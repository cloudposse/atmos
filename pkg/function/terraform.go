package function

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Terraform function tags are defined in tags.go.
// Use YAMLTag(TagTerraformOutput) and YAMLTag(TagTerraformState) to get the YAML tag format.

// terraformArgs holds parsed arguments for terraform functions.
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
	// Split by whitespace - Terraform component/stack/output names don't contain spaces.
	parts := strings.Fields(args)

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

// TerraformOutputFunction implements the !terraform.output YAML function.
// It retrieves Terraform outputs from remote state.
// Note: This is a PostMerge function that requires stack context.
// During HCL parsing, it returns a placeholder for later resolution.
type TerraformOutputFunction struct {
	BaseFunction
}

// NewTerraformOutputFunction creates a new TerraformOutputFunction.
func NewTerraformOutputFunction() *TerraformOutputFunction {
	defer perf.Track(nil, "function.NewTerraformOutputFunction")()

	return &TerraformOutputFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "terraform.output",
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !terraform.output component stack output
// Syntax: !terraform.output component output (uses current stack)
// The actual Terraform state lookup happens during YAML post-merge processing.
func (f *TerraformOutputFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TerraformOutputFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: !terraform.output requires arguments: component [stack] output", ErrInvalidArguments)
	}

	// Return placeholder with the original arguments for post-merge resolution.
	return fmt.Sprintf("%s %s", TagTerraformOutput, args), nil
}

// TerraformStateFunction implements the !terraform.state YAML function.
// It queries Terraform state directly from backends.
// Note: This is a PostMerge function that requires stack context.
// During HCL parsing, it returns a placeholder for later resolution.
type TerraformStateFunction struct {
	BaseFunction
}

// NewTerraformStateFunction creates a new TerraformStateFunction.
func NewTerraformStateFunction() *TerraformStateFunction {
	defer perf.Track(nil, "function.NewTerraformStateFunction")()

	return &TerraformStateFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "terraform.state",
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !terraform.state component stack output
// Syntax: !terraform.state component output (uses current stack)
// The actual Terraform state lookup happens during YAML post-merge processing.
func (f *TerraformStateFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TerraformStateFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: !terraform.state requires arguments: component [stack] output", ErrInvalidArguments)
	}

	// Return placeholder with the original arguments for post-merge resolution.
	return fmt.Sprintf("%s %s", TagTerraformState, args), nil
}
