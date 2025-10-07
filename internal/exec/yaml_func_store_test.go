package exec

import (
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

func TestProcessTagStore(t *testing.T) {
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
			result, err := processTagStore(&atmosConfig, tt.input, tt.currentStack)
			expectedStr, isString := tt.expected.(string)
			if isString && expectedStr != "" && strings.Contains(expectedStr, "invalid YAML function") {
				// For error cases, check the error message
				assert.Error(t, err)
				if err != nil {
					assert.Contains(t, err.Error(), "invalid YAML function")
				}
			} else {
				// For success cases, check the result
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestProcessTagStore_ErrorPaths(t *testing.T) {
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

	// Populate with test data
	require.NoError(t, redisStore.Set("dev", "vpc", "cidr", "10.0.0.0/16"))

	tests := []struct {
		name             string
		input            string
		currentStack     string
		expectError      bool
		errorContains    string
		setupEmptyConfig bool // Use config with no stores
	}{
		{
			name:          "empty after tag",
			input:         "!store",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid Atmos YAML function",
		},
		{
			name:          "insufficient parameters - 2 params",
			input:         "!store redis vpc",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of parameters: 2",
		},
		{
			name:          "insufficient parameters - 1 param",
			input:         "!store redis",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of parameters: 1",
		},
		{
			name:          "too many parameters - 5 params",
			input:         "!store redis dev vpc cidr extra",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of parameters: 5",
		},
		{
			name:          "invalid pipe parameter - missing value after default",
			input:         "!store redis vpc cidr | default",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of parameters after the pipe: 1",
		},
		{
			name:          "invalid pipe parameter - extra params after default",
			input:         "!store redis vpc cidr | default value extra",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid number of parameters after the pipe: 3",
		},
		{
			name:          "invalid pipe identifier",
			input:         "!store redis vpc cidr | invalid value",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "invalid identifier after the pipe: invalid",
		},
		{
			name:             "store not found in config",
			input:            "!store nonexistent vpc cidr",
			currentStack:     "dev",
			expectError:      true,
			errorContains:    "Store nonexistent not found",
			setupEmptyConfig: false,
		},
		{
			name:          "key not found without default",
			input:         "!store redis vpc nonexistent",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "Failed to get key nonexistent",
		},
		{
			name:          "invalid yq expression",
			input:         "!store redis vpc cidr | query .invalid.[syntax",
			currentStack:  "dev",
			expectError:   true,
			errorContains: "", // YQ error varies
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &atmosConfig
			if tt.setupEmptyConfig {
				config = &schema.AtmosConfiguration{
					Stores: map[string]store.Store{},
				}
			}

			result, err := processTagStore(config, tt.input, tt.currentStack)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}
