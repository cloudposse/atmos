// Package deferred binds deferred evaluation to stack configuration.
package deferred

import (
	"crypto/sha256"
	"encoding/json"
	"sync"

	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Protect lazy initialization on a shared configuration; cached values themselves
// remain invocation-local and use their own concurrent maps.
var evaluationContextMu sync.Mutex

func evaluationContext(ac *schema.AtmosConfiguration) *schema.DeferredEvaluationContext {
	evaluationContextMu.Lock()
	defer evaluationContextMu.Unlock()
	if ac.DeferredEvaluation == nil || ac.DeferredEvaluation.Manager != ac.AuthManager {
		ac.DeferredEvaluation = &schema.DeferredEvaluationContext{Manager: ac.AuthManager}
	}
	return ac.DeferredEvaluation
}

// CacheFor returns an invocation-local cache for the target's raw configuration,
// including inherited auth. Callers must resolve authentication before loading values.
// Unsupported configuration values safely disable caching without affecting evaluation.
func CacheFor(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) *deferred.ValueCache {
	defer perf.Track(ac, "stack.deferred.CacheFor")()

	if ac == nil {
		return nil
	}
	if !authdeferred.IsDeferred(ac.AuthManager) || info == nil {
		return nil
	}
	encoded, err := json.Marshal(struct {
		BasePath, ConfigPath, Stack, Component string
		Auth                                   schema.AuthConfig
		Section                                map[string]any
		Disabled                               bool
	}{ac.BasePath, ac.CliConfigPath, info.Stack, info.Component, ac.Auth, info.ComponentSection, authdeferred.AuthDisabled(ac.AuthManager) || info.AuthDisabled})
	if err != nil {
		return nil
	}
	cache, _ := evaluationContext(ac).Values.LoadOrStore(sha256.Sum256(encoded), &deferred.ValueCache{})
	return cache.(*deferred.ValueCache)
}

// StoreValue memoizes successful template store reads for one invocation. In-flight
// reads of the same key share a result, while nil values and errors remain retryable.
func StoreValue(ac *schema.AtmosConfiguration, key string, lookup func() (any, error)) (any, error) {
	defer perf.Track(ac, "stack.deferred.StoreValue")()

	if ac == nil || !authdeferred.IsDeferred(ac.AuthManager) {
		return lookup()
	}
	context := evaluationContext(ac)
	if value, ok := context.StoreValues.Load(key); ok {
		return value, nil
	}
	value, err, _ := context.StoreFlights.Do(key, func() (any, error) {
		if cached, ok := context.StoreValues.Load(key); ok {
			return cached, nil
		}
		resolved, resolveErr := lookup()
		if resolveErr == nil && resolved != nil {
			context.StoreValues.Store(key, resolved)
		}
		return resolved, resolveErr
	})
	return value, err
}
