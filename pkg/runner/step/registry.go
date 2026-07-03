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

// aliasedHandler is implemented by handlers that respond to alternate type names
// in addition to their canonical GetName(). Aliases resolve via Get() but are not
// reported by List/ListByCategory/Count, so they never appear as duplicate entries.
type aliasedHandler interface {
	GetAliases() []string
}

// Registry manages step type handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]StepHandler // Canonical type name -> handler.
	aliases  map[string]StepHandler // Alias type name -> canonical handler.
}

// Global registry instance.
var registry = &Registry{
	handlers: make(map[string]StepHandler),
	aliases:  make(map[string]StepHandler),
}

// Register adds a step handler to the registry.
// Called from init() in each handler file.
// If a handler with the same name already exists, it is replaced.
func Register(handler StepHandler) {
	defer perf.Track(nil, "step.Register")()

	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.handlers[handler.GetName()] = handler
	if aliased, ok := handler.(aliasedHandler); ok {
		for _, alias := range aliased.GetAliases() {
			registry.aliases[alias] = handler
		}
	}
}

// Get returns a handler by type name, falling back to registered aliases.
func Get(typeName string) (StepHandler, bool) {
	defer perf.Track(nil, "step.Get")()

	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if h, ok := registry.handlers[typeName]; ok {
		return h, true
	}
	h, ok := registry.aliases[typeName]
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
