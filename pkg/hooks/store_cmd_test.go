package hooks

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

func TestNewStoreCommand(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "test-component",
		Stack:            "test-stack",
	}

	cmd, err := NewStoreCommand(atmosConfig, info)

	require.NoError(t, err)
	assert.NotNil(t, cmd)
	assert.Equal(t, "store", cmd.Name)
	assert.Equal(t, atmosConfig, cmd.atmosConfig)
	assert.Equal(t, info, cmd.info)
}

func TestStoreCommand_GetName(t *testing.T) {
	cmd := &StoreCommand{
		Name: "store",
	}

	assert.Equal(t, "store", cmd.GetName())
}

func TestStoreCommand_GetOutputValue(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		expectedKey   string
		expectedValue any
	}{
		{
			name:          "literal value without dot prefix returns value as-is",
			value:         "literal-value",
			expectedKey:   "literal-value",
			expectedValue: "literal-value",
		},
		{
			name:          "empty string returns empty string",
			value:         "",
			expectedKey:   "",
			expectedValue: "",
		},
		{
			name:          "complex literal value",
			value:         "arn:aws:vpc:us-east-1:123456789012:vpc/vpc-12345",
			expectedKey:   "arn:aws:vpc:us-east-1:123456789012:vpc/vpc-12345",
			expectedValue: "arn:aws:vpc:us-east-1:123456789012:vpc/vpc-12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &StoreCommand{
				atmosConfig: &schema.AtmosConfiguration{},
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
			}

			key, value := cmd.getOutputValue(tt.value)

			assert.Equal(t, tt.expectedKey, key)
			assert.Equal(t, tt.expectedValue, value)
		})
	}
}

func TestStoreCommand_GetOutputValue_DotPrefix(t *testing.T) {
	t.Skipf("Skipping test that requires full Atmos configuration with terraform outputs")

	// Test that dot prefix correctly strips the dot and prepares for terraform output lookup
	// Note: This test verifies the key transformation only, not the actual terraform output retrieval
	cmd := &StoreCommand{
		atmosConfig: &schema.AtmosConfiguration{},
		info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "test-component",
			Stack:            "test-stack",
		},
	}

	key, _ := cmd.getOutputValue(".vpc_id")

	// Verify the dot is stripped from the key
	assert.Equal(t, "vpc_id", key)
	// The actual value retrieval would require GetTerraformOutput to work,
	// which requires a full Atmos setup. We're only testing the key transformation here.
}

func TestStoreCommand_StoreOutput(t *testing.T) {
	tests := []struct {
		name          string
		hookName      string
		setupStore    bool
		setupStoreErr bool
		key           string
		outputKey     string
		outputValue   any
		wantErr       bool
		errContains   string
	}{
		{
			name:        "successfully stores output when store exists",
			hookName:    "test-store",
			setupStore:  true,
			key:         "vpc_id",
			outputKey:   "vpc_id",
			outputValue: "vpc-12345",
			wantErr:     false,
		},
		{
			name:        "returns error when store not found",
			hookName:    "nonexistent-store",
			setupStore:  false,
			key:         "vpc_id",
			outputKey:   "vpc_id",
			outputValue: "vpc-12345",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:          "returns error when store Set fails",
			hookName:      "test-store",
			setupStore:    true,
			setupStoreErr: true,
			key:           "vpc_id",
			outputKey:     "vpc_id",
			outputValue:   "vpc-12345",
			wantErr:       true,
			errContains:   "store error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := NewMockStore()
			if tt.setupStoreErr {
				mockStore.SetSetError(errors.New("store error"))
			}

			atmosConfig := &schema.AtmosConfiguration{
				Stores: make(store.StoreRegistry),
			}

			if tt.setupStore {
				atmosConfig.Stores[tt.hookName] = mockStore
			}

			cmd := &StoreCommand{
				atmosConfig: atmosConfig,
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
			}

			hook := &Hook{
				Name: tt.hookName,
			}

			err := cmd.storeOutput(hook, tt.key, tt.outputKey, tt.outputValue)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				// Verify the value was stored
				data := mockStore.GetData()
				expectedKey := "test-stack/test-component/" + tt.key
				assert.Equal(t, tt.outputValue, data[expectedKey])
			}
		})
	}
}

func TestStoreCommand_ProcessStoreCommand(t *testing.T) {
	tests := []struct {
		name        string
		hook        *Hook
		setupStore  bool
		setupErr    bool
		wantErr     bool
		errContains string
		expectSkip  bool
	}{
		{
			name: "skips when no outputs configured",
			hook: &Hook{
				Name:    "test-store",
				Outputs: map[string]string{},
			},
			setupStore: true,
			expectSkip: true,
			wantErr:    false,
		},
		{
			name: "processes single literal output successfully",
			hook: &Hook{
				Name: "test-store",
				Outputs: map[string]string{
					"key1": "value1",
				},
			},
			setupStore: true,
			wantErr:    false,
		},
		{
			name: "processes multiple outputs successfully",
			hook: &Hook{
				Name: "test-store",
				Outputs: map[string]string{
					"key1": "value1",
					"key2": "value2",
					"key3": "value3",
				},
			},
			setupStore: true,
			wantErr:    false,
		},
		{
			name: "returns error when store not found",
			hook: &Hook{
				Name: "nonexistent-store",
				Outputs: map[string]string{
					"key1": "value1",
				},
			},
			setupStore:  false,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name: "returns error when store Set fails",
			hook: &Hook{
				Name: "test-store",
				Outputs: map[string]string{
					"key1": "value1",
				},
			},
			setupStore:  true,
			setupErr:    true,
			wantErr:     true,
			errContains: "store error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := NewMockStore()
			if tt.setupErr {
				mockStore.SetSetError(errors.New("store error"))
			}

			atmosConfig := &schema.AtmosConfiguration{
				Stores: make(store.StoreRegistry),
			}

			if tt.setupStore {
				atmosConfig.Stores[tt.hook.Name] = mockStore
			}

			cmd := &StoreCommand{
				Name:        "store",
				atmosConfig: atmosConfig,
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
			}

			err := cmd.processStoreCommand(tt.hook)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)

				if !tt.expectSkip {
					// Verify all outputs were stored
					data := mockStore.GetData()
					for key, value := range tt.hook.Outputs {
						expectedKey := "test-stack/test-component/" + key
						assert.Equal(t, value, data[expectedKey], "output %s should be stored", key)
					}
				}
			}
		})
	}
}

func TestStoreCommand_RunE(t *testing.T) {
	tests := []struct {
		name        string
		hook        *Hook
		setupStore  bool
		wantErr     bool
		errContains string
	}{
		{
			name: "delegates to processStoreCommand successfully",
			hook: &Hook{
				Name: "test-store",
				Outputs: map[string]string{
					"key1": "value1",
				},
			},
			setupStore: true,
			wantErr:    false,
		},
		{
			name: "returns error from processStoreCommand",
			hook: &Hook{
				Name: "nonexistent-store",
				Outputs: map[string]string{
					"key1": "value1",
				},
			},
			setupStore:  false,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStore := NewMockStore()

			atmosConfig := &schema.AtmosConfiguration{
				Stores: make(store.StoreRegistry),
			}

			if tt.setupStore {
				atmosConfig.Stores[tt.hook.Name] = mockStore
			}

			cmd := &StoreCommand{
				Name:        "store",
				atmosConfig: atmosConfig,
				info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
				},
			}

			err := cmd.RunE(tt.hook, AfterTerraformApply, nil, nil)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
