package function

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Function defines the interface for a configuration function.
// Functions process special syntax in configuration files (e.g., !env, !terraform.output).
type Function interface {
	// Name returns the primary function name (e.g., "env", "terraform.output").
	Name() string

	// Aliases returns alternative names for this function.
	// For example, "store" might be an alias for "store.get".
	Aliases() []string

	// Phase returns when this function should be executed.
	// PreMerge functions run during initial file loading.
	// PostMerge functions run after configuration merging.
	Phase() Phase

	// Execute runs the function with the given arguments.
	// The args string contains everything after the function name in the syntax.
	// Returns the processed value or an error.
	Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error)
}

// BaseFunction provides a base implementation for Function that can be embedded.
type BaseFunction struct {
	FunctionName    string
	FunctionAliases []string
	FunctionPhase   Phase
}

// Name returns the function name.
func (f *BaseFunction) Name() string {
	defer perf.Track(nil, "function.BaseFunction.Name")()

	return f.FunctionName
}

// Aliases returns the function aliases.
func (f *BaseFunction) Aliases() []string {
	defer perf.Track(nil, "function.BaseFunction.Aliases")()

	if f.FunctionAliases == nil {
		return []string{}
	}
	return f.FunctionAliases
}

// Phase returns the function phase.
func (f *BaseFunction) Phase() Phase {
	defer perf.Track(nil, "function.BaseFunction.Phase")()

	return f.FunctionPhase
}

// PlaceholderFunction is a base for PostMerge functions that return a placeholder.
// These functions defer actual execution to post-merge processing.
type PlaceholderFunction struct {
	BaseFunction
	Tag      string // The tag to prefix the placeholder (e.g., "!store").
	ArgsHelp string // Help text for required arguments (e.g., "store_name key").
}

// NewPlaceholderFunction creates a new PlaceholderFunction.
func NewPlaceholderFunction(name, tag, argsHelp string) PlaceholderFunction {
	defer perf.Track(nil, "function.NewPlaceholderFunction")()

	return PlaceholderFunction{
		BaseFunction: BaseFunction{
			FunctionName:    name,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
		Tag:      tag,
		ArgsHelp: argsHelp,
	}
}

// Execute returns a placeholder for post-merge resolution.
func (f *PlaceholderFunction) Execute(_ context.Context, args string, _ *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.PlaceholderFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: %s requires arguments: %s", ErrInvalidArguments, f.Tag, f.ArgsHelp)
	}

	// Return placeholder with the original arguments for post-merge resolution.
	return fmt.Sprintf("%s %s", f.Tag, args), nil
}
