package function

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Store function tags.
const (
	TagStore    = "!store"
	TagStoreGet = "!store.get"
)

// StoreFunction implements the !store YAML function.
// It retrieves values from configured stores.
// Note: This is a PostMerge function that requires stack context.
// During HCL parsing, it returns a placeholder for later resolution.
type StoreFunction struct {
	BaseFunction
}

// NewStoreFunction creates a new StoreFunction.
func NewStoreFunction() *StoreFunction {
	defer perf.Track(nil, "function.NewStoreFunction")()

	return &StoreFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "store",
			FunctionAliases: []string{"store.get"},
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !store store_name key
// Syntax: !store.get store_name key
// The actual store lookup happens during YAML post-merge processing.
func (f *StoreFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.StoreFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: !store requires arguments: store_name key", ErrInvalidArguments)
	}

	// Return placeholder with the original arguments for post-merge resolution.
	return fmt.Sprintf("%s %s", TagStore, args), nil
}

// StoreGetFunction implements the !store.get YAML function.
// This is an alias for !store with explicit .get suffix.
// Note: This is a PostMerge function that requires stack context.
type StoreGetFunction struct {
	BaseFunction
}

// NewStoreGetFunction creates a new StoreGetFunction.
func NewStoreGetFunction() *StoreGetFunction {
	defer perf.Track(nil, "function.NewStoreGetFunction")()

	return &StoreGetFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "store.get",
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute returns a placeholder for post-merge resolution.
// Syntax: !store.get store_name key
// The actual store lookup happens during YAML post-merge processing.
func (f *StoreGetFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.StoreGetFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: !store.get requires arguments: store_name key", ErrInvalidArguments)
	}

	// Return placeholder with the original arguments for post-merge resolution.
	return fmt.Sprintf("%s %s", TagStoreGet, args), nil
}
