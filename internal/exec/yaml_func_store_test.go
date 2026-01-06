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
			expected:     "invalid number of arguments in the Atmos YAML function: !store redis staging vpc cidr | default: invalid number of parameters after the pipe: 1",
		},
		{
			name:         "lookup with extra parameters after default",
			input:        "!store redis staging vpc cidr | default 172.20.0.0/16 extra",
			currentStack: "dev",
			expected:     "invalid number of arguments in the Atmos YAML function: !store redis staging vpc cidr | default 172.20.0.0/16 extra: invalid number of parameters after the pipe: 3",
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
			if err != nil {
				assert.Equal(t, tt.expected, err.Error())
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestProcessTagStore_ErrorPaths tests error handling paths in processTagStore.
func TestProcessTagStore_ErrorPaths(t *testing.T) {
	// Start a new Redis server.
	s := miniredis.RunT(t)
	defer s.Close()

	redisUrl := fmt.Sprintf("redis://%s", s.Addr())
	t.Setenv("ATMOS_REDIS_URL", redisUrl)

	redisStore, err := store.NewRedisStore(store.RedisStoreOptions{
		URL: &redisUrl,
	})
	require.NoError(t, err)

	// Populate store with test data.
	require.NoError(t, redisStore.Set("dev", "vpc", "cidr", "10.0.0.0/16"))

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
			input:        "!store nonexistent dev vpc cidr",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "store nonexistent not found",
		},
		{
			name: "invalid identifier after pipe (not default or query)",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store redis dev vpc cidr | invalid value",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "invalid identifier after the pipe: invalid",
		},
		{
			name: "too few parameters",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store redis vpc",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "invalid number of parameters: 2",
		},
		{
			name: "too many parameters",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store redis dev vpc cidr extra",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "invalid number of parameters: 5",
		},
		{
			name: "key not found without default",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{
					"redis": redisStore,
				},
			},
			input:        "!store redis dev vpc nonexistent",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "failed to get key nonexistent",
		},
		{
			name: "empty stores map",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: map[string]store.Store{},
			},
			input:        "!store redis dev vpc cidr",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "store redis not found",
		},
		{
			name: "nil stores map",
			atmosConfig: &schema.AtmosConfiguration{
				Stores: nil,
			},
			input:        "!store redis dev vpc cidr",
			currentStack: "dev",
			wantErr:      true,
			errContains:  "store redis not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processTagStore(tt.atmosConfig, tt.input, tt.currentStack)
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
