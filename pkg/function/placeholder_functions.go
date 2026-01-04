package function

import (
	"context"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Store function tags.
const (
	TagStore    = "!store"
	TagStoreGet = "!store.get"
)

// Terraform function tags.
const (
	TagTerraformOutput = "!terraform.output"
	TagTerraformState  = "!terraform.state"
)

// StoreFunction implements the !store YAML function.
// It retrieves values from configured stores.
// Note: This is a PostMerge function that requires stack context.
// During HCL parsing, it returns a placeholder for later resolution.
type StoreFunction struct {
	PlaceholderFunction
}

// NewStoreFunction creates a new StoreFunction.
func NewStoreFunction() *StoreFunction {
	defer perf.Track(nil, "function.NewStoreFunction")()

	return &StoreFunction{
		PlaceholderFunction: NewPlaceholderFunction("store", TagStore, "store_name key"),
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !store store_name key
// The actual store lookup happens during YAML post-merge processing.
func (f *StoreFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.StoreFunction.Execute")()

	return f.PlaceholderFunction.Execute(ctx, args, execCtx)
}

// StoreGetFunction implements the !store.get YAML function.
// This is an alias for !store with explicit .get suffix.
// Note: This is a PostMerge function that requires stack context.
type StoreGetFunction struct {
	PlaceholderFunction
}

// NewStoreGetFunction creates a new StoreGetFunction.
func NewStoreGetFunction() *StoreGetFunction {
	defer perf.Track(nil, "function.NewStoreGetFunction")()

	return &StoreGetFunction{
		PlaceholderFunction: NewPlaceholderFunction("store.get", TagStoreGet, "store_name key"),
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !store.get store_name key
// The actual store lookup happens during YAML post-merge processing.
func (f *StoreGetFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.StoreGetFunction.Execute")()

	return f.PlaceholderFunction.Execute(ctx, args, execCtx)
}

// TerraformOutputFunction implements the !terraform.output YAML function.
// It retrieves Terraform outputs from remote state.
// Note: This is a PostMerge function that requires stack context.
// During HCL parsing, it returns a placeholder for later resolution.
type TerraformOutputFunction struct {
	PlaceholderFunction
}

// NewTerraformOutputFunction creates a new TerraformOutputFunction.
func NewTerraformOutputFunction() *TerraformOutputFunction {
	defer perf.Track(nil, "function.NewTerraformOutputFunction")()

	return &TerraformOutputFunction{
		PlaceholderFunction: NewPlaceholderFunction("terraform.output", TagTerraformOutput, "component [stack] output"),
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !terraform.output component stack output
// Syntax: !terraform.output component output (uses current stack)
// The actual Terraform state lookup happens during YAML post-merge processing.
func (f *TerraformOutputFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TerraformOutputFunction.Execute")()

	return f.PlaceholderFunction.Execute(ctx, args, execCtx)
}

// TerraformStateFunction implements the !terraform.state YAML function.
// It queries Terraform state directly from backends.
// Note: This is a PostMerge function that requires stack context.
// During HCL parsing, it returns a placeholder for later resolution.
type TerraformStateFunction struct {
	PlaceholderFunction
}

// NewTerraformStateFunction creates a new TerraformStateFunction.
func NewTerraformStateFunction() *TerraformStateFunction {
	defer perf.Track(nil, "function.NewTerraformStateFunction")()

	return &TerraformStateFunction{
		PlaceholderFunction: NewPlaceholderFunction("terraform.state", TagTerraformState, "component [stack] output"),
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !terraform.state component stack output
// Syntax: !terraform.state component output (uses current stack)
// The actual Terraform state lookup happens during YAML post-merge processing.
func (f *TerraformStateFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.TerraformStateFunction.Execute")()

	return f.PlaceholderFunction.Execute(ctx, args, execCtx)
}
