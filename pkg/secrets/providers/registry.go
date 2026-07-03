package providers

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Backend-track keys. Each concrete provider self-registers under its track via
// init(); pkg/secrets maps a declaration's BackendType onto these.
const (
	// TrackStore is the store-backed track (a `secret: true` store).
	TrackStore = "store"
	// TrackSops is the SOPS file-backed track.
	TrackSops = "sops"
)

// Registry-construction errors.
var (
	// ErrProviderAlreadyRegistered indicates two providers registered the same track.
	ErrProviderAlreadyRegistered = errors.New("secrets provider track already registered")
	// ErrTrackNotRegistered indicates no provider is registered for a backend track.
	ErrTrackNotRegistered = errors.New("no secrets provider registered for backend track")
)

// Constructor builds a Provider for a backend track. The name argument is the
// declaration's BackendName (a store name for track 1, a SOPS provider name for
// track 2). The sectionProviders argument carries a stack/component
// `secrets.providers` map (used by file-based tracks like SOPS; ignored by
// store-backed tracks).
type Constructor func(atmosConfig *schema.AtmosConfiguration, name string, sectionProviders map[string]any) (Provider, error)

var (
	registryMu   sync.RWMutex
	constructors = make(map[string]Constructor)
)

// Register adds a backend-track constructor. Providers self-register from this
// package's files via init(), so dispatch never requires a central switch.
// It panics on duplicate registration, which is a programming error.
func Register(track string, c Constructor) {
	defer perf.Track(nil, "providers.Register")()

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := constructors[track]; exists {
		panic(fmt.Errorf("%w: %q", ErrProviderAlreadyRegistered, track))
	}
	constructors[track] = c
}

// New constructs the provider registered for track, returning
// ErrTrackNotRegistered if none is registered.
func New(atmosConfig *schema.AtmosConfiguration, track, name string, sectionProviders map[string]any) (Provider, error) {
	defer perf.Track(atmosConfig, "providers.New")()

	registryMu.RLock()
	c, ok := constructors[track]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrTrackNotRegistered, track)
	}
	return c(atmosConfig, name, sectionProviders)
}

// Registered returns the registered backend tracks, sorted, for diagnostics and tests.
func Registered() []string {
	defer perf.Track(nil, "providers.Registered")()

	registryMu.RLock()
	defer registryMu.RUnlock()

	tracks := make([]string, 0, len(constructors))
	for t := range constructors {
		tracks = append(tracks, t)
	}
	sort.Strings(tracks)
	return tracks
}
