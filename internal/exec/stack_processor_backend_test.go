package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessTerraformBackend(t *testing.T) {
	tests := []struct {
		name                        string
		component                   string
		baseComponentName           string
		globalBackendType           string
		globalBackendSection        map[string]any
		baseComponentBackendType    string
		baseComponentBackendSection map[string]any
		componentBackendType        string
		componentBackendSection     map[string]any
		expectedBackendType         string
		expectedBackendConfig       map[string]any
		expectedWorkspaceKeyPrefix  string // For S3
		expectedPrefix              string // For GCS
		expectedKey                 string // For Azure
	}{
		{
			name:              "s3 backend with default workspace_key_prefix",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
					"region": "us-east-1",
				},
			},
			expectedBackendType: "s3",
			expectedBackendConfig: map[string]any{
				"bucket":               "test-bucket",
				"region":               "us-east-1",
				"workspace_key_prefix": "vpc",
			},
			expectedWorkspaceKeyPrefix: "vpc",
		},
		{
			name:              "s3 backend with base component name",
			component:         "derived-vpc",
			baseComponentName: "base-vpc",
			globalBackendType: "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType:        "s3",
			expectedWorkspaceKeyPrefix: "base-vpc",
		},
		{
			name:              "s3 backend with existing workspace_key_prefix",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket":               "test-bucket",
					"workspace_key_prefix": "custom-prefix",
				},
			},
			expectedBackendType:        "s3",
			expectedWorkspaceKeyPrefix: "custom-prefix",
		},
		{
			name:              "gcs backend with default prefix",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "gcs",
			globalBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType: "gcs",
			expectedPrefix:      "vpc",
		},
		{
			name:              "gcs backend with base component name",
			component:         "derived-vpc",
			baseComponentName: "base-vpc",
			globalBackendType: "gcs",
			globalBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType: "gcs",
			expectedPrefix:      "base-vpc",
		},
		{
			name:              "azurerm backend with default key",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "azurerm",
			globalBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
					"container_name":       "tfstate",
					"key":                  "global",
				},
			},
			componentBackendSection: map[string]any{},
			expectedBackendType:     "azurerm",
			expectedKey:             "global/vpc.terraform.tfstate",
		},
		{
			name:              "azurerm backend with base component name",
			component:         "derived-vpc",
			baseComponentName: "base-vpc",
			globalBackendType: "azurerm",
			globalBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
					"container_name":       "tfstate",
					"key":                  "global",
				},
			},
			componentBackendSection: map[string]any{},
			expectedBackendType:     "azurerm",
			expectedKey:             "global/base-vpc.terraform.tfstate",
		},
		{
			name:                     "backend type precedence - component overrides base",
			component:                "vpc",
			baseComponentName:        "",
			globalBackendType:        "s3",
			baseComponentBackendType: "gcs",
			componentBackendType:     "azurerm",
			componentBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
				},
			},
			expectedBackendType: "azurerm",
		},
		{
			name:                     "backend type precedence - base overrides global",
			component:                "vpc",
			baseComponentName:        "",
			globalBackendType:        "s3",
			baseComponentBackendType: "gcs",
			globalBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType: "gcs",
		},
		{
			name:              "component with slashes in name",
			component:         "path/to/vpc",
			baseComponentName: "",
			globalBackendType: "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType:        "s3",
			expectedWorkspaceKeyPrefix: "path-to-vpc",
		},
		{
			name:              "azurerm backend with authored key from base component",
			component:         "derived-vpc",
			baseComponentName: "base-vpc",
			globalBackendType: "azurerm",
			globalBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
					"container_name":       "tfstate",
					"key":                  "global",
				},
			},
			baseComponentBackendSection: map[string]any{
				"azurerm": map[string]any{
					"key": "custom/authored/state.tfstate",
				},
			},
			componentBackendSection: map[string]any{},
			expectedBackendType:     "azurerm",
			expectedKey:             "custom/authored/state.tfstate",
		},
		{
			name:              "azurerm backend with global key treated as prefix",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "azurerm",
			globalBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
					"container_name":       "tfstate",
					"key":                  "global",
				},
			},
			baseComponentBackendSection: map[string]any{
				"azurerm": map[string]any{
					"key": "global",
				},
			},
			componentBackendSection: map[string]any{},
			expectedBackendType:     "azurerm",
			expectedKey:             "global/vpc.terraform.tfstate",
		},
		{
			name:              "azurerm backend preserves component-specific key",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "azurerm",
			globalBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
					"container_name":       "tfstate",
				},
			},
			componentBackendSection: map[string]any{
				"azurerm": map[string]any{
					"key": "component-specific.tfstate",
				},
			},
			expectedBackendType: "azurerm",
			expectedKey:         "component-specific.tfstate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			backendType, backendConfig, err := processTerraformBackend(
				&terraformBackendConfig{
					atmosConfig:                 atmosConfig,
					component:                   tt.component,
					baseComponentName:           tt.baseComponentName,
					globalBackendType:           tt.globalBackendType,
					globalBackendSection:        tt.globalBackendSection,
					baseComponentBackendType:    tt.baseComponentBackendType,
					baseComponentBackendSection: tt.baseComponentBackendSection,
					componentBackendType:        tt.componentBackendType,
					componentBackendSection:     tt.componentBackendSection,
				},
			)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedBackendType, backendType)

			if tt.expectedWorkspaceKeyPrefix != "" {
				assert.Equal(t, tt.expectedWorkspaceKeyPrefix, backendConfig["workspace_key_prefix"])
			}

			if tt.expectedPrefix != "" {
				assert.Equal(t, tt.expectedPrefix, backendConfig["prefix"])
			}

			if tt.expectedKey != "" {
				assert.Equal(t, tt.expectedKey, backendConfig["key"])
			}

			if tt.expectedBackendConfig != nil {
				for key, expectedValue := range tt.expectedBackendConfig {
					assert.Equal(t, expectedValue, backendConfig[key])
				}
			}
		})
	}
}

func TestProcessTerraformRemoteStateBackend(t *testing.T) {
	tests := []struct {
		name                                   string
		component                              string
		finalComponentBackendType              string
		finalComponentBackendSection           map[string]any
		globalRemoteStateBackendType           string
		globalRemoteStateBackendSection        map[string]any
		baseComponentRemoteStateBackendType    string
		baseComponentRemoteStateBackendSection map[string]any
		componentRemoteStateBackendType        string
		componentRemoteStateBackendSection     map[string]any
		expectedRemoteStateBackendType         string
		expectedRemoteStateBackendConfigNotNil bool
	}{
		{
			name:                      "remote state backend inherits from backend type",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedRemoteStateBackendType:         "s3",
			expectedRemoteStateBackendConfigNotNil: true,
		},
		{
			name:                      "global remote state backend type overrides backend type",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			globalRemoteStateBackendType: "gcs",
			globalRemoteStateBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "remote-state-bucket",
				},
			},
			expectedRemoteStateBackendType:         "gcs",
			expectedRemoteStateBackendConfigNotNil: true,
		},
		{
			name:                      "component remote state backend type takes precedence",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			globalRemoteStateBackendType:        "gcs",
			baseComponentRemoteStateBackendType: "azurerm",
			componentRemoteStateBackendType:     "s3",
			componentRemoteStateBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "component-remote-state",
				},
			},
			expectedRemoteStateBackendType:         "s3",
			expectedRemoteStateBackendConfigNotNil: true,
		},
		{
			name:                      "merges backend and remote_state_backend sections",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "backend-bucket",
					"region": "us-east-1",
				},
			},
			componentRemoteStateBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "remote-state-bucket",
				},
			},
			expectedRemoteStateBackendType:         "s3",
			expectedRemoteStateBackendConfigNotNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			remoteStateBackendType, remoteStateBackendConfig, err := processTerraformRemoteStateBackend(
				&remoteStateBackendConfig{
					atmosConfig:                            atmosConfig,
					component:                              tt.component,
					finalComponentBackendType:              tt.finalComponentBackendType,
					finalComponentBackendSection:           tt.finalComponentBackendSection,
					globalRemoteStateBackendType:           tt.globalRemoteStateBackendType,
					globalRemoteStateBackendSection:        tt.globalRemoteStateBackendSection,
					baseComponentRemoteStateBackendType:    tt.baseComponentRemoteStateBackendType,
					baseComponentRemoteStateBackendSection: tt.baseComponentRemoteStateBackendSection,
					componentRemoteStateBackendType:        tt.componentRemoteStateBackendType,
					componentRemoteStateBackendSection:     tt.componentRemoteStateBackendSection,
				},
			)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedRemoteStateBackendType, remoteStateBackendType)

			if tt.expectedRemoteStateBackendConfigNotNil {
				assert.NotNil(t, remoteStateBackendConfig)
			}
		})
	}
}
