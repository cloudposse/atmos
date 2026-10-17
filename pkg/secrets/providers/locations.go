package providers

import (
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// FileLocations describes where the files of a file-backed provider definition may live when the
// exact file of a particular secret cannot be computed. Files are exact paths; Folders are
// directories under which any file may be one of the provider's files. Paths are relative to the
// working directory, like the paths the provider itself resolves.
type FileLocations struct {
	Files   []string
	Folders []string
}

// LocationsFunc reports every location a file-backed provider definition (its `spec` map) could
// place its files in, independent of any coordinate. A spec value that is itself a YAML-function
// selector (see IsSelector) is unknown, so the func must widen the result to cover anything that
// value could produce.
type LocationsFunc func(spec map[string]any) FileLocations

var (
	locationsMu sync.RWMutex
	locations   = make(map[string]LocationsFunc)
)

// RegisterLocations adds the file-location reporter of a file-backed backend track. It lets
// `describe affected` stay conservative for a secret whose backend selector cannot be resolved
// without credentials: the component is treated as affected by any change under these locations.
// Like Register, it is called from a provider's init and panics on a duplicate registration.
func RegisterLocations(track string, fn LocationsFunc) {
	defer perf.Track(nil, "providers.RegisterLocations")()

	locationsMu.Lock()
	defer locationsMu.Unlock()

	if _, exists := locations[track]; exists {
		panic(ErrProviderAlreadyRegistered)
	}
	locations[track] = fn
}

// PossibleLocations returns the locations a provider definition of the given track may use, and
// false when the track is not file-backed (store-backed secrets have no files).
func PossibleLocations(track string, spec map[string]any) (FileLocations, bool) {
	defer perf.Track(nil, "providers.PossibleLocations")()

	locationsMu.RLock()
	fn, ok := locations[track]
	locationsMu.RUnlock()
	if !ok {
		return FileLocations{}, false
	}
	return fn(spec), true
}

// IsSelector reports whether a string value is a YAML-function selector (such as
// `!aws.cloudformation.output ...`) that must be evaluated before it is concrete.
func IsSelector(value string) bool {
	defer perf.Track(nil, "providers.IsSelector")()

	return strings.HasPrefix(strings.TrimSpace(value), "!")
}
