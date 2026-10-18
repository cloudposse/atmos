// Package refcache defines the key shared by the process-wide caches behind
// Terraform references: !terraform.state, !terraform.output, and atmos.Component.
package refcache

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// uncacheableSequence yields values that never compare equal, so a lookup that cannot
// be keyed safely is never shared.
var uncacheableSequence atomic.Uint64

// Key identifies one Terraform reference lookup in a process-wide cache.
// Every field changes the cached result, and the key is always comparable.
type Key struct {
	BasePath, ConfigPath string // Configuration root (HEAD vs BASE in describe affected).
	Stack, Component     string // Target.
	Invocation           any    // ac.AuthManager; separates deferred invocations.
	Auth                 any    // Caller credentials, from AuthScope.
	Inspection           bool   // Masked placeholders vs resolved values.
	RawDescribe          bool   // Target described without YAML functions.
}

// NewKey returns the key for a target, filled with the configuration root and invocation.
// Callers set Auth, Inspection, and RawDescribe on the result as needed.
//
//nolint:lintroller // Simple factory; no I/O or lookup work to measure.
func NewKey(ac *schema.AtmosConfiguration, stack, component string) Key {
	key := Key{Stack: stack, Component: component}
	if ac == nil {
		return key
	}
	key.BasePath, key.ConfigPath = ac.BasePath, ac.CliConfigPath
	key.Invocation = ac.AuthManager
	if key.Invocation != nil && !reflect.ValueOf(key.Invocation).Comparable() {
		key.Invocation = uncacheableSequence.Add(1)
	}
	return key
}

// authScope is the comparable form of the credentials supplied by a caller.
type authScope struct {
	manager any
	digest  [sha256.Size]byte
	unique  uint64
}

// AuthScope partitions cached values by the authentication actually supplied to a lookup.
// A target component can use a different identity from its caller, and identity names alone
// are not unique across independently configured managers. It returns nil when the caller
// supplies no credentials and authentication is not disabled.
func AuthScope(authContext *schema.AuthContext, authManager any, disabled bool) any {
	defer perf.Track(nil, "refcache.AuthScope")()

	if authContext == nil && authManager == nil && !disabled {
		return nil
	}
	if authManager != nil && !reflect.ValueOf(authManager).Comparable() {
		return authScope{unique: uncacheableSequence.Add(1)}
	}

	scope := struct {
		Context        *schema.AuthContext
		ManagerContext *schema.AuthContext
		Identity       string
		Disabled       bool
	}{Context: authContext, Disabled: disabled}
	if manager, ok := authManager.(auth.AuthManager); ok {
		if info := manager.GetStackInfo(); info != nil {
			scope.Identity = info.Identity
			scope.Disabled = scope.Disabled || info.AuthDisabled
			scope.ManagerContext = info.AuthContext
		}
	}
	encoded, err := json.Marshal(scope)
	if err != nil {
		// An unencodable context must never fall back to another identity's key.
		return authScope{unique: uncacheableSequence.Add(1)}
	}
	// Contexts can contain credentials; retain only a digest, never plaintext.
	// Retain the manager itself, not a formatted pointer: addresses can be reused
	// after collection, which would make an old cache entry reachable by a new manager.
	return authScope{manager: authManager, digest: sha256.Sum256(encoded)}
}

// DeleteTarget removes every cached variant of one target from a cache keyed by Key,
// whatever its credentials, inspection mode, or describe mode.
func DeleteTarget(cache *sync.Map, stack, component string) {
	defer perf.Track(nil, "refcache.DeleteTarget")()

	cache.Range(func(k, _ any) bool {
		if key, ok := k.(Key); ok && key.Stack == stack && key.Component == component {
			cache.Delete(k)
		}
		return true
	})
}

// Kinds names the invocation-local value-cache entries used by one mode.
type Kinds struct {
	Component, State, StaticState string
}

const (
	componentKind   = "atmos.Component"
	stateKind       = "terraform.state"
	staticStateKind = "terraform.state.static"

	inspectionComponentKind   = "inspect:atmos.Component"
	inspectionStateKind       = "inspect:terraform.state"
	inspectionStaticStateKind = "inspect:terraform.state.static"
)

var (
	executionKinds  = Kinds{Component: componentKind, State: stateKind, StaticState: staticStateKind}
	inspectionKinds = Kinds{Component: inspectionComponentKind, State: inspectionStateKind, StaticState: inspectionStaticStateKind}
)

// KindsFor returns the value-cache kinds for inspection (masked placeholders) or
// execution (resolved values). The two never share a kind.
//
//nolint:lintroller // Simple lookup; no I/O or work to measure.
func KindsFor(inspection bool) Kinds {
	if inspection {
		return inspectionKinds
	}
	return executionKinds
}
