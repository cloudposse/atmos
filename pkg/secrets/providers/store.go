package providers

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// init self-registers the store-backed track (track 1) so backend selection is a
// registry lookup rather than a central switch. The store track ignores the
// stack/component `secrets.providers` map.
func init() {
	Register(TrackStore, func(atmosConfig *schema.AtmosConfiguration, name string, _ map[string]any) (Provider, error) {
		return newStoreProvider(atmosConfig, name)
	})
}

// storeProvider adapts a `secret: true` store (track 1) to the Provider interface.
type storeProvider struct {
	name  string
	store store.Store
	kind  string
}

// newStoreProvider builds a store-backed provider, validating that the named store exists and
// is marked `secret: true`.
func newStoreProvider(atmosConfig *schema.AtmosConfiguration, name string) (Provider, error) {
	defer perf.Track(atmosConfig, "providers.newStoreProvider")()

	cfg, ok := atmosConfig.StoresConfig[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrStoreNotFound, name)
	}
	if !cfg.Secret {
		return nil, fmt.Errorf("%w: %q", ErrStoreNotSecret, name)
	}
	s, ok := atmosConfig.Stores[name]
	if !ok || s == nil {
		return nil, fmt.Errorf("%w: %q (registry)", ErrStoreNotFound, name)
	}

	kind := cfg.Kind
	if kind == "" {
		kind = cfg.Type
	}

	return &storeProvider{name: name, store: s, kind: kind}, nil
}

func (p *storeProvider) Kind() string {
	defer perf.Track(nil, "providers.storeProvider.Kind")()

	return p.kind
}

func (p *storeProvider) Set(coord Coordinate, value any) error {
	defer perf.Track(nil, "providers.storeProvider.Set")()

	return p.store.Set(coord.Stack, coord.Component, coord.Key, value)
}

func (p *storeProvider) Get(coord Coordinate) (any, error) {
	defer perf.Track(nil, "providers.storeProvider.Get")()

	return p.store.Get(coord.Stack, coord.Component, coord.Key)
}

func (p *storeProvider) Delete(coord Coordinate) error {
	defer perf.Track(nil, "providers.storeProvider.Delete")()

	ds, ok := p.store.(store.DeletableStore)
	if !ok {
		return fmt.Errorf("%w: store %q (%s)", ErrDeleteNotSupported, p.name, p.kind)
	}
	return ds.Delete(coord.Stack, coord.Component, coord.Key)
}

func (p *storeProvider) Status(coord Coordinate) (bool, error) {
	defer perf.Track(nil, "providers.storeProvider.Status")()

	if ss, ok := p.store.(store.StatusStore); ok {
		return ss.Has(coord.Stack, coord.Component, coord.Key)
	}
	// Fallback: a successful Get implies existence. Note this retrieves (and thus may
	// register) the value; callers that must avoid retrieval should prefer StatusStore.
	// A Get error is treated as "not initialized" for status purposes.
	if _, err := p.store.Get(coord.Stack, coord.Component, coord.Key); err != nil {
		return false, nil
	}
	return true, nil
}
