package mcp

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Adapter adapts existing Atmos AI tools for use with MCP SDK.
// It provides a bridge between Atmos's tool system and the MCP protocol.
type Adapter struct {
	registry *tools.Registry
	executor *tools.Executor
}

// NewAdapter creates a new tool adapter.
func NewAdapter(registry *tools.Registry, executor *tools.Executor) *Adapter {
	defer perf.Track(nil, "mcp.NewAdapter")()

	return &Adapter{
		registry: registry,
		executor: executor,
	}
}

// ExecuteTool executes a tool and returns the result.
func (a *Adapter) ExecuteTool(ctx context.Context, name string, arguments map[string]interface{}) (*tools.Result, error) {
	defer perf.Track(nil, "mcp.Adapter.ExecuteTool")()

	// Execute the tool using the existing executor.
	result, err := a.executor.Execute(ctx, name, arguments)
	if err != nil {
		// Return error in result format for better error handling.
		return &tools.Result{
			Success: false,
			Output:  "",
			Error:   fmt.Errorf("tool execution failed: %w", err),
		}, nil
	}

	return result, nil
}
