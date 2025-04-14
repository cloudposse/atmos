package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
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
			name:        "Test invalid store key",
			store:       "redis",
			stack:       "prod",
			component:   "vpc",
			key:         "invalid",
			expectedErr: "invalid template function: atmos.Store(redis, prod, vpc, invalid)",
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
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)

		// Change back to the original working directory after the test
		if err = os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-4"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	d, err := componentFunc(&atmosConfig, &info, "component-2", "nonprod")
	assert.NoError(t, err)

	y, err := u.ConvertToYAML(d)
	assert.NoError(t, err)

	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-b--component-1-c")
}
