// Package viperguard mutex-guards the process-wide global spf13/viper singleton.
//
// spf13/viper has no internal locking of its own: viper.Get*/Set/BindEnv all
// read or mutate one shared underlying map via deepSearch, and Go maps are not
// safe for any concurrent read/write access, regardless of which key each
// goroutine touches -- a BindEnv registering an unrelated key can still race
// with a concurrent Get of a completely different key. Any code that may run
// concurrently with other global-viper access (the DAG scheduler's per-node
// LoadConfig calls, the toolchain's concurrent batch installer, ...) must
// route through this package's functions instead of calling viper.* directly.
//
// This lives in its own leaf package (no imports of any other Atmos package)
// specifically so packages low in the dependency graph -- pkg/http and
// pkg/ui/theme, which pkg/config itself depends on -- can use it without an
// import cycle. pkg/config.GlobalViper() delegates here rather than keeping a
// second, independent mutex: two separate locks guarding the same underlying
// viper singleton would not actually exclude each other, leaving exactly the
// kind of cross-package race this package exists to close.
package viperguard

import (
	"slices"
	"sync"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

var mu sync.RWMutex

// Set sets key's value on the global Viper singleton.
func Set(key string, value any) {
	defer perf.Track(nil, "viperguard.Set")()

	mu.Lock()
	defer mu.Unlock()
	viper.GetViper().Set(key, value)
}

// BindEnv binds a Viper key to one or more environment variable names on the
// global Viper singleton. See viper.BindEnv for the input argument shape.
func BindEnv(input ...string) error {
	defer perf.Track(nil, "viperguard.BindEnv")()

	mu.Lock()
	defer mu.Unlock()
	return viper.BindEnv(input...)
}

// GetString returns key's value coerced to string. Returns "" if unset.
func GetString(key string) string {
	defer perf.Track(nil, "viperguard.GetString")()

	mu.RLock()
	defer mu.RUnlock()
	return viper.GetViper().GetString(key)
}

// GetBool returns key's value coerced to bool. Returns false if unset.
func GetBool(key string) bool {
	defer perf.Track(nil, "viperguard.GetBool")()

	mu.RLock()
	defer mu.RUnlock()
	return viper.GetViper().GetBool(key)
}

// GetStringSlice returns a clone of key's value coerced to []string: viper's
// own GetStringSlice can return its value's existing backing array rather
// than a copy, and handing that out under the lock would let a caller mutate
// shared Viper state after the lock is released. Returns nil if unset.
func GetStringSlice(key string) []string {
	defer perf.Track(nil, "viperguard.GetStringSlice")()

	mu.RLock()
	defer mu.RUnlock()
	return slices.Clone(viper.GetViper().GetStringSlice(key))
}

// IsSet reports whether key has a value from any source, including a
// registered default: viper.IsSet's underlying find() also checks
// viper.defaults, so a key registered only via SetDefault also reports true
// here. It cannot distinguish an explicit value (flag, env, config, override)
// from a default; callers needing that distinction need a separate check.
func IsSet(key string) bool {
	defer perf.Track(nil, "viperguard.IsSet")()

	mu.RLock()
	defer mu.RUnlock()
	return viper.GetViper().IsSet(key)
}

// ViperReader exposes only *viper.Viper's read methods. View passes this (not
// *viper.Viper) to its callback, so the callback cannot call a mutator like
// Set while holding only the read lock -- doing so would race against
// another concurrent View call's reads, or against Set's write lock,
// defeating the whole point of View. Extend with more read methods as
// callers need them; never add a mutator here.
type ViperReader interface {
	// IsSet reports whether key has a value from any source, including a
	// registered default (see the package-level IsSet's doc comment for why).
	IsSet(key string) bool
	// GetBool returns key's value coerced to bool. Returns false if unset.
	GetBool(key string) bool
	// GetString returns key's value coerced to string. Returns "" if unset.
	GetString(key string) string
	// GetStringSlice returns key's value coerced to []string, cloned so the
	// caller cannot mutate Viper's own backing array. Returns nil if unset.
	GetStringSlice(key string) []string
}

// viperReaderAdapter wraps *viper.Viper to satisfy ViperReader without
// exposing the concrete *viper.Viper type to View callbacks. Passing
// *viper.Viper itself through the ViperReader interface would only hide Set
// behind a narrower static type -- Go interfaces retain their dynamic type,
// so a callback could still type-assert the value back to *viper.Viper and
// call Set while holding only View's read lock. Because viperReaderAdapter is
// unexported, code outside this package cannot name it to assert against it,
// so it cannot recover the underlying *viper.Viper this way.
type viperReaderAdapter struct {
	v *viper.Viper
}

func (a viperReaderAdapter) IsSet(key string) bool {
	defer perf.Track(nil, "viperguard.viperReaderAdapter.IsSet")()

	return a.v.IsSet(key)
}

func (a viperReaderAdapter) GetBool(key string) bool {
	defer perf.Track(nil, "viperguard.viperReaderAdapter.GetBool")()

	return a.v.GetBool(key)
}

func (a viperReaderAdapter) GetString(key string) string {
	defer perf.Track(nil, "viperguard.viperReaderAdapter.GetString")()

	return a.v.GetString(key)
}

func (a viperReaderAdapter) GetStringSlice(key string) []string {
	defer perf.Track(nil, "viperguard.viperReaderAdapter.GetStringSlice")()

	return slices.Clone(a.v.GetStringSlice(key))
}

// View executes fn with a read lock held on the global Viper singleton,
// giving fn a consistent snapshot for the whole call. Use this instead of
// separate Get*/IsSet calls whenever a decision combines more than one read
// (e.g. an IsSet presence check followed by a GetBool value read): each
// individual function in this package locks and unlocks independently, so a
// concurrent Set() between two separate calls could let the decision combine
// one snapshot's presence result with a different snapshot's value.
//
// The callback fn must not call Set, BindEnv, or any other guard writer,
// directly or transitively: mu.RLock is held for the whole call, and those
// writers block on mu.Lock until fn returns, so fn calling one deadlocks
// against itself.
func View(fn func(v ViperReader)) {
	defer perf.Track(nil, "viperguard.View")()

	mu.RLock()
	defer mu.RUnlock()
	fn(viperReaderAdapter{v: viper.GetViper()})
}
