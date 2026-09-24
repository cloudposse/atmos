package deferred

import (
	"crypto/sha256"
	"encoding/json"
	"sync"

	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ValueCache holds successfully resolved values for one target and effective configuration.
// Its lifetime is limited to the authentication resolver's invocation.
type ValueCache struct {
	values sync.Map
}

// CacheFor returns an invocation-local cache for the target's raw configuration,
// including inherited auth. Callers must resolve authentication before loading values.
// Unsupported configuration values safely disable caching without affecting evaluation.
func CacheFor(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) *ValueCache {
	defer perf.Track(ac, "deferred.CacheFor")()

	if ac == nil {
		return nil
	}
	resolver, ok := ac.DeferredAuth.(*deferredAuthResolver)
	if !ok || info == nil {
		return nil
	}
	encoded, err := json.Marshal(struct {
		BasePath, ConfigPath, Stack, Component string
		Auth                                   schema.AuthConfig
		Section                                map[string]any
		Disabled                               bool
	}{ac.BasePath, ac.CliConfigPath, info.Stack, info.Component, ac.Auth, info.ComponentSection, resolver.disabled || info.AuthDisabled})
	if err != nil {
		return nil
	}
	cache, _ := resolver.values.LoadOrStore(sha256.Sum256(encoded), &ValueCache{})
	return cache.(*ValueCache)
}

// Load retrieves a resolved value; a nil cache means caching is disabled.
func (c *ValueCache) Load(kind string) (any, bool) {
	defer perf.Track(nil, "deferred.ValueCache.Load")()

	if c == nil {
		return nil, false
	}
	return c.values.Load(kind)
}

// Store retains only successful, fully resolved values, never display placeholders.
func (c *ValueCache) Store(kind string, value any) {
	defer perf.Track(nil, "deferred.ValueCache.Store")()

	if c != nil && !containsComputed(value) {
		c.values.Store(kind, value)
	}
}

func containsComputed(value any) bool {
	switch v := value.(type) {
	case degradation.AtmosComputedValue:
		return true
	case map[string]any:
		for _, child := range v {
			if containsComputed(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if containsComputed(child) {
				return true
			}
		}
	}
	return false
}
