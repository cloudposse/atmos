package deferred

import (
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
