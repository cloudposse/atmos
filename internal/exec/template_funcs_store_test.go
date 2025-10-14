package exec

import (
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestStoreTemplateFunc(t *testing.T) {
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

	tests := []struct {
		name        string
		store       string
		stack       string
		component   string
		key         string
		expected    any
		expectedErr string
	}{
		{
			name:      "Test {{ atmos.Store redis dev vpc cidr }}",
			store:     "redis",
			stack:     "dev",
			component: "vpc",
			key:       "cidr",
			expected:  "10.0.0.0/16",
		},
		{
			name:      "Test {{ atmos.Store redis prod vpc cidr }}",
			store:     "redis",
			stack:     "prod",
			component: "vpc",
			key:       "cidr",
			expected:  "172.16.0.0/16",
		},
		{
			name:        "Test invalid store",
			store:       "invalid",
			stack:       "prod",
			component:   "vpc",
			key:         "cidr",
			expectedErr: "invalid template function: atmos.Store(invalid, prod, vpc, cidr)",
		},
		{
			name:      "Test invalid store key",
			store:     "redis",
			stack:     "prod",
			component: "vpc",
			key:       "invalid",
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := storeFunc(&atmosConfig, tt.store, tt.stack, tt.component, tt.key)

			if tt.expectedErr != "" {
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestComponentConfigWithStoreTemplateFunc(t *testing.T) {
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

	// Populate the store with some data
	require.NoError(t, redisStore.Set("prod", "vpc", "cidr", "172.16.0.0/16"))
	require.NoError(t, redisStore.Set("nonprod", "c2", "map", map[string]any{
		"a": "a1",
		"b": "b2",
		"c": map[string]any{
			"d": "d3",
			"e": "e4",
		},
	}))

	stacksPath := "../../tests/fixtures/scenarios/stack-templates-4"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	res, err := ExecuteDescribeComponent(
		"component-1",
		"nonprod",
		true,
		true,
		nil,
	)

	assert.NoError(t, err)

	atmosConfig := schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Info",
		},
	}

	vars, err := u.EvaluateYqExpression(&atmosConfig, res, ".vars")
	assert.NoError(t, err)

	varsYAML, err := u.ConvertToYAML(vars)
	assert.NoError(t, err)

	expected := `
c:
  d: d3
  e: e4
c_d: d3
c_e: e4
cidr: 172.16.0.0/16
lambda_environment:
  ENGINE_CONFIG_JSON: |
    {
      "vpc_cidr": "172.16.0.0/16",
      "c": {"d":"d3","e":"e4"},
      "c_e": "e4"
    }
stage: nonprod
`

	assert.Equal(t, expected, "\n"+varsYAML)
}
