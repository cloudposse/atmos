package function

import (
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Registry is a thread-safe registry for Function implementations.
type Registry struct {
	mu        sync.RWMutex
	functions map[string]Function
	aliases   map[string]string // alias -> primary name
}

// NewRegistry creates a new empty function registry.
func NewRegistry() *Registry {
	defer perf.Track(nil, "function.NewRegistry")()

	return &Registry{
		functions: make(map[string]Function),
		aliases:   make(map[string]string),
	}
}

// defaultRegistry is the global registry instance.
var (
	defaultRegistry     *Registry
	defaultRegistryOnce sync.Once
)

// DefaultRegistry returns the global function registry.
func DefaultRegistry() *Registry {
	defer perf.Track(nil, "function.DefaultRegistry")()

	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
	})
	return defaultRegistry
}

// Register adds a function to the registry.
// Returns an error if the name or any alias is already registered.
func (r *Registry) Register(fn Function) error {
	defer perf.Track(nil, "function.Registry.Register")()

	r.mu.Lock()
	defer r.mu.Unlock()

	name := strings.ToLower(fn.Name())

	// Check if primary name conflicts.
	if _, exists := r.functions[name]; exists {
		return ErrFunctionAlreadyRegistered
	}
	if _, exists := r.aliases[name]; exists {
		return ErrFunctionAlreadyRegistered
	}

	// Check if any alias conflicts.
	for _, alias := range fn.Aliases() {
		alias = strings.ToLower(alias)
		if _, exists := r.functions[alias]; exists {
			return ErrFunctionAlreadyRegistered
		}
		if _, exists := r.aliases[alias]; exists {
			return ErrFunctionAlreadyRegistered
		}
	}

	// Register the function and its aliases.
	r.functions[name] = fn
	for _, alias := range fn.Aliases() {
		r.aliases[strings.ToLower(alias)] = name
	}

	return nil
}

// Get retrieves a function by name or alias.
// Returns ErrFunctionNotFound if the function is not registered.
func (r *Registry) Get(name string) (Function, error) {
	defer perf.Track(nil, "function.Registry.Get")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	name = strings.ToLower(name)

	// Check primary names first.
	if fn, exists := r.functions[name]; exists {
		return fn, nil
	}

	// Check aliases.
	if primaryName, exists := r.aliases[name]; exists {
		if fn, exists := r.functions[primaryName]; exists {
			return fn, nil
		}
	}

	return nil, ErrFunctionNotFound
}

// Has checks if a function is registered by name or alias.
func (r *Registry) Has(name string) bool {
	defer perf.Track(nil, "function.Registry.Has")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	name = strings.ToLower(name)

	if _, exists := r.functions[name]; exists {
		return true
	}
	if _, exists := r.aliases[name]; exists {
		return true
	}
	return false
}

// GetByPhase returns all functions that should execute in the given phase.
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

// List returns all registered function names.
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
func (r *Registry) Unregister(name string) {
	defer perf.Track(nil, "function.Registry.Unregister")()

	r.mu.Lock()
	defer r.mu.Unlock()

	name = strings.ToLower(name)

	fn, exists := r.functions[name]
	if !exists {
		return
	}

	// Remove aliases first.
	for _, alias := range fn.Aliases() {
		delete(r.aliases, strings.ToLower(alias))
	}

	// Remove the function.
	delete(r.functions, name)
}

// Clear removes all functions from the registry.
func (r *Registry) Clear() {
	defer perf.Track(nil, "function.Registry.Clear")()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.functions = make(map[string]Function)
	r.aliases = make(map[string]string)
}
