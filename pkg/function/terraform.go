package function

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Terraform function tags.
const (
	TagTerraformOutput = "!terraform.output"
	TagTerraformState  = "!terraform.state"
)

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
