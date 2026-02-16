package ci

import (
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	providersMu sync.RWMutex
	providers   = make(map[string]Provider)
)

// Register registers a CI provider.
// Providers should call this in their init() function.
func Register(p Provider) {
	defer perf.Track(nil, "ci.Register")()

	providersMu.Lock()
	defer providersMu.Unlock()
	providers[p.Name()] = p
}

// Get returns a provider by name.
func Get(name string) (Provider, error) {
	defer perf.Track(nil, "ci.Get")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	p, ok := providers[name]
	if !ok {
		return nil, errUtils.ErrCIProviderNotFound
	}
	return p, nil
}

// Detect returns a provider that detects it is active in the current environment.
func Detect() Provider {
	defer perf.Track(nil, "ci.Detect")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	for _, p := range providers {
		if p.Detect() {
			return p
		}
	}
	return nil
}

// DetectOrError returns the detected provider or an error if none is detected.
func DetectOrError() (Provider, error) {
	defer perf.Track(nil, "ci.DetectOrError")()

	p := Detect()
	if p == nil {
		return nil, errUtils.ErrCIProviderNotDetected
	}
	return p, nil
}

// List returns all registered provider names.
func List() []string {
	defer perf.Track(nil, "ci.List")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}

// IsCI returns true if any CI provider is detected.
func IsCI() bool {
	defer perf.Track(nil, "ci.IsCI")()

	return Detect() != nil
}

// testSaveAndClearRegistry clears the provider registry and returns the previous
// map. For use in tests only. Restore with testRestoreRegistry.
func testSaveAndClearRegistry() map[string]Provider {
	providersMu.Lock()
	defer providersMu.Unlock()
	prev := providers
	providers = make(map[string]Provider)
	return prev
}

// testRestoreRegistry restores the provider registry from a previous snapshot.
func testRestoreRegistry(m map[string]Provider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers = m
}
