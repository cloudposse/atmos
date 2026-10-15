package deferred

import (
	"reflect"
	"sync"

	"github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValueCache holds successfully resolved values for one target and effective configuration.
// Its owner determines the lifetime and supplies the configuration-specific key.
type ValueCache struct {
	values sync.Map
}

// Load retrieves a resolved value; a nil cache means caching is disabled.
func (c *ValueCache) Load(kind string) (any, bool) {
	defer perf.Track(nil, "deferred.ValueCache.Load")()

	if c == nil {
		return nil, false
	}
	return c.values.Load(kind)
}

// Store rejects unresolved computed values. Callers must partition inspection
// placeholders from execution values using separate kinds.
func (c *ValueCache) Store(kind string, value any) {
	defer perf.Track(nil, "deferred.ValueCache.Store")()

	if c != nil && !containsComputed(value) {
		c.values.Store(kind, value)
	}
}

// IsCacheable reports whether a successful lookup contains a reusable value.
// Computed placeholders and nil results must remain retryable.
//
//nolint:lintroller // Pure value validation; no I/O or lookup work to measure.
func IsCacheable(value any) bool {
	if value == nil {
		return false
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return false
		}
	}
	return !containsComputed(value)
}

// LoadOrStore binds a stable value to this target's effective configuration.
// A nil cache or an unresolved value remains uncached.
func (c *ValueCache) LoadOrStore(kind string, value any) any {
	defer perf.Track(nil, "deferred.ValueCache.LoadOrStore")()

	if c == nil || !IsCacheable(value) {
		return value
	}
	actual, _ := c.values.LoadOrStore(kind, value)
	return actual
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
