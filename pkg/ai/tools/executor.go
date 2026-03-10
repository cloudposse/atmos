package tools

import (
	"context"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
)

const (
	// DefaultTimeout is the default timeout for tool execution.
	DefaultTimeout = 30 * time.Second
)

// Executor executes tools with permission checking.
type Executor struct {
	registry    *Registry
	permChecker *permission.Checker
	timeout     time.Duration
}

// NewExecutor creates a new tool executor.
func NewExecutor(registry *Registry, permChecker *permission.Checker, timeout time.Duration) *Executor {
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return &Executor{
		registry:    registry,
		permChecker: permChecker,
		timeout:     timeout,
	}
}

// Execute runs a tool with the given parameters.
func (e *Executor) Execute(ctx context.Context, toolName string, params map[string]interface{}) (*Result, error) {
	// Get tool from registry.
	tool, err := e.registry.Get(toolName)
	if err != nil {
		return nil, err
	}

	// Check permissions.
	if tool.RequiresPermission() || tool.IsRestricted() {
		allowed, err := e.permChecker.CheckPermission(ctx, tool, params)
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}

		if !allowed {
			return &Result{
				Success: false,
				Error:   errUtils.ErrAIToolExecutionDenied,
			}, errUtils.ErrAIToolExecutionDenied
		}
	}

	// Create timeout context.
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Execute tool.
	result, err := tool.Execute(execCtx, params)
	if err != nil {
		return &Result{
			Success: false,
			Error:   fmt.Errorf("%w: %w", errUtils.ErrAIToolExecutionFailed, err),
		}, err
	}

	return result, nil
}

// ExecuteBatch runs multiple tools in sequence.
func (e *Executor) ExecuteBatch(ctx context.Context, calls []ToolCall) ([]*Result, error) {
	results := make([]*Result, len(calls))

	for i, call := range calls {
		result, err := e.Execute(ctx, call.Tool, call.Params)
		if err != nil {
			// Continue with other tools even if one fails.
			results[i] = &Result{
				Success: false,
				Error:   err,
			}
			continue
		}

		results[i] = result
	}

	return results, nil
}

// ToolCall represents a tool execution request.
type ToolCall struct {
	Tool   string                 `json:"tool"`
	Params map[string]interface{} `json:"params"`
}

// ListTools returns all available tools from the registry.
func (e *Executor) ListTools() []Tool {
	if e.registry == nil {
		return nil
	}
	return e.registry.List()
}
