package ci

import (
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	providersMu sync.RWMutex
	providers   = make(map[string]provider.Provider)
)

// Register registers a CI provider.
// Providers should call this in their init() function.
func Register(p provider.Provider) {
	defer perf.Track(nil, "provider.Register")()

	providersMu.Lock()
	defer providersMu.Unlock()
	providers[p.Name()] = p
}

// Get returns a provider by name.
func Get(name string) (provider.Provider, error) {
	defer perf.Track(nil, "provider.Get")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	p, ok := providers[name]
	if !ok {
		return nil, errUtils.ErrCIProviderNotFound
	}
	return p, nil
}

// Detect returns a provider that detects it is active in the current environment.
func Detect() provider.Provider {
	defer perf.Track(nil, "provider.Detect")()

	providersMu.RLock()
	defer providersMu.RUnlock()

	for _, p := range providers {
		if p.Detect() {
			log.Debug("CI provider detected", "provider", p.Name())
			return p
		} else {
			log.Debug("CI provider not detected", "provider", p.Name())
		}
	}
	return nil
}

// DetectOrError returns the detected provider or an error if none is detected.
func DetectOrError() (provider.Provider, error) {
	defer perf.Track(nil, "provider.DetectOrError")()

	p := Detect()
	if p == nil {
		return nil, errUtils.ErrCIProviderNotDetected
	}
	return p, nil
}

// List returns all registered provider names.
func List() []string {
	defer perf.Track(nil, "provider.List")()

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
	defer perf.Track(nil, "provider.IsCI")()

	return Detect() != nil
}
