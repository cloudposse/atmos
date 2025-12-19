package config

import (
	"context"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	importAdapterRegistry = &ImportAdapterRegistry{
		adapters: make([]ImportAdapter, 0),
	}
	initAdaptersOnce sync.Once
)

// ImportAdapterRegistry manages import adapter registration and lookup.
// Thread-safe for concurrent access.
type ImportAdapterRegistry struct {
	mu             sync.RWMutex
	adapters       []ImportAdapter // Ordered list for prefix matching.
	defaultAdapter ImportAdapter   // Fallback adapter (LocalAdapter).
}

// RegisterImportAdapter adds an import adapter to the registry.
// Adapters are matched in registration order, so register more specific
// schemes before less specific ones.
//
// Call this from init() functions to self-register adapters.
func RegisterImportAdapter(adapter ImportAdapter) {
	defer perf.Track(nil, "config.RegisterImportAdapter")()

	if adapter == nil {
		return
	}

	importAdapterRegistry.mu.Lock()
	defer importAdapterRegistry.mu.Unlock()

	// Check if this is the default adapter (no schemes).
	schemes := adapter.Schemes()
	if len(schemes) == 0 {
		importAdapterRegistry.defaultAdapter = adapter
		return
	}

	importAdapterRegistry.adapters = append(importAdapterRegistry.adapters, adapter)
}

// initBuiltinAdapters is called by initAdaptersOnce to register built-in adapters.
// This function pointer is set by the adapters package to avoid circular imports.
var initBuiltinAdapters func()

// SetBuiltinAdaptersInitializer sets the function used to initialize built-in adapters.
// This should be called from the adapters package's init() function.
func SetBuiltinAdaptersInitializer(f func()) {
	initBuiltinAdapters = f
}

// EnsureAdaptersInitialized ensures all built-in adapters are registered.
// This is called automatically by FindImportAdapter.
func EnsureAdaptersInitialized() {
	initAdaptersOnce.Do(func() {
		if initBuiltinAdapters != nil {
			initBuiltinAdapters()
		}
	})
}

// FindImportAdapter returns the appropriate adapter for the given import path.
// It checks registered adapters' schemes in order and returns the first match.
// If no adapter matches, returns the default adapter (LocalAdapter).
//
// This function always returns an adapter - never nil.
func FindImportAdapter(importPath string) ImportAdapter {
	defer perf.Track(nil, "config.FindImportAdapter")()

	// Ensure adapters are initialized on first use.
	EnsureAdaptersInitialized()

	importAdapterRegistry.mu.RLock()
	defer importAdapterRegistry.mu.RUnlock()

	loweredPath := strings.ToLower(importPath)

	// Check each registered adapter's schemes.
	for _, adapter := range importAdapterRegistry.adapters {
		for _, scheme := range adapter.Schemes() {
			if strings.HasPrefix(loweredPath, strings.ToLower(scheme)) {
				return adapter
			}
		}
	}

	// Return default adapter if no scheme matched.
	if importAdapterRegistry.defaultAdapter != nil {
		return importAdapterRegistry.defaultAdapter
	}

	// Fallback: return a nil-safe adapter that does nothing.
	// This should never happen in practice as LocalAdapter should be registered.
	return &noopAdapter{}
}

// SetDefaultAdapter sets the fallback adapter for paths without recognized schemes.
// This is typically the LocalAdapter for filesystem paths.
func SetDefaultAdapter(adapter ImportAdapter) {
	defer perf.Track(nil, "config.SetDefaultAdapter")()

	importAdapterRegistry.mu.Lock()
	defer importAdapterRegistry.mu.Unlock()

	importAdapterRegistry.defaultAdapter = adapter
}

// ResetImportAdapterRegistry clears all registered adapters.
// This is intended for testing only.
func ResetImportAdapterRegistry() {
	importAdapterRegistry.mu.Lock()
	defer importAdapterRegistry.mu.Unlock()

	importAdapterRegistry.adapters = make([]ImportAdapter, 0)
	importAdapterRegistry.defaultAdapter = nil
	// Reset the sync.Once to allow reinitialization.
	initAdaptersOnce = sync.Once{}
}

// GetRegisteredAdapters returns a copy of all registered adapters.
// This is intended for testing and debugging.
func GetRegisteredAdapters() []ImportAdapter {
	importAdapterRegistry.mu.RLock()
	defer importAdapterRegistry.mu.RUnlock()

	result := make([]ImportAdapter, len(importAdapterRegistry.adapters))
	copy(result, importAdapterRegistry.adapters)
	return result
}

// GetDefaultAdapter returns the current default adapter.
// This is intended for testing and debugging.
func GetDefaultAdapter() ImportAdapter {
	importAdapterRegistry.mu.RLock()
	defer importAdapterRegistry.mu.RUnlock()

	return importAdapterRegistry.defaultAdapter
}

// noopAdapter is a fallback adapter that does nothing.
// It's used when no default adapter is registered (should never happen in practice).
type noopAdapter struct{}

func (n *noopAdapter) Schemes() []string {
	return nil
}

func (n *noopAdapter) Resolve(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ int,
	_ int,
) ([]ResolvedPaths, error) {
	return nil, nil
}
