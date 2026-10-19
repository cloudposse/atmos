package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/terraform/refcache"
)

func TestInvalidateComponentOutputs(t *testing.T) {
	for name, outputs := range map[string]map[string]any{
		"before first apply": {},
		"before update":      {"vpc_id": "old-vpc"},
	} {
		t.Run(name, func(t *testing.T) {
			ResetOutputsCache()
			t.Cleanup(ResetOutputsCache)

			key := refcache.NewKey(nil, "us-east-1-dev", "vpc")
			terraformOutputsCache.Store(key, outputs)
			// Include names that would collide if invalidation joined them with a hyphen.
			otherKeys := []refcache.Key{
				refcache.NewKey(nil, "us-east-1", "dev-vpc"),
				refcache.NewKey(nil, "us-east-1-dev", "other"),
				refcache.NewKey(nil, "other", "vpc"),
			}
			for _, otherKey := range otherKeys {
				terraformOutputsCache.Store(otherKey, map[string]any{"vpc_id": "unchanged"})
			}

			InvalidateComponentOutputs("us-east-1-dev", "vpc")
			_, found := terraformOutputsCache.Load(key)
			assert.False(t, found, "both empty and populated snapshots must be evicted")
			for _, otherKey := range otherKeys {
				cached, found := terraformOutputsCache.Load(otherKey)
				require.True(t, found, "unrelated component outputs must remain cached")
				assert.Equal(t, map[string]any{"vpc_id": "unchanged"}, cached)
			}

			// Invalidating an already absent entry is safe.
			InvalidateComponentOutputs("us-east-1-dev", "vpc")
		})
	}
}
