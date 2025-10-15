package exec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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
	origRedisUrl := os.Getenv("ATMOS_REDIS_URL")
	os.Setenv("ATMOS_REDIS_URL", redisUrl)
	defer os.Setenv("ATMOS_REDIS_URL", origRedisUrl)

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
			result, err := processTagStoreGet(&atmosConfig, tt.input, tt.currentStack)
			expectedStr, isString := tt.expected.(string)
			if isString && expectedStr != "" && strings.Contains(expectedStr, "invalid") {
				// For error cases, check the error message
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), "invalid")
				}
			} else {
				// For success cases, check the result
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestProcessTagStoreGet_ErrorPaths(t *testing.T) {
	// Start a new Redis server
	s := miniredis.RunT(t)
	defer s.Close()

	// Setup the Redis ENV variable
	redisUrl := fmt.Sprintf("redis://%s", s.Addr())
	origRedisUrl := os.Getenv("ATMOS_REDIS_URL")
	os.Setenv("ATMOS_REDIS_URL", redisUrl)
	defer os.Setenv("ATMOS_REDIS_URL", origRedisUrl)

	// Create a new Redis store
	redisStore, err := store.NewRedisStore(store.RedisStoreOptions{
		URL: &redisUrl,
	})
	require.NoError(t, err)

	// Setup test configuration
	atmosConfig := schema.AtmosConfiguration{
		Stores: map[string]store.Store{
			"redis": redisStore,
		},
	}

	// Add test data
	redisClient := redisStore.(*store.RedisStore).RedisClient()
	redisClient.Set(context.Background(), "test-key", "test-value", 0)

	tests := []struct {
		name          string
		input         string
		currentStack  string
		expectError   bool
		errorContains string
	}{
		{
			name:          "empty after tag",
			input:         "!store.get",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid Atmos YAML function",
		},
		{
			name:          "insufficient parameters - 1 param",
			input:         "!store.get redis",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of parameters: 1",
		},
		{
			name:          "too many parameters - 3 params",
			input:         "!store.get redis key1 key2",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of parameters: 3",
		},
		{
			name:          "insufficient parameters - 0 params (empty space)",
			input:         "!store.get ",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid Atmos YAML function", // Caught by getStringAfterTag
		},
		{
			name:          "invalid pipe parameter - missing value after default",
			input:         "!store.get redis test-key | default",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid parameters after pipe",
		},
		{
			name:          "invalid pipe parameter - extra params after default",
			input:         "!store.get redis test-key | default value extra",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid parameters after pipe",
		},
		{
			name:          "invalid pipe identifier",
			input:         "!store.get redis test-key | invalid value",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid identifier after pipe",
		},
		{
			name:          "store not found in config",
			input:         "!store.get nonexistent some-key",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "store not found",
		},
		{
			name:          "key not found without default",
			input:         "!store.get redis nonexistent-key",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "failed to get key from store",
		},
		{
			name:          "invalid yq expression",
			input:         "!store.get redis test-key | query .invalid.[syntax",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "", // YQ error varies
		},
		{
			name:          "query on string with field path (valid - yq allows this)",
			input:         "!store.get redis test-key | query .field",
			currentStack:  "dev",
			expectError:   false, // YQ on string returns null, not error
			errorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processTagStoreGet(&atmosConfig, tt.input, tt.currentStack)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				// For error cases, result should be nil
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				// Result may be nil for valid queries that return null (e.g., YQ on strings)
			}
		})
	}
}
