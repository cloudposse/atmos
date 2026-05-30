package step

import (
	"context"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen@latest -source=registry.go -destination=mock_handler_test.go -package=step

// StepHandler defines the interface for workflow step type handlers.
type StepHandler interface {
	// GetName returns the step type name (e.g., "input", "choose", "success").
	GetName() string

	// GetCategory returns the step category for grouping.
	GetCategory() StepCategory

	// Execute runs the step and returns the result.
	Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error)

	// Validate checks step configuration before execution.
	Validate(step *schema.WorkflowStep) error

	// RequiresTTY returns true if the step requires an interactive terminal.
	RequiresTTY() bool
}

// Registry manages step type handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]StepHandler
}

// Global registry instance.
var registry = &Registry{handlers: make(map[string]StepHandler)}

// Register adds a step handler to the registry.
// Called from init() in each handler file.
// If a handler with the same name already exists, it is replaced.
func Register(handler StepHandler) {
	defer perf.Track(nil, "step.Register")()

	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.handlers[handler.GetName()] = handler
}

// Get returns a handler by type name.
func Get(typeName string) (StepHandler, bool) {
	defer perf.Track(nil, "step.Get")()

	registry.mu.RLock()
	defer registry.mu.RUnlock()
	h, ok := registry.handlers[typeName]
	return h, ok
}

// List returns all registered handlers.
func List() map[string]StepHandler {
	defer perf.Track(nil, "step.List")()

	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make(map[string]StepHandler, len(registry.handlers))
	for k, v := range registry.handlers {
		result[k] = v
	}
	return result
}

// ListByCategory returns handlers grouped by category.
func ListByCategory() map[StepCategory][]StepHandler {
	defer perf.Track(nil, "step.ListByCategory")()

	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make(map[StepCategory][]StepHandler)
	for _, h := range registry.handlers {
		cat := h.GetCategory()
		result[cat] = append(result[cat], h)
	}
	return result
}

// Count returns the number of registered handlers.
func Count() int {
	defer perf.Track(nil, "step.Count")()

	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return len(registry.handlers)
}

// Reset clears the registry. For testing only.
func Reset() {
	defer perf.Track(nil, "step.Reset")()

	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.handlers = make(map[string]StepHandler)
}
