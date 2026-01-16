package loader

import (
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// errFmtWithName is a format string for errors with a name.
const errFmtWithName = "%w: %s"

// Registry manages registered loaders and provides lookup by extension.
type Registry struct {
	mu         sync.RWMutex
	loaders    map[string]StackLoader // Name -> Loader.
	extensions map[string]string      // Extension -> Loader name.
}

// NewRegistry creates a new empty loader registry.
func NewRegistry() *Registry {
	defer perf.Track(nil, "loader.NewRegistry")()

	return &Registry{
		loaders:    make(map[string]StackLoader),
		extensions: make(map[string]string),
	}
}

// Register adds a loader to the registry.
// Returns an error if a loader with the same name or extension already exists.
func (r *Registry) Register(loader StackLoader) error {
	defer perf.Track(nil, "loader.Registry.Register")()

	// Validate loader is not nil.
	if loader == nil {
		return fmt.Errorf("%w: loader cannot be nil", errUtils.ErrInvalidArguments)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	name := loader.Name()

	// Validate loader has a non-empty name.
	if name == "" {
		return fmt.Errorf("%w: loader name cannot be empty", errUtils.ErrInvalidArguments)
	}

	// Validate loader has at least one extension.
	if len(loader.Extensions()) == 0 {
		return fmt.Errorf("%w: loader must support at least one extension", errUtils.ErrInvalidArguments)
	}

	// Check if the name is already registered.
	if _, exists := r.loaders[name]; exists {
		return fmt.Errorf(errFmtWithName, errUtils.ErrDuplicateLoader, name)
	}

	// Check if any extensions conflict.
	for _, ext := range loader.Extensions() {
		normalizedExt := normalizeExtension(ext)
		if existingName, exists := r.extensions[normalizedExt]; exists {
			return fmt.Errorf("%w: extension %s already registered for %s", errUtils.ErrDuplicateLoader, ext, existingName)
		}
	}

	// Register the loader.
	r.loaders[name] = loader

	// Register all extensions.
	for _, ext := range loader.Extensions() {
		r.extensions[normalizeExtension(ext)] = name
	}

	return nil
}

// GetByExtension retrieves a loader by file extension.
// The extension can be with or without the leading dot.
func (r *Registry) GetByExtension(ext string) (StackLoader, error) {
	defer perf.Track(nil, "loader.Registry.GetByExtension")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	normalizedExt := normalizeExtension(ext)
	loaderName, exists := r.extensions[normalizedExt]
	if !exists {
		return nil, fmt.Errorf(errFmtWithName, errUtils.ErrLoaderNotFound, ext)
	}

	loader, exists := r.loaders[loaderName]
	if !exists {
		return nil, fmt.Errorf("%w: %s (registered but not found)", errUtils.ErrLoaderNotFound, ext)
	}

	return loader, nil
}

// GetByName retrieves a loader by its name.
func (r *Registry) GetByName(name string) (StackLoader, error) {
	defer perf.Track(nil, "loader.Registry.GetByName")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	loader, exists := r.loaders[name]
	if !exists {
		return nil, fmt.Errorf(errFmtWithName, errUtils.ErrLoaderNotFound, name)
	}

	return loader, nil
}

// HasExtension returns true if a loader for the given extension exists.
func (r *Registry) HasExtension(ext string) bool {
	defer perf.Track(nil, "loader.Registry.HasExtension")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.extensions[normalizeExtension(ext)]
	return exists
}

// List returns all registered loader names.
func (r *Registry) List() []string {
	defer perf.Track(nil, "loader.Registry.List")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.loaders))
	for name := range r.loaders {
		names = append(names, name)
	}
	return names
}

// Extensions returns all registered extensions.
func (r *Registry) Extensions() []string {
	defer perf.Track(nil, "loader.Registry.Extensions")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	exts := make([]string, 0, len(r.extensions))
	for ext := range r.extensions {
		exts = append(exts, ext)
	}
	return exts
}

// Len returns the number of registered loaders.
func (r *Registry) Len() int {
	defer perf.Track(nil, "loader.Registry.Len")()

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.loaders)
}

// Unregister removes a loader from the registry by name.
// Also removes all associated extensions.
func (r *Registry) Unregister(name string) error {
	defer perf.Track(nil, "loader.Registry.Unregister")()

	r.mu.Lock()
	defer r.mu.Unlock()

	loader, exists := r.loaders[name]
	if !exists {
		return fmt.Errorf(errFmtWithName, errUtils.ErrLoaderNotFound, name)
	}

	// Remove extensions first.
	for _, ext := range loader.Extensions() {
		delete(r.extensions, normalizeExtension(ext))
	}

	// Remove the loader.
	delete(r.loaders, name)

	return nil
}

// Clear removes all loaders from the registry.
func (r *Registry) Clear() {
	defer perf.Track(nil, "loader.Registry.Clear")()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.loaders = make(map[string]StackLoader)
	r.extensions = make(map[string]string)
}

// normalizeExtension ensures the extension starts with a dot and is lowercase.
// Returns empty string for empty input to avoid returning just ".".
func normalizeExtension(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}
