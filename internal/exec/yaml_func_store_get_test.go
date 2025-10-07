package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestProcessTagStoreGet(t *testing.T) {
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

	// Populate the store with some data using arbitrary keys
	require.NoError(t, redisStore.Set("dev", "vpc", "cidr", "10.0.0.0/16"))
	require.NoError(t, redisStore.Set("prod", "vpc", "cidr", "172.16.0.0/16"))

	// Add some arbitrary keys directly in Redis for testing GetKey
	// We need to access the Redis client directly to set arbitrary keys
	redisClient := redisStore.(*store.RedisStore).RedisClient()

	// Set arbitrary keys directly in Redis
	globalConfig := map[string]interface{}{
		"api_version": "v1",
		"region":      "us-east-1",
	}
	globalConfigJSON, _ := json.Marshal(globalConfig)
	redisClient.Set(context.Background(), "global-config", globalConfigJSON, 0)
	redisClient.Set(context.Background(), "shared-secret", "my-secret-value", 0)

	tests := []struct {
		name         string
		input        string
		currentStack string
		expected     any
	}{
		{
			name:         "Test !store.get redis global-config",
			input:        "!store.get redis global-config",
			currentStack: "dev",
			expected: map[string]interface{}{
				"api_version": "v1",
				"region":      "us-east-1",
			},
		},
		{
			name:         "Test !store.get redis shared-secret",
			input:        "!store.get redis shared-secret",
			currentStack: "prod",
			expected:     "my-secret-value",
		},
		{
			name:         "Test !store.get with query",
			input:        "!store.get redis global-config | query .region",
			currentStack: "dev",
			expected:     "us-east-1",
		},
		{
			name:         "Test !store.get with default value",
			input:        "!store.get redis non-existent-key | default default-value",
			currentStack: "dev",
			expected:     "default-value",
		},
		{
			name:         "Test invalid number of parameters",
			input:        "!store.get redis",
			currentStack: "dev",
			expected:     "invalid YAML function: !store.get redis",
		},
		{
			name:         "Test invalid number of parameters (too many)",
			input:        "!store.get redis key1 key2",
			currentStack: "dev",
			expected:     "invalid YAML function: !store.get redis key1 key2",
		},
		{
			name:         "Test invalid default format",
			input:        "!store.get redis some-key | default",
			currentStack: "dev",
			expected:     "invalid parameters after pipe: !store.get redis some-key | default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processTagStoreGet(&atmosConfig, tt.input, tt.currentStack)
			assert.Equal(t, tt.expected, result)
		})
	}
}
