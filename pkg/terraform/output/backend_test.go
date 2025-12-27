package output

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGenerateBackendConfig(t *testing.T) {
	tests := []struct {
		name          string
		backendType   string
		backendConfig map[string]any
		workspace     string
		authContext   *schema.AuthContext
		expectError   bool
		expectedErr   error
		expectedType  string
	}{
		{
			name:        "s3 backend",
			backendType: "s3",
			backendConfig: map[string]any{
				"bucket":         "my-terraform-state",
				"key":            "terraform.tfstate",
				"region":         "us-west-2",
				"dynamodb_table": "terraform-locks",
				"encrypt":        true,
			},
			workspace:    "dev",
			authContext:  nil,
			expectError:  false,
			expectedType: "s3",
		},
		{
			name:        "local backend",
			backendType: "local",
			backendConfig: map[string]any{
				"path": "/tmp/terraform.tfstate",
			},
			workspace:    "default",
			authContext:  nil,
			expectError:  false,
			expectedType: "local",
		},
		{
			name:        "gcs backend",
			backendType: "gcs",
			backendConfig: map[string]any{
				"bucket": "my-gcs-bucket",
				"prefix": "terraform/state",
			},
			workspace:    "prod",
			authContext:  nil,
			expectError:  false,
			expectedType: "gcs",
		},
		{
			name:          "azurerm backend",
			backendType:   "azurerm",
			backendConfig: map[string]any{},
			workspace:     "staging",
			authContext:   nil,
			expectError:   false,
			expectedType:  "azurerm",
		},
		{
			name:          "empty backend type",
			backendType:   "",
			backendConfig: map[string]any{"bucket": "test"},
			workspace:     "dev",
			authContext:   nil,
			expectError:   true,
			expectedErr:   errUtils.ErrBackendTypeRequired,
		},
		{
			name:        "with auth context",
			backendType: "s3",
			backendConfig: map[string]any{
				"bucket": "my-bucket",
			},
			workspace: "dev",
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "my-profile",
					Region:  "us-east-1",
				},
			},
			expectError:  false,
			expectedType: "s3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateBackendConfig(tt.backendType, tt.backendConfig, tt.workspace, tt.authContext)

			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr), "expected %v, got %v", tt.expectedErr, err)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify structure.
			terraform, ok := result["terraform"].(map[string]any)
			require.True(t, ok, "expected terraform key in result")

			backend, ok := terraform["backend"].(map[string]any)
			require.True(t, ok, "expected backend key in terraform")

			backendTypeConfig, ok := backend[tt.expectedType]
			require.True(t, ok, "expected backend type %s in backend config", tt.expectedType)

			// Verify the backend config is passed through.
			assert.Equal(t, tt.backendConfig, backendTypeConfig)
		})
	}
}

func TestGenerateProviderOverrides(t *testing.T) {
	tests := []struct {
		name           string
		providers      map[string]any
		authContext    *schema.AuthContext
		expectedResult map[string]any
	}{
		{
			name: "single aws provider",
			providers: map[string]any{
				"aws": map[string]any{
					"region": "us-west-2",
				},
			},
			authContext: nil,
			expectedResult: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
					},
				},
			},
		},
		{
			name: "multiple providers",
			providers: map[string]any{
				"aws": map[string]any{
					"region": "us-west-2",
				},
				"google": map[string]any{
					"project": "my-project",
					"region":  "us-central1",
				},
			},
			authContext: nil,
			expectedResult: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
					},
					"google": map[string]any{
						"project": "my-project",
						"region":  "us-central1",
					},
				},
			},
		},
		{
			name:        "empty providers",
			providers:   map[string]any{},
			authContext: nil,
			expectedResult: map[string]any{
				"provider": map[string]any{},
			},
		},
		{
			name: "with auth context",
			providers: map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
			},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "prod-profile",
				},
			},
			expectedResult: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateProviderOverrides(tt.providers, tt.authContext)

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestDefaultBackendGenerator_GenerateBackendIfNeeded(t *testing.T) {
	tests := []struct {
		name                string
		config              *ComponentConfig
		component           string
		stack               string
		authContext         *schema.AuthContext
		expectError         bool
		expectedErr         error
		expectFileCreated   bool
		expectedBackendType string
	}{
		{
			name: "auto-generate disabled - no file created",
			config: &ComponentConfig{
				AutoGenerateBackend: false,
				BackendType:         "s3",
				Backend:             map[string]any{"bucket": "test"},
			},
			component:         "vpc",
			stack:             "dev",
			authContext:       nil,
			expectError:       false,
			expectFileCreated: false,
		},
		{
			name: "successful backend generation",
			config: &ComponentConfig{
				AutoGenerateBackend: true,
				BackendType:         "s3",
				Backend: map[string]any{
					"bucket": "test-bucket",
					"key":    "state.tfstate",
				},
				Workspace: "dev-workspace",
			},
			component:           "vpc",
			stack:               "dev-us-west-2",
			authContext:         nil,
			expectError:         false,
			expectFileCreated:   true,
			expectedBackendType: "s3",
		},
		{
			name: "validation error - missing backend type",
			config: &ComponentConfig{
				AutoGenerateBackend: true,
				BackendType:         "",
				Backend:             map[string]any{"bucket": "test"},
			},
			component:   "vpc",
			stack:       "dev",
			authContext: nil,
			expectError: true,
			expectedErr: errUtils.ErrBackendFileGeneration,
		},
		{
			name: "validation error - missing backend config",
			config: &ComponentConfig{
				AutoGenerateBackend: true,
				BackendType:         "s3",
				Backend:             nil,
			},
			component:   "vpc",
			stack:       "dev",
			authContext: nil,
			expectError: true,
			expectedErr: errUtils.ErrBackendFileGeneration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp directory for the component path.
			tempDir := t.TempDir()
			tt.config.ComponentPath = tempDir

			generator := &defaultBackendGenerator{}
			err := generator.GenerateBackendIfNeeded(tt.config, tt.component, tt.stack, tt.authContext)

			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.True(t, errors.Is(err, tt.expectedErr), "expected %v, got %v", tt.expectedErr, err)
				}
				return
			}

			require.NoError(t, err)

			// Check if file was created.
			backendFile := filepath.Join(tempDir, "backend.tf.json")
			if tt.expectFileCreated {
				data, err := os.ReadFile(backendFile)
				require.NoError(t, err, "backend file should exist")

				// Verify JSON structure.
				var backendConfig map[string]any
				err = json.Unmarshal(data, &backendConfig)
				require.NoError(t, err, "backend file should be valid JSON")

				terraform, ok := backendConfig["terraform"].(map[string]any)
				require.True(t, ok, "should have terraform key")

				backend, ok := terraform["backend"].(map[string]any)
				require.True(t, ok, "should have backend key")

				_, ok = backend[tt.expectedBackendType]
				assert.True(t, ok, "should have backend type %s", tt.expectedBackendType)
			} else {
				_, err := os.Stat(backendFile)
				assert.True(t, os.IsNotExist(err), "backend file should not exist")
			}
		})
	}
}

func TestDefaultBackendGenerator_GenerateProvidersIfNeeded(t *testing.T) {
	tests := []struct {
		name              string
		config            *ComponentConfig
		authContext       *schema.AuthContext
		expectError       bool
		expectFileCreated bool
	}{
		{
			name: "no providers - no file created",
			config: &ComponentConfig{
				Providers: nil,
			},
			authContext:       nil,
			expectError:       false,
			expectFileCreated: false,
		},
		{
			name: "empty providers - no file created",
			config: &ComponentConfig{
				Providers: map[string]any{},
			},
			authContext:       nil,
			expectError:       false,
			expectFileCreated: false,
		},
		{
			name: "successful provider override generation",
			config: &ComponentConfig{
				Providers: map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
					},
				},
			},
			authContext:       nil,
			expectError:       false,
			expectFileCreated: true,
		},
		{
			name: "multiple providers",
			config: &ComponentConfig{
				Providers: map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
					},
					"google": map[string]any{
						"project": "my-project",
					},
				},
			},
			authContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "prod-profile",
				},
			},
			expectError:       false,
			expectFileCreated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp directory for the component path.
			tempDir := t.TempDir()
			tt.config.ComponentPath = tempDir

			generator := &defaultBackendGenerator{}
			err := generator.GenerateProvidersIfNeeded(tt.config, tt.authContext)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Check if file was created.
			providerFile := filepath.Join(tempDir, "providers_override.tf.json")
			if tt.expectFileCreated {
				data, err := os.ReadFile(providerFile)
				require.NoError(t, err, "provider file should exist")

				// Verify JSON structure.
				var providerConfig map[string]any
				err = json.Unmarshal(data, &providerConfig)
				require.NoError(t, err, "provider file should be valid JSON")

				_, ok := providerConfig["provider"]
				assert.True(t, ok, "should have provider key")
			} else {
				_, err := os.Stat(providerFile)
				assert.True(t, os.IsNotExist(err), "provider file should not exist")
			}
		})
	}
}
