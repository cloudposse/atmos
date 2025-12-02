package function

import (
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Registry manages registered functions and provides lookup capabilities.
type Registry struct {
	mu        sync.RWMutex
	functions map[string]Function // Primary name -> Function.
	aliases   map[string]string   // Alias -> Primary name.
}

// NewRegistry creates a new empty function registry.
func NewRegistry() *Registry {
	defer perf.Track(nil, "function.NewRegistry")()

	return &Registry{
		functions: make(map[string]Function),
		aliases:   make(map[string]string),
	}
}

// Register adds a function to the registry.
// Returns an error if a function with the same name or alias already exists.
func (r *Registry) Register(fn Function) error {
	defer perf.Track(nil, "function.Registry.Register")()

	r.mu.Lock()
	defer r.mu.Unlock()

	name := fn.Name()

	// Check if the name is already registered.
	if _, exists := r.functions[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateFunction, name)
	}

	// Check if the name conflicts with an alias.
	if _, exists := r.aliases[name]; exists {
		return fmt.Errorf("%w: %s is registered as an alias", ErrDuplicateFunction, name)
	}

	// Check if any aliases conflict.
	for _, alias := range fn.Aliases() {
		if _, exists := r.functions[alias]; exists {
			return fmt.Errorf("%w: alias %s conflicts with function name", ErrDuplicateFunction, alias)
		}
		if existingPrimary, exists := r.aliases[alias]; exists {
			return fmt.Errorf("%w: alias %s already registered for %s", ErrDuplicateFunction, alias, existingPrimary)
		}
	}

	// Register the function.
	r.functions[name] = fn

	// Register all aliases.
	for _, alias := range fn.Aliases() {
		r.aliases[alias] = name
	}

	return nil
}

// Get retrieves a function by name or alias.
// Returns ErrFunctionNotFound if the function doesn't exist.
func (r *Registry) Get(name string) (Function, error) {
	defer perf.Track(nil, "function.Registry.Get")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try direct lookup.
	if fn, exists := r.functions[name]; exists {
		return fn, nil
	}

	// Try alias lookup.
	if primaryName, exists := r.aliases[name]; exists {
		if fn, exists := r.functions[primaryName]; exists {
			return fn, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, name)
}

// Has returns true if a function with the given name or alias exists.
func (r *Registry) Has(name string) bool {
	defer perf.Track(nil, "function.Registry.Has")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.functions[name]; exists {
		return true
	}
	if _, exists := r.aliases[name]; exists {
		return true
	}
	return false
}

// GetByPhase returns all functions that should run in the specified phase.
func (r *Registry) GetByPhase(phase Phase) []Function {
	defer perf.Track(nil, "function.Registry.GetByPhase")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Function
	for _, fn := range r.functions {
		if fn.Phase() == phase {
			result = append(result, fn)
		}
	}
	return result
}

// List returns all registered function names (not aliases).
func (r *Registry) List() []string {
	defer perf.Track(nil, "function.Registry.List")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.functions))
	for name := range r.functions {
		names = append(names, name)
	}
	return names
}

// Len returns the number of registered functions.
func (r *Registry) Len() int {
	defer perf.Track(nil, "function.Registry.Len")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.functions)
}

// Unregister removes a function from the registry.
// Also removes all associated aliases.
func (r *Registry) Unregister(name string) error {
	defer perf.Track(nil, "function.Registry.Unregister")()

	r.mu.Lock()
	defer r.mu.Unlock()

	fn, exists := r.functions[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrFunctionNotFound, name)
	}

	// Remove aliases first.
	for _, alias := range fn.Aliases() {
		delete(r.aliases, alias)
	}

	// Remove the function.
	delete(r.functions, name)

	return nil
}

// Clear removes all functions from the registry.
func (r *Registry) Clear() {
	defer perf.Track(nil, "function.Registry.Clear")()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.functions = make(map[string]Function)
	r.aliases = make(map[string]string)
}
