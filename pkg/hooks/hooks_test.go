package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestHasHooks(t *testing.T) {
	tests := []struct {
		name     string
		hooks    Hooks
		expected bool
	}{
		{
			name: "returns false when hooks items is nil",
			hooks: Hooks{
				items: nil,
			},
			expected: false,
		},
		{
			name: "returns false when hooks items is empty",
			hooks: Hooks{
				items: map[string]Hook{},
			},
			expected: false,
		},
		{
			name: "returns true when hooks items has one hook",
			hooks: Hooks{
				items: map[string]Hook{
					"test-hook": {
						Events:  []string{"after-terraform-apply"},
						Command: "store",
					},
				},
			},
			expected: true,
		},
		{
			name: "returns true when hooks items has multiple hooks",
			hooks: Hooks{
				items: map[string]Hook{
					"hook1": {Events: []string{"after-terraform-apply"}, Command: "store"},
					"hook2": {Events: []string{"before-terraform-plan"}, Command: "store"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.hooks.HasHooks()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetHooks(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		info        *schema.ConfigAndStacksInfo
		wantErr     bool
		wantNilMap  bool
	}{
		{
			name:        "empty component and stack returns hooks with nil items",
			atmosConfig: &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				Stack:            "",
			},
			wantErr:    false,
			wantNilMap: true,
		},
		{
			name:        "empty component only returns hooks with nil items",
			atmosConfig: &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				Stack:            "test-stack",
			},
			wantErr:    false,
			wantNilMap: true,
		},
		{
			name:        "empty stack only returns hooks with nil items",
			atmosConfig: &schema.AtmosConfiguration{},
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Stack:            "",
			},
			wantErr:    false,
			wantNilMap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hooks, err := GetHooks(tt.atmosConfig, tt.info)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, hooks)
			assert.Equal(t, tt.atmosConfig, hooks.config)
			assert.Equal(t, tt.info, hooks.info)

			if tt.wantNilMap {
				assert.Nil(t, hooks.items)
			} else {
				assert.NotNil(t, hooks.items)
			}
		})
	}
}

func TestGetHooks_WithRealComponent(t *testing.T) {
	// This test uses the hooks-component-scoped test case to test the full GetHooks path
	testDir := "../../tests/test-cases/hooks-component-scoped"

	absTestDir, err := filepath.Abs(testDir)
	require.NoError(t, err)

	// Change to test directory so atmos finds the config
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(absTestDir)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "acme-dev-test",
	}

	hooks, err := GetHooks(atmosConfig, info)

	require.NoError(t, err)
	assert.NotNil(t, hooks)
	assert.Equal(t, atmosConfig, hooks.config)
	assert.Equal(t, info, hooks.info)
	assert.NotNil(t, hooks.items)
	assert.Contains(t, hooks.items, "vpc-store-outputs")
	assert.Equal(t, "store", hooks.items["vpc-store-outputs"].Command)
}

func TestConvertToHooks(t *testing.T) {
	// ConvertToHooks is currently a stub/placeholder that returns an empty Hooks struct.
	// This test documents the current behavior.
	h := Hooks{}

	result, err := h.ConvertToHooks(map[string]any{
		"test-hook": map[string]any{
			"events":  []string{"after-terraform-apply"},
			"command": "store",
		},
	})

	assert.NoError(t, err)
	assert.Empty(t, result.items)
	assert.Nil(t, result.config)
	assert.Nil(t, result.info)
}

func TestRunAll(t *testing.T) {
	tests := []struct {
		name        string
		hooks       Hooks
		setupStore  bool
		setupErr    bool
		wantErr     bool
		expectCalls int
	}{
		{
			name: "returns nil when no hooks present",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{},
				info:   &schema.ConfigAndStacksInfo{},
				items:  nil,
			},
			wantErr: false,
		},
		{
			name: "returns nil when hooks map is empty",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{},
				info:   &schema.ConfigAndStacksInfo{},
				items:  map[string]Hook{},
			},
			wantErr: false,
		},
		{
			name: "executes single store hook successfully",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{
					Stores: make(store.StoreRegistry),
				},
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
				items: map[string]Hook{
					"test-hook": {
						Events:  []string{"after-terraform-apply"},
						Command: "store",
						Name:    "test-store",
						Outputs: map[string]string{
							"key1": "value1",
						},
					},
				},
			},
			setupStore:  true,
			wantErr:     false,
			expectCalls: 1,
		},
		{
			name: "executes multiple store hooks successfully",
			hooks: Hooks{
				config: &schema.AtmosConfiguration{
					Stores: make(store.StoreRegistry),
				},
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
				items: map[string]Hook{
					"hook1": {
						Events:  []string{"after-terraform-apply"},
						Command: "store",
						Name:    "store1",
						Outputs: map[string]string{"key1": "value1"},
					},
					"hook2": {
						Events:  []string{"after-terraform-apply"},
						Command: "store",
						Name:    "store2",
						Outputs: map[string]string{"key2": "value2"},
					},
				},
			},
			setupStore:  true,
			wantErr:     false,
			expectCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock stores if needed
			if tt.setupStore && tt.hooks.config != nil {
				for _, hook := range tt.hooks.items {
					mockStore := NewMockStore()
					if tt.setupErr {
						mockStore.SetSetError(assert.AnError)
					}
					tt.hooks.config.Stores[hook.Name] = mockStore
				}
			}

			// Note: RunAll calls CheckErrorPrintAndExit on errors, which would exit the process
			// In a real scenario, this would need to be refactored to return errors instead
			// For now, we can only test the successful path
			if !tt.wantErr {
				err := tt.hooks.RunAll(AfterTerraformApply, tt.hooks.config, tt.hooks.info, nil, nil)
				assert.NoError(t, err)
			}
		})
	}
}
