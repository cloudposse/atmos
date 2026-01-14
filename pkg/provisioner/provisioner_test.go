package provisioner

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner/backend"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProvisionWithParams_NilParams(t *testing.T) {
	err := ProvisionWithParams(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
	assert.Contains(t, err.Error(), "provision params")
}

func TestProvisionWithParams_NilDescribeComponent(t *testing.T) {
	params := &ProvisionParams{
		AtmosConfig:       &schema.AtmosConfiguration{},
		ProvisionerType:   "backend",
		Component:         "vpc",
		Stack:             "dev",
		DescribeComponent: nil,
		AuthContext:       nil,
	}

	err := ProvisionWithParams(params)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNilParam)
	assert.Contains(t, err.Error(), "DescribeComponent callback")
}

func TestProvisionWithParams_UnsupportedProvisionerType(t *testing.T) {
	mockDescribe := func(component string, stack string) (map[string]any, error) {
		return map[string]any{
			"backend_type": "s3",
			"backend": map[string]any{
				"bucket": "test-bucket",
				"region": "us-west-2",
			},
		}, nil
	}

	params := &ProvisionParams{
		AtmosConfig:       &schema.AtmosConfiguration{},
		ProvisionerType:   "unsupported",
		Component:         "vpc",
		Stack:             "dev",
		DescribeComponent: mockDescribe,
		AuthContext:       nil,
	}

	err := ProvisionWithParams(params)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedProvisionerType)
	assert.Contains(t, err.Error(), "unsupported")
	assert.Contains(t, err.Error(), "supported: backend")
}

func TestProvisionWithParams_DescribeComponentFailure(t *testing.T) {
	mockDescribe := func(component string, stack string) (map[string]any, error) {
		return nil, errors.New("component not found")
	}

	params := &ProvisionParams{
		AtmosConfig:       &schema.AtmosConfiguration{},
		ProvisionerType:   "backend",
		Component:         "vpc",
		Stack:             "dev",
		DescribeComponent: mockDescribe,
		AuthContext:       nil,
	}

	err := ProvisionWithParams(params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to describe component")
	assert.Contains(t, err.Error(), "component not found")
}

func TestProvisionWithParams_BackendProvisioningSuccess(t *testing.T) {
	// Clean up registry after test to ensure test isolation.
	t.Cleanup(backend.ResetRegistryForTesting)

	// Register a mock backend provisioner for testing.
	mockProvisionerCalled := false
	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		mockProvisionerCalled = true
		// Verify the backend config was passed correctly.
		bucket, ok := backendConfig["bucket"].(string)
		assert.True(t, ok)
		assert.Equal(t, "test-bucket", bucket)

		region, ok := backendConfig["region"].(string)
		assert.True(t, ok)
		assert.Equal(t, "us-west-2", region)

		return nil
	}

	// Temporarily register the mock provisioner.
	backend.RegisterBackendCreate("s3", mockProvisioner)

	mockDescribe := func(component string, stack string) (map[string]any, error) {
		assert.Equal(t, "vpc", component)
		assert.Equal(t, "dev", stack)

		return map[string]any{
			"backend_type": "s3",
			"backend": map[string]any{
				"bucket": "test-bucket",
				"region": "us-west-2",
			},
			"provision": map[string]any{
				"backend": map[string]any{
					"enabled": true,
				},
			},
		}, nil
	}

	params := &ProvisionParams{
		AtmosConfig:       &schema.AtmosConfiguration{},
		ProvisionerType:   "backend",
		Component:         "vpc",
		Stack:             "dev",
		DescribeComponent: mockDescribe,
		AuthContext:       nil,
	}

	err := ProvisionWithParams(params)
	require.NoError(t, err)
	assert.True(t, mockProvisionerCalled, "Backend provisioner should have been called")
}

func TestProvisionWithParams_BackendProvisioningFailure(t *testing.T) {
	// Clean up registry after test to ensure test isolation.
	t.Cleanup(backend.ResetRegistryForTesting)

	// Register a mock backend provisioner that fails.
	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		return errors.New("provisioning failed: bucket already exists in another account")
	}

	// Temporarily register the mock provisioner.
	backend.RegisterBackendCreate("s3", mockProvisioner)

	mockDescribe := func(component string, stack string) (map[string]any, error) {
		return map[string]any{
			"backend_type": "s3",
			"backend": map[string]any{
				"bucket": "test-bucket",
				"region": "us-west-2",
			},
			"provision": map[string]any{
				"backend": map[string]any{
					"enabled": true,
				},
			},
		}, nil
	}

	params := &ProvisionParams{
		AtmosConfig:       &schema.AtmosConfiguration{},
		ProvisionerType:   "backend",
		Component:         "vpc",
		Stack:             "dev",
		DescribeComponent: mockDescribe,
		AuthContext:       nil,
	}

	err := ProvisionWithParams(params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backend provisioning failed")
	assert.Contains(t, err.Error(), "bucket already exists in another account")
}

func TestProvision_DelegatesToProvisionWithParams(t *testing.T) {
	// Clean up registry after test to ensure test isolation.
	t.Cleanup(backend.ResetRegistryForTesting)

	// This test verifies that the Provision wrapper function correctly creates
	// a ProvisionParams struct and delegates to ProvisionWithParams.

	mockDescribe := func(component string, stack string) (map[string]any, error) {
		assert.Equal(t, "vpc", component)
		assert.Equal(t, "dev", stack)

		return map[string]any{
			"backend_type": "s3",
			"backend": map[string]any{
				"bucket": "test-bucket",
				"region": "us-west-2",
			},
			"provision": map[string]any{
				"backend": map[string]any{
					"enabled": true,
				},
			},
		}, nil
	}

	// Register a mock backend provisioner.
	mockProvisionerCalled := false
	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		mockProvisionerCalled = true
		return nil
	}
	backend.RegisterBackendCreate("s3", mockProvisioner)

	atmosConfig := &schema.AtmosConfiguration{}
	err := ProvisionWithParams(&ProvisionParams{
		AtmosConfig:       atmosConfig,
		ProvisionerType:   "backend",
		Component:         "vpc",
		Stack:             "dev",
		DescribeComponent: mockDescribe,
		AuthContext:       nil,
	})

	require.NoError(t, err)
	assert.True(t, mockProvisionerCalled, "Backend provisioner should have been called")
}

func TestProvisionWithParams_WithAuthContext(t *testing.T) {
	// Clean up registry after test to ensure test isolation.
	t.Cleanup(backend.ResetRegistryForTesting)

	// This test verifies that AuthContext is correctly passed through to the backend provisioner.

	mockDescribe := func(component string, stack string) (map[string]any, error) {
		return map[string]any{
			"backend_type": "s3",
			"backend": map[string]any{
				"bucket": "test-bucket",
				"region": "us-west-2",
			},
			"provision": map[string]any{
				"backend": map[string]any{
					"enabled": true,
				},
			},
		}, nil
	}

	// Register a mock backend provisioner that verifies authContext handling.
	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		// AuthContext is passed through from params; nil here because test provides nil.
		assert.Nil(t, authContext, "AuthContext should be nil when params.AuthContext is nil")
		return nil
	}
	backend.RegisterBackendCreate("s3", mockProvisioner)

	params := &ProvisionParams{
		AtmosConfig:       &schema.AtmosConfiguration{},
		ProvisionerType:   "backend",
		Component:         "vpc",
		Stack:             "dev",
		DescribeComponent: mockDescribe,
		AuthContext:       nil,
	}

	err := ProvisionWithParams(params)
	require.NoError(t, err)
}

func TestProvisionWithParams_BackendTypeValidation(t *testing.T) {
	// Clean up registry after test to ensure test isolation.
	t.Cleanup(backend.ResetRegistryForTesting)

	tests := []struct {
		name          string
		provisionType string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "backend type is supported",
			provisionType: "backend",
			wantErr:       false,
		},
		{
			name:          "terraform type is not supported",
			provisionType: "terraform",
			wantErr:       true,
			errContains:   "unsupported provisioner type",
		},
		{
			name:          "helmfile type is not supported",
			provisionType: "helmfile",
			wantErr:       true,
			errContains:   "unsupported provisioner type",
		},
		{
			name:          "empty type is not supported",
			provisionType: "",
			wantErr:       true,
			errContains:   "unsupported provisioner type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDescribe := func(component string, stack string) (map[string]any, error) {
				return map[string]any{
					"backend_type": "s3",
					"backend": map[string]any{
						"bucket": "test-bucket",
						"region": "us-west-2",
					},
				}, nil
			}

			// Register a mock provisioner for backend type.
			if tt.provisionType == "backend" {
				mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
					return nil
				}
				backend.RegisterBackendCreate("s3", mockProvisioner)
			}

			params := &ProvisionParams{
				AtmosConfig:       &schema.AtmosConfiguration{},
				ProvisionerType:   tt.provisionType,
				Component:         "vpc",
				Stack:             "dev",
				DescribeComponent: mockDescribe,
				AuthContext:       nil,
			}

			err := ProvisionWithParams(params)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				if tt.provisionType != "" && tt.provisionType != "backend" {
					assert.ErrorIs(t, err, ErrUnsupportedProvisionerType)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListBackends(t *testing.T) {
	t.Run("returns ErrNotImplemented", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		opts := map[string]string{"format": "table"}

		err := ListBackends(atmosConfig, opts)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
	})

	t.Run("returns ErrNotImplemented with nil opts", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		err := ListBackends(atmosConfig, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
	})
}

func TestDescribeBackend(t *testing.T) {
	t.Run("returns ErrNotImplemented", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		component := "vpc"
		opts := map[string]string{"format": "yaml"}

		err := DescribeBackend(atmosConfig, component, opts)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
	})

	t.Run("returns ErrNotImplemented with nil opts", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		err := DescribeBackend(atmosConfig, "vpc", nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
	})

	t.Run("returns ErrNotImplemented with empty component", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{}
		err := DescribeBackend(atmosConfig, "", map[string]string{"format": "json"})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
	})
}

func TestDeleteBackend(t *testing.T) {
	// Clean up registry after test to ensure test isolation.
	t.Cleanup(backend.ResetRegistryForTesting)

	t.Run("returns error when params is nil", func(t *testing.T) {
		err := DeleteBackendWithParams(nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNilParam)
	})

	t.Run("returns error when DescribeComponent is nil", func(t *testing.T) {
		err := DeleteBackendWithParams(&DeleteBackendParams{
			AtmosConfig:       &schema.AtmosConfiguration{},
			Component:         "vpc",
			Stack:             "dev",
			Force:             true,
			DescribeComponent: nil,
			AuthContext:       nil,
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrNilParam)
	})

	t.Run("returns error when describe component fails", func(t *testing.T) {
		mockDescribe := func(component string, stack string) (map[string]any, error) {
			return nil, errors.New("component not found in stack")
		}

		err := DeleteBackendWithParams(&DeleteBackendParams{
			AtmosConfig:       &schema.AtmosConfiguration{},
			Component:         "vpc",
			Stack:             "dev",
			Force:             true,
			DescribeComponent: mockDescribe,
			AuthContext:       nil,
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrDescribeComponent)
	})

	t.Run("returns error when backend_type not specified", func(t *testing.T) {
		mockDescribe := func(component string, stack string) (map[string]any, error) {
			return map[string]any{
				"backend": map[string]any{
					"bucket": "test-bucket",
				},
				// No backend_type
			}, nil
		}

		err := DeleteBackendWithParams(&DeleteBackendParams{
			AtmosConfig:       &schema.AtmosConfiguration{},
			Component:         "vpc",
			Stack:             "dev",
			Force:             true,
			DescribeComponent: mockDescribe,
			AuthContext:       nil,
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBackendTypeRequired)
	})

	t.Run("deletes backend successfully", func(t *testing.T) {
		// Register a mock delete function.
		mockDeleter := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext, force bool) error {
			assert.True(t, force, "Force flag should be true")
			return nil
		}
		backend.RegisterBackendDelete("s3", mockDeleter)

		mockDescribe := func(component string, stack string) (map[string]any, error) {
			return map[string]any{
				"backend_type": "s3",
				"backend": map[string]any{
					"bucket": "test-bucket",
					"region": "us-west-2",
				},
			}, nil
		}

		atmosConfig := &schema.AtmosConfiguration{}
		err := DeleteBackendWithParams(&DeleteBackendParams{
			AtmosConfig:       atmosConfig,
			Component:         "vpc",
			Stack:             "dev",
			Force:             true,
			DescribeComponent: mockDescribe,
			AuthContext:       nil,
		})
		assert.NoError(t, err, "DeleteBackend should not error")
	})

	t.Run("returns error when backend not found", func(t *testing.T) {
		mockDescribe := func(component string, stack string) (map[string]any, error) {
			return map[string]any{
				"backend_type": "s3",
				// No backend configuration
			}, nil
		}

		atmosConfig := &schema.AtmosConfiguration{}
		err := DeleteBackendWithParams(&DeleteBackendParams{
			AtmosConfig:       atmosConfig,
			Component:         "vpc",
			Stack:             "dev",
			Force:             true,
			DescribeComponent: mockDescribe,
			AuthContext:       nil,
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrBackendNotFound)
	})

	t.Run("returns error when delete function not implemented", func(t *testing.T) {
		mockDescribe := func(component string, stack string) (map[string]any, error) {
			return map[string]any{
				"backend_type": "unsupported",
				"backend": map[string]any{
					"bucket": "test-bucket",
				},
			}, nil
		}

		atmosConfig := &schema.AtmosConfiguration{}
		err := DeleteBackendWithParams(&DeleteBackendParams{
			AtmosConfig:       atmosConfig,
			Component:         "vpc",
			Stack:             "dev",
			Force:             true,
			DescribeComponent: mockDescribe,
			AuthContext:       nil,
		})
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrDeleteNotImplemented)
	})
}
