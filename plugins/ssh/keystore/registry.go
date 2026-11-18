package keystore

import (
	"fmt"
	"sort"
	"sync"
)

// Factory builds a Store whose key material lives in dir. The caller decides where dir is.
type Factory func(dir string) (Store, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register adds a key store factory under kind. It panics on an empty kind, a nil factory, or a
// duplicate kind, because those are programming errors caught at start-up.
func Register(kind string, factory Factory) {
	if kind == "" || factory == nil {
		panic("keystore: Register needs a kind and a factory")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[kind]; dup {
		panic(fmt.Sprintf("keystore: kind %q registered twice", kind))
	}
	registry[kind] = factory
}

// New builds the store registered under kind, keeping its key material in dir.
func New(kind, dir string) (Store, error) {
	registryMu.RLock()
	factory, ok := registry[kind]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	return factory(dir)
}

// Kinds returns the registered kind names, sorted.
func Kinds() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	kinds := make([]string, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}
