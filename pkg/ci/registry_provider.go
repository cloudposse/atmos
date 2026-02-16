package ci

import (
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

// Register registers a CI provider.
// Providers should call this in their init() function.
func Register(p Provider) {
	provider.Register(p)
}

// Get returns a provider by name.
func Get(name string) (Provider, error) {
	return provider.Get(name)
}

// Detect returns a provider that detects it is active in the current environment.
func Detect() Provider {
	return provider.Detect()
}

// DetectOrError returns the detected provider or an error if none is detected.
func DetectOrError() (Provider, error) {
	return provider.DetectOrError()
}

// List returns all registered provider names.
func List() []string {
	return provider.List()
}

// IsCI returns true if any CI provider is detected.
func IsCI() bool {
	return provider.IsCI()
}
