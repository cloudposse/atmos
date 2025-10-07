package exec

import (
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestProcessTagStore(t *testing.T) {
	// Start a new Redis server
	s := miniredis.RunT(t)
	defer s.Close()

	// Setup the Redis ENV variable
	redisUrl := fmt.Sprintf("redis://%s", s.Addr())
	t.Setenv("ATMOS_REDIS_URL", redisUrl)

	// Create a new Redis store
	redisStore, err := store.NewRedisStore(store.RedisStoreOptions{
		URL: &redisUrl,
	})
	assert.NoError(t, err)

	// Setup test configuration
	atmosConfig := schema.AtmosConfiguration{
		Stores: map[string]store.Store{
			"redis": redisStore,
		},
	}

	// Populate the store with some data
	require.NoError(t, redisStore.Set("dev", "vpc", "cidr", "10.0.0.0/16"))
	require.NoError(t, redisStore.Set("prod", "vpc", "cidr", "172.16.0.0/16"))
	require.NoError(t, redisStore.Set("test", "c1", "map", map[string]any{
		"a": "a1",
		"b": "b2",
		"c": map[string]any{
			"d": "d3",
			"e": "e4",
		},
	}))

	tests := []struct {
		name         string
		input        string
		currentStack string
		expected     interface{}
	}{
		{
			name:         "lookup using current stack",
			input:        "!store redis vpc cidr",
			currentStack: "dev",
			expected:     "10.0.0.0/16",
		},
		{
			name:         "basic lookup cross-stack",
			input:        "!store redis dev vpc cidr",
			currentStack: "prod",
			expected:     "10.0.0.0/16",
		},
		{
			name:         "lookup with default value without quotes",
			input:        "!store redis staging vpc cidr | default 172.20.0.0/16",
			currentStack: "dev",
			expected:     "172.20.0.0/16",
		},
		{
			name:         "lookup with default value with single quotes",
			input:        "!store redis staging vpc cidr | default '172.20.0.0/16'",
			currentStack: "dev",
			expected:     "172.20.0.0/16",
		},
		{
			name:         "lookup with default value with double quotes",
			input:        "!store redis staging vpc cidr | default \"172.20.0.0/16\"",
			currentStack: "dev",
			expected:     "172.20.0.0/16",
		},
		{
			name:         "lookup with invalid default format",
			input:        "!store redis staging vpc cidr | default",
			currentStack: "dev",
			expected:     "invalid YAML function: !store redis staging vpc cidr | default",
		},
		{
			name:         "lookup with extra parameters after default",
			input:        "!store redis staging vpc cidr | default 172.20.0.0/16 extra",
			currentStack: "dev",
			expected:     "invalid YAML function: !store redis staging vpc cidr | default 172.20.0.0/16 extra",
		},
		{
			name:         "lookup cross-stack and get a value from the result map using a YQ expression",
			input:        "!store redis test c1 map | default \"0\" | query .a",
			currentStack: "dev",
			expected:     "a1",
		},
		{
			name:         "lookup in current stack and get a value from the result map using a YQ expression",
			input:        "!store redis c1 map | query .c.e",
			currentStack: "test",
			expected:     "e4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processTagStore(&atmosConfig, tt.input, tt.currentStack)
			assert.Equal(t, tt.expected, result)
		})
	}
}
