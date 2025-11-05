package internal

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Context keys for passing values through cobra command context.
type contextKey string

// IoContextKey is the key for storing io.Context in cobra command context.
const IoContextKey contextKey = "ioContext"

// registry is the global command registry instance.
var registry = &CommandRegistry{
	providers: make(map[string]CommandProvider),
}

// CommandRegistry manages built-in command registration.
//
// This registry is for BUILT-IN commands only. Custom commands from atmos.yaml
// are processed separately via processCustomCommands() in cmd/cmd_utils.go.
//
// The registry uses a singleton pattern with package-level registration
// functions for convenience. Commands register themselves during package
// initialization via init() functions.
type CommandRegistry struct {
	mu        sync.RWMutex
	providers map[string]CommandProvider
}

// Register adds a built-in command provider to the registry.
//
// This function is typically called during package initialization via init():
//
//	func init() {
//	    internal.Register(&AboutCommandProvider{})
//	}
//
// If a provider with the same name is already registered, the new provider
// replaces the existing one. This allows for plugin override functionality
// in the future and makes testing easier.
func Register(provider CommandProvider) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	name := provider.GetName()

	// Note: We allow re-registration for flexibility in testing and future
	// plugin override functionality. In production, each built-in command
	// should only register once during init().
	// The existence check is intentionally not used - we simply overwrite.
	registry.providers[name] = provider
}

// RegisterAll registers all built-in commands with the root command.
//
// This function should be called once during application initialization
// in cmd/root.go init(). It registers all commands that have been added
// to the registry via Register().
//
// Custom commands from atmos.yaml are processed AFTER this function
// via processCustomCommands(), allowing custom commands to extend or
// override built-in commands.
//
// Example usage in cmd/root.go:
//
//	func init() {
//	    // Register built-in commands
//	    if err := internal.RegisterAll(RootCmd); err != nil {
//	        log.Error("Failed to register built-in commands", "error", err)
//	    }
//	}
func RegisterAll(root *cobra.Command) error {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	for name, provider := range registry.providers {
		cmd := provider.GetCommand()
		if cmd == nil {
			return fmt.Errorf("%w: provider %s", errUtils.ErrCommandNil, name)
		}

		root.AddCommand(cmd)
	}

	return nil
}

// GetProvider returns a built-in command provider by name.
//
// This function is primarily used for testing and diagnostics.
// Returns the provider and true if found, nil and false otherwise.
func GetProvider(name string) (CommandProvider, bool) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	provider, ok := registry.providers[name]
	return provider, ok
}

// ListProviders returns all registered providers grouped by category.
//
// This function is useful for generating help text and diagnostics.
// The returned map uses group names as keys and slices of providers as values.
//
// Example output:
//
//	{
//	    "Core Stack Commands": [terraform, helmfile, workflow],
//	    "Stack Introspection": [describe, list, validate],
//	    ...
//	}
func ListProviders() map[string][]CommandProvider {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	grouped := make(map[string][]CommandProvider)

	for _, provider := range registry.providers {
		group := provider.GetGroup()
		grouped[group] = append(grouped[group], provider)
	}

	return grouped
}

// Count returns the number of registered built-in providers.
//
// This function is primarily used for testing and diagnostics.
func Count() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	return len(registry.providers)
}

// Reset clears the registry, removing all registered providers.
//
// WARNING: This function is for TESTING ONLY. It should never be called
// in production code. It allows tests to start with a clean registry state.
func Reset() {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.providers = make(map[string]CommandProvider)
}
