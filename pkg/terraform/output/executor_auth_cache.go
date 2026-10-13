package output

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/schema"
)

var uncacheableAuthSequence atomic.Uint64

type scopedOutputKey struct {
	component  string
	basePath   string
	configPath string
	invocation any
	auth       any
	maskOnly   bool
}

// scopedOutputCacheKey keeps inspection placeholders separate from execution
// values and partitions outputs by configuration paths and invocation manager.
//
//nolint:revive // argument-limit: matches the executor cache inputs plus inspection mode.
func scopedOutputCacheKey(ac *schema.AtmosConfiguration, stack, component string, authContext *schema.AuthContext, authManager any, maskOnly bool) scopedOutputKey {
	key := scopedOutputKey{component: stackComponentKey(stack, component), auth: outputCacheKey(stack, component, authContext, authManager), maskOnly: maskOnly}
	if ac != nil {
		key.basePath, key.configPath = ac.BasePath, ac.CliConfigPath
		key.invocation = ac.AuthManager
		if key.invocation != nil && !reflect.ValueOf(key.invocation).Comparable() {
			key.invocation = uncacheableAuthSequence.Add(1)
		}
	}
	return key
}

type authenticatedOutputKey struct {
	component string
	manager   any
	context   [sha256.Size]byte
	unique    uint64
}

// outputCacheKey partitions outputs by the authentication actually supplied to the
// executor. A target component can use a different identity from its caller, and
// identity names alone are not unique across independently configured managers.
func outputCacheKey(stack, component string, authContext *schema.AuthContext, authManager any) any {
	base := stackComponentKey(stack, component)
	if authContext == nil && authManager == nil {
		return base
	}

	scope := struct {
		Context        *schema.AuthContext
		ManagerContext *schema.AuthContext
		Identity       string
		Disabled       bool
	}{Context: authContext}
	if authManager != nil && !reflect.ValueOf(authManager).Comparable() {
		return authenticatedOutputKey{component: base, unique: uncacheableAuthSequence.Add(1)}
	}
	if manager, ok := authManager.(auth.AuthManager); ok {
		if info := manager.GetStackInfo(); info != nil {
			scope.Identity = info.Identity
			scope.Disabled = info.AuthDisabled
			scope.ManagerContext = info.AuthContext
		}
	}
	encoded, err := json.Marshal(scope)
	if err != nil {
		// An unencodable context must never fall back to another identity's key.
		return authenticatedOutputKey{component: base, unique: uncacheableAuthSequence.Add(1)}
	}
	// Contexts can contain credentials; retain only a digest, never plaintext.
	// Retain the manager itself, not a formatted pointer: addresses can be reused
	// after collection, which would make an old cache entry reachable by a new manager.
	return authenticatedOutputKey{component: base, manager: authManager, context: sha256.Sum256(encoded)}
}
