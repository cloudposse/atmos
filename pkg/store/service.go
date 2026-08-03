package store

import (
	"fmt"
	"sort"
)

// Descriptor summarizes a configured store backend for `atmos store list`: its name, backend
// kind, secret-subsystem membership, and which optional capabilities (see store.go) its live
// instance implements.
type Descriptor struct {
	Name      string
	Kind      string
	Secret    bool
	Deletable bool
	HasStatus bool
	Local     bool
	Listable  bool
}

// Service is a thin CRUD facade over a StoreRegistry, used by the `atmos store` command family
// to read, write, delete, and enumerate raw values in any configured store by name. Unlike the
// secrets subsystem (pkg/secrets), there is no declaration or scope layer here: callers name a
// configured store directly and this reads/writes/deletes against it.
type Service struct {
	config   StoresConfig
	registry StoreRegistry
}

// NewService builds a Service over the given configured stores and their live registry
// (typically atmosConfig.StoresConfig and atmosConfig.Stores).
func NewService(config StoresConfig, registry StoreRegistry) *Service {
	return &Service{config: config, registry: registry}
}

// lookup returns the named store's live instance, or ErrStoreNotConfigured if no store with
// that name is configured.
func (s *Service) lookup(name string) (Store, error) {
	st, ok := s.registry[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrStoreNotConfigured, name)
	}
	return st, nil
}

// Set writes a value to the named store.
func (s *Service) Set(name, stack, component, key string, value any) error {
	st, err := s.lookup(name)
	if err != nil {
		return err
	}
	return st.Set(stack, component, key, value)
}

// Get retrieves a value from the named store, scoped to a stack and component. Note: unlike
// secrets.Service.Get, this does not register the value with the masker -- pkg/store sits below
// pkg/io in the import graph (pkg/schema, which pkg/store's config types are part of, is imported
// by pkg/io), so registering here would create an import cycle. Callers that surface a Get result
// to a human (e.g. `atmos store get`, see cmd/store/get.go) are responsible for registering the
// value with io.RegisterSecretValue themselves before writing it out.
func (s *Service) Get(name, stack, component, key string) (any, error) {
	st, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	return st.Get(stack, component, key)
}

// GetKey retrieves a value directly by key, without stack or component scoping. See the Get
// doc comment for why masker registration is not done here.
func (s *Service) GetKey(name, key string) (any, error) {
	st, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	return st.GetKey(key)
}

// Delete removes a value from the named store. It returns ErrDeleteNotSupported if the store's
// backend does not implement DeletableStore.
func (s *Service) Delete(name, stack, component, key string) error {
	st, err := s.lookup(name)
	if err != nil {
		return err
	}
	ds, ok := st.(DeletableStore)
	if !ok {
		return fmt.Errorf("%w: store %q (kind %s)", ErrDeleteNotSupported, name, resolveKind(s.config[name]))
	}
	return ds.Delete(stack, component, key)
}

// Keys lists the keys under a stack/component scope (or globally when both are empty) in the
// named store. It returns ErrListNotSupported if the store's backend does not implement
// ListableStore.
func (s *Service) Keys(name, stack, component string) ([]string, error) {
	st, err := s.lookup(name)
	if err != nil {
		return nil, err
	}
	ls, ok := st.(ListableStore)
	if !ok {
		return nil, fmt.Errorf("%w: store %q (kind %s)", ErrListNotSupported, name, resolveKind(s.config[name]))
	}
	return ls.Keys(stack, component)
}

// List returns a Descriptor for every configured store, sorted by name.
func (s *Service) List() []Descriptor {
	names := make([]string, 0, len(s.config))
	for name := range s.config {
		names = append(names, name)
	}
	sort.Strings(names)

	descriptors := make([]Descriptor, 0, len(names))
	for _, name := range names {
		cfg := s.config[name]
		d := Descriptor{
			Name:   name,
			Kind:   resolveKind(cfg),
			Secret: cfg.Secret,
		}
		if st, ok := s.registry[name]; ok {
			_, d.Deletable = st.(DeletableStore)
			_, d.HasStatus = st.(StatusStore)
			_, d.Listable = st.(ListableStore)
			if ls, ok := st.(LocalStore); ok {
				d.Local = ls.IsLocal()
			}
		}
		descriptors = append(descriptors, d)
	}
	return descriptors
}
