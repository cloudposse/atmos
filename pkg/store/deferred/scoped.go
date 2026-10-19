package deferred

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// ScopedStores gives secret providers lookup-local store handles. Every requested
// operation binds credentials and reads under one lock, including SOPS age keys.
// Unused stores remain unauthenticated, and the shared registry is never changed.
func ScopedStores(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) store.StoreRegistry {
	defer perf.Track(ac, "store.deferred.ScopedStores")()
	registry := make(store.StoreRegistry, len(ac.Stores))
	for name, backend := range ac.Stores {
		registry[name] = backend
		if _, ok := backend.(store.IdentityAwareStore); !ok {
			continue
		}
		scoped := &scopedStore{config: ac, info: info, name: name, backend: backend}
		registry[name] = scoped
		if raw, ok := backend.(store.RawStore); ok {
			registry[name] = &rawScopedStore{scopedStore: scoped, raw: raw}
		}
	}
	return registry
}

type scopedStore struct {
	config  *schema.AtmosConfiguration
	info    *schema.ConfigAndStacksInfo
	name    string
	backend store.Store
}

func (s *scopedStore) Get(stack, component, key string) (any, error) {
	return WithStoreAuth(s.config, s.info, s.name, func() (any, error) {
		return s.backend.Get(stack, component, key)
	})
}

func (s *scopedStore) GetKey(key string) (any, error) {
	return WithStoreAuth(s.config, s.info, s.name, func() (any, error) {
		return s.backend.GetKey(key)
	})
}

func (s *scopedStore) Set(stack, component, key string, value any) error {
	_, err := WithStoreAuth(s.config, s.info, s.name, func() (any, error) {
		return nil, s.backend.Set(stack, component, key, value)
	})
	return err
}

type rawScopedStore struct {
	*scopedStore
	raw store.RawStore
}

func (s *rawScopedStore) GetRaw(stack, component, key string) (string, error) {
	value, err := WithStoreAuth(s.config, s.info, s.name, func() (any, error) {
		return s.raw.GetRaw(stack, component, key)
	})
	if err != nil {
		return "", err
	}
	return value.(string), nil
}
