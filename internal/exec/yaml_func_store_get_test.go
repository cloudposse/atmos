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
			expected:     "invalid number of arguments in the Atmos YAML function: !store.get redis: invalid number of parameters: 1",
		},
		{
			name:         "Test invalid number of parameters (too many)",
			input:        "!store.get redis key1 key2",
			currentStack: "dev",
			expected:     "invalid number of arguments in the Atmos YAML function: !store.get redis key1 key2: invalid number of parameters: 3",
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
			result, err := processTagStoreGet(&atmosConfig, tt.input, tt.currentStack)
			if err != nil {
				assert.Equal(t, tt.expected, err.Error())
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestProcessTagStoreGet_ErrorPaths tests error handling paths in processTagStoreGet.
func TestProcessTagStoreGet_ErrorPaths(t *testing.T) {
	// Start a new Redis server.
	s := miniredis.RunT(t)
	defer s.Close()

	redisUrl := fmt.Sprintf("redis://%s", s.Addr())
	t.Setenv("ATMOS_REDIS_URL", redisUrl)

	redisStore, err := store.NewRedisStore(store.RedisStoreOptions{
		URL: &redisUrl,
	})
	require.NoError(t, err)

	// Set up a key directly in Redis.
	redisClient := redisStore.(*store.RedisStore).RedisClient()
	redisClient.Set(context.Background(), "test-key", "test-value", 0)

	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		input        string
		currentStack string
		wantErr      bool
		errContains  string
	}{
		{
			name: "store not found",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store.get nonexistent test-key",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "store not found: nonexistent",
		},
		{
			name: "invalid identifier after pipe (not default or query)",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store.get redis test-key | invalid value",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "invalid identifier after pipe",
		},
		{
			name: "key not found without default",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store.get redis nonexistent-key",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "failed to execute YAML function",
		},
		{
			name: "empty stores map",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{},
			},
			input:        "!store.get redis test-key",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "store not found: redis",
		},
		{
			name: "nil stores map",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: nil,
			},
			input:        "!store.get redis test-key",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "store not found: redis",
		},
		{
			name: "empty input after tag",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store.get",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "invalid Atmos YAML function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processTagStoreGet(tt.atmosConfig, tt.input, tt.currentStack)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestProcessTagStoreGet_NilValueWithDefault tests that nil values stored (e.g., from rate limit failures)
// correctly fall back to default values.
func TestProcessTagStoreGet_NilValueWithDefault(t *testing.T) {
	// Start a new Redis server.
	s := miniredis.RunT(t)
	defer s.Close()

	redisUrl := fmt.Sprintf("redis://%s", s.Addr())
	t.Setenv("ATMOS_REDIS_URL", redisUrl)

	redisStore, err := store.NewRedisStore(store.RedisStoreOptions{
		URL: &redisUrl,
	})
	require.NoError(t, err)

	// Set a null/nil value in Redis.
	redisClient := redisStore.(*store.RedisStore).RedisClient()
	redisClient.Set(context.Background(), "nil-key", "null", 0)

	atmosConfig := schema.AtmosConfiguration{
		Stores: map[string]store.Store{
			"redis": redisStore,
		},
	}

	// Test that a nil value with default returns the default.
	result, err := processTagStoreGet(&atmosConfig, "!store.get redis nil-key | default fallback-value", "dev")
	require.NoError(t, err)
	// The "null" string decodes to nil, which should trigger the default.
	assert.Equal(t, "fallback-value", result)
}
