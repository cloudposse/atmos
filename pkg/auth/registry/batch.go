package registry

import (
	"fmt"
	"maps"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Registration groups constructors that must become visible atomically.
type Registration struct {
	Providers  map[string]ProviderConfigConstructor
	Identities map[string]IdentityConfigConstructor
}

// RegisterBatch validates every name before changing the registry. The returned
// cleanup removes only this batch and can safely be called more than once.
func (f *Registry) RegisterBatch(batch Registration) (func(), error) {
	providers, identities := maps.Clone(batch.Providers), maps.Clone(batch.Identities)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := validateRegistrations(providers, f.providers, "provider"); err != nil {
		return nil, err
	}
	if err := validateRegistrations(identities, f.identities, "identity"); err != nil {
		return nil, err
	}
	if f.providers == nil {
		f.providers = make(map[string]ProviderConfigConstructor)
	}
	if f.identities == nil {
		f.identities = make(map[string]IdentityConfigConstructor)
	}
	maps.Copy(f.providers, providers)
	maps.Copy(f.identities, identities)
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			for kind := range providers {
				delete(f.providers, kind)
			}
			for kind := range identities {
				delete(f.identities, kind)
			}
		})
	}, nil
}

// RegisterBatch installs a package's constructors in the process-wide registry.
func RegisterBatch(batch Registration) (func(), error) { return defaultRegistry.RegisterBatch(batch) }

func validateRegistrations[T ProviderConfigConstructor | IdentityConfigConstructor](incoming, existing map[string]T, category string) error {
	for kind, constructor := range incoming {
		if kind == "" || constructor == nil {
			return errUtils.ErrInvalidAuthRegistration
		}
		if _, exists := existing[kind]; exists {
			return fmt.Errorf("%w: %s %s", errUtils.ErrDuplicateAuthRegistration, category, kind)
		}
	}
	return nil
}
