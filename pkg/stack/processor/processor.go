package processor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/stack/loader"
)

// yamlFunctionPrefix is the prefix for YAML function syntax.
const yamlFunctionPrefix = "!"

// Processor orchestrates function execution with stack context.
// It bridges the generic function package with stack-specific processing needs.
type Processor struct {
	functions *function.Registry
	loaders   *loader.Registry
}

// New creates a new Processor with the given registries.
func New(functions *function.Registry, loaders *loader.Registry) *Processor {
	defer perf.Track(nil, "processor.New")()

	return &Processor{
		functions: functions,
		loaders:   loaders,
	}
}

// FunctionRegistry returns the function registry.
func (p *Processor) FunctionRegistry() *function.Registry {
	defer perf.Track(nil, "processor.Processor.FunctionRegistry")()

	if p == nil {
		return nil
	}
	return p.functions
}

// LoaderRegistry returns the loader registry.
func (p *Processor) LoaderRegistry() *loader.Registry {
	defer perf.Track(nil, "processor.Processor.LoaderRegistry")()

	if p == nil {
		return nil
	}
	return p.loaders
}

// ProcessPreMerge processes pre-merge functions in the data.
// This is called during initial file loading, before configuration merging.
func (p *Processor) ProcessPreMerge(ctx context.Context, data any, sourceFile string) (any, error) {
	defer perf.Track(nil, "processor.Processor.ProcessPreMerge")()

	if p == nil {
		return nil, errUtils.ErrNilProcessor
	}

	execCtx := function.NewExecutionContext(nil, "", sourceFile)
	stackCtx := NewStackContext(execCtx)

	return p.processData(ctx, data, stackCtx, function.PreMerge)
}

// ProcessPostMerge processes post-merge functions in the data.
// This is called after configuration merging, when the full stack context is available.
func (p *Processor) ProcessPostMerge(ctx context.Context, stackCtx *StackContext, data any) (any, error) {
	defer perf.Track(nil, "processor.Processor.ProcessPostMerge")()

	if p == nil {
		return nil, errUtils.ErrNilProcessor
	}

	if stackCtx == nil {
		return nil, fmt.Errorf("%w: stack context is required for post-merge processing", errUtils.ErrProcessorNilContext)
	}

	return p.processData(ctx, data, stackCtx, function.PostMerge)
}

// processData recursively processes data, executing functions of the specified phase.
func (p *Processor) processData(ctx context.Context, data any, stackCtx *StackContext, phase function.Phase) (any, error) {
	// Check context cancellation.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	switch v := data.(type) {
	case map[string]any:
		return p.processMap(ctx, v, stackCtx, phase)
	case []any:
		return p.processSlice(ctx, v, stackCtx, phase)
	case string:
		return p.processString(ctx, v, stackCtx, phase)
	default:
		// Return other types as-is (numbers, booleans, nil, etc.).
		return data, nil
	}
}

// processMap processes a map, applying functions to all values.
func (p *Processor) processMap(ctx context.Context, data map[string]any, stackCtx *StackContext, phase function.Phase) (map[string]any, error) {
	result := make(map[string]any, len(data))

	for key, value := range data {
		processed, err := p.processData(ctx, value, stackCtx, phase)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		result[key] = processed
	}

	return result, nil
}

// processSlice processes a slice, applying functions to all elements.
func (p *Processor) processSlice(ctx context.Context, data []any, stackCtx *StackContext, phase function.Phase) ([]any, error) {
	result := make([]any, len(data))

	for i, value := range data {
		processed, err := p.processData(ctx, value, stackCtx, phase)
		if err != nil {
			return nil, fmt.Errorf("index %d: %w", i, err)
		}
		result[i] = processed
	}

	return result, nil
}

// processString checks if a string is a function call and executes it.
func (p *Processor) processString(ctx context.Context, value string, stackCtx *StackContext, phase function.Phase) (any, error) {
	// Check if this is a YAML function call (starts with !).
	funcName, args, isFunc := p.parseFunction(value)
	if !isFunc {
		return value, nil
	}

	// Check if function should be skipped.
	if stackCtx.ShouldSkip(funcName) {
		return value, nil
	}

	// Look up the function.
	fn, err := p.functions.Get(funcName)
	if errors.Is(err, function.ErrFunctionNotFound) {
		// Function not found - return original value.
		// This allows for custom YAML tags that aren't functions.
		return value, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup function %q: %w", funcName, err)
	}

	// Check if the function should run in this phase.
	if fn.Phase() != phase {
		return value, nil
	}

	// Execute the function.
	result, err := fn.Execute(ctx, args, stackCtx.ExecutionContext)
	if err != nil {
		return nil, fmt.Errorf("function %q: %w", funcName, err)
	}

	return result, nil
}

// parseFunction parses a function call string.
// Returns the function name, arguments, and whether it is a function.
func (p *Processor) parseFunction(value string) (name string, args string, isFunc bool) {
	defer perf.Track(nil, "processor.Processor.parseFunction")()

	// Check for YAML function syntax: !function_name args.
	if !strings.HasPrefix(value, yamlFunctionPrefix) {
		return "", "", false
	}

	// Remove the prefix.
	rest := strings.TrimPrefix(value, yamlFunctionPrefix)
	if rest == "" {
		return "", "", false
	}

	// Split into function name and arguments.
	parts := strings.SplitN(rest, " ", 2)
	name = parts[0]
	if len(parts) > 1 {
		args = parts[1]
	}

	return name, args, true
}

// DetectFunctions scans data for function calls and returns their names.
// This is useful for dependency analysis before processing.
func (p *Processor) DetectFunctions(data any) []string {
	defer perf.Track(nil, "processor.Processor.DetectFunctions")()

	if p == nil {
		return nil
	}

	var functions []string
	p.detectFunctionsRecursive(data, &functions)
	return functions
}

// detectFunctionsRecursive recursively scans data for function calls.
func (p *Processor) detectFunctionsRecursive(data any, functions *[]string) {
	switch v := data.(type) {
	case map[string]any:
		for _, value := range v {
			p.detectFunctionsRecursive(value, functions)
		}
	case []any:
		for _, item := range v {
			p.detectFunctionsRecursive(item, functions)
		}
	case string:
		if name, _, isFunc := p.parseFunction(v); isFunc {
			*functions = append(*functions, name)
		}
	}
}

// HasFunctions returns true if the data contains any function calls.
func (p *Processor) HasFunctions(data any) bool {
	defer perf.Track(nil, "processor.Processor.HasFunctions")()

	return len(p.DetectFunctions(data)) > 0
}

// HasPreMergeFunctions returns true if the data contains pre-merge function calls.
func (p *Processor) HasPreMergeFunctions(data any) bool {
	defer perf.Track(nil, "processor.Processor.HasPreMergeFunctions")()

	for _, name := range p.DetectFunctions(data) {
		if fn, err := p.functions.Get(name); err == nil && fn.Phase() == function.PreMerge {
			return true
		}
	}
	return false
}

// HasPostMergeFunctions returns true if the data contains post-merge function calls.
func (p *Processor) HasPostMergeFunctions(data any) bool {
	defer perf.Track(nil, "processor.Processor.HasPostMergeFunctions")()

	for _, name := range p.DetectFunctions(data) {
		if fn, err := p.functions.Get(name); err == nil && fn.Phase() == function.PostMerge {
			return true
		}
	}
	return false
}
