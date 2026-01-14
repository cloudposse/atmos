package backend

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resetBackendRegistry clears the backend provisioner registry for testing.
func resetBackendRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	backendCreators = make(map[string]BackendCreateFunc)
}

func TestRegisterBackendCreate(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		return nil
	}

	RegisterBackendCreate("s3", mockProvisioner)

	provisioner := GetBackendCreate("s3")
	assert.NotNil(t, provisioner)
}

func TestGetBackendCreate_NotFound(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	provisioner := GetBackendCreate("nonexistent")
	assert.Nil(t, provisioner)
}

func TestGetBackendCreate_MultipleTypes(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	s3Provisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		return nil
	}

	gcsProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		return nil
	}

	RegisterBackendCreate("s3", s3Provisioner)
	RegisterBackendCreate("gcs", gcsProvisioner)

	assert.NotNil(t, GetBackendCreate("s3"))
	assert.NotNil(t, GetBackendCreate("gcs"))
	assert.Nil(t, GetBackendCreate("azurerm"))
}

func TestRegisterBackendDelete(t *testing.T) {
	// Reset registry before test.
	ResetRegistryForTesting()
	t.Cleanup(ResetRegistryForTesting)

	mockDeleter := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext, force bool) error {
		return nil
	}

	RegisterBackendDelete("s3", mockDeleter)

	deleter := GetBackendDelete("s3")
	assert.NotNil(t, deleter)
}

func TestGetBackendDelete_NotFound(t *testing.T) {
	// Reset registry before test.
	ResetRegistryForTesting()
	t.Cleanup(ResetRegistryForTesting)

	deleter := GetBackendDelete("nonexistent")
	assert.Nil(t, deleter)
}

func TestGetBackendDelete_MultipleTypes(t *testing.T) {
	// Reset registry before test.
	ResetRegistryForTesting()
	t.Cleanup(ResetRegistryForTesting)

	s3Deleter := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext, force bool) error {
		return nil
	}

	gcsDeleter := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext, force bool) error {
		return nil
	}

	RegisterBackendDelete("s3", s3Deleter)
	RegisterBackendDelete("gcs", gcsDeleter)

	assert.NotNil(t, GetBackendDelete("s3"))
	assert.NotNil(t, GetBackendDelete("gcs"))
	assert.Nil(t, GetBackendDelete("azurerm"))
}

func TestResetRegistryForTesting(t *testing.T) {
	// Register some functions first.
	mockCreator := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		return nil
	}
	mockDeleter := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext, force bool) error {
		return nil
	}

	RegisterBackendCreate("test-backend", mockCreator)
	RegisterBackendDelete("test-backend", mockDeleter)

	// Verify they're registered.
	assert.NotNil(t, GetBackendCreate("test-backend"))
	assert.NotNil(t, GetBackendDelete("test-backend"))

	// Reset the registry.
	ResetRegistryForTesting()

	// Verify they're cleared.
	assert.Nil(t, GetBackendCreate("test-backend"))
	assert.Nil(t, GetBackendDelete("test-backend"))
}

func TestResetRegistryForTesting_ClearsAllEntries(t *testing.T) {
	// Reset at start.
	ResetRegistryForTesting()

	mockCreator := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		return nil
	}
	mockDeleter := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext, force bool) error {
		return nil
	}

	// Register multiple backends.
	RegisterBackendCreate("s3", mockCreator)
	RegisterBackendCreate("gcs", mockCreator)
	RegisterBackendCreate("azurerm", mockCreator)
	RegisterBackendDelete("s3", mockDeleter)
	RegisterBackendDelete("gcs", mockDeleter)

	// Verify all are registered.
	assert.NotNil(t, GetBackendCreate("s3"))
	assert.NotNil(t, GetBackendCreate("gcs"))
	assert.NotNil(t, GetBackendCreate("azurerm"))
	assert.NotNil(t, GetBackendDelete("s3"))
	assert.NotNil(t, GetBackendDelete("gcs"))

	// Reset.
	ResetRegistryForTesting()

	// Verify all are cleared.
	assert.Nil(t, GetBackendCreate("s3"))
	assert.Nil(t, GetBackendCreate("gcs"))
	assert.Nil(t, GetBackendCreate("azurerm"))
	assert.Nil(t, GetBackendDelete("s3"))
	assert.Nil(t, GetBackendDelete("gcs"))
}

func TestProvisionBackend_NoProvisioningConfiguration(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Component config without provision block.
	componentConfig := map[string]any{
		"backend_type": "s3",
		"backend": map[string]any{
			"bucket": "test-bucket",
			"region": "us-west-2",
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err, "Should return nil when no provisioning configuration exists")
}

func TestProvisionBackend_NoBackendProvisioningConfiguration(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Component config with provision block but no backend sub-block.
	componentConfig := map[string]any{
		"backend_type": "s3",
		"backend": map[string]any{
			"bucket": "test-bucket",
			"region": "us-west-2",
		},
		"provision": map[string]any{
			"other": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err, "Should return nil when no backend provisioning configuration exists")
}

func TestProvisionBackend_ProvisioningDisabled(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Component config with provisioning explicitly disabled.
	componentConfig := map[string]any{
		"backend_type": "s3",
		"backend": map[string]any{
			"bucket": "test-bucket",
			"region": "us-west-2",
		},
		"provision": map[string]any{
			"backend": map[string]any{
				"enabled": false,
			},
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err, "Should return nil when provisioning is disabled")
}

func TestProvisionBackend_ProvisioningEnabledMissingField(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Component config with backend block but no enabled field (defaults to false).
	componentConfig := map[string]any{
		"backend_type": "s3",
		"backend": map[string]any{
			"bucket": "test-bucket",
			"region": "us-west-2",
		},
		"provision": map[string]any{
			"backend": map[string]any{},
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err, "Should return nil when enabled field is missing")
}

func TestProvisionBackend_MissingBackendConfiguration(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Component config with provisioning enabled but no backend configuration.
	componentConfig := map[string]any{
		"backend_type": "s3",
		"provision": map[string]any{
			"backend": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrBackendNotFound)
	assert.Contains(t, err.Error(), "backend configuration not found")
}

func TestProvisionBackend_MissingBackendType(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Component config with provisioning enabled but no backend_type.
	componentConfig := map[string]any{
		"backend": map[string]any{
			"bucket": "test-bucket",
			"region": "us-west-2",
		},
		"provision": map[string]any{
			"backend": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrBackendTypeRequired)
	assert.Contains(t, err.Error(), "backend_type not specified")
}

func TestProvisionBackend_UnsupportedBackendType(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	// Component config with unsupported backend type.
	componentConfig := map[string]any{
		"backend_type": "unsupported",
		"backend": map[string]any{
			"bucket": "test-bucket",
		},
		"provision": map[string]any{
			"backend": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCreateNotImplemented)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestProvisionBackend_Success(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	provisionerCalled := false
	var capturedBackendConfig map[string]any
	var capturedAuthContext *schema.AuthContext

	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		provisionerCalled = true
		capturedBackendConfig = backendConfig
		capturedAuthContext = authContext
		return nil
	}

	RegisterBackendCreate("s3", mockProvisioner)

	componentConfig := map[string]any{
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
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err)
	assert.True(t, provisionerCalled, "Provisioner should have been called")
	assert.NotNil(t, capturedBackendConfig)
	assert.Equal(t, "test-bucket", capturedBackendConfig["bucket"])
	assert.Equal(t, "us-west-2", capturedBackendConfig["region"])
	assert.Nil(t, capturedAuthContext)
}

func TestProvisionBackend_WithAuthContext(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	var capturedAuthContext *schema.AuthContext

	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		capturedAuthContext = authContext
		return nil
	}

	RegisterBackendCreate("s3", mockProvisioner)

	componentConfig := map[string]any{
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
	}

	authContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-profile",
			Region:  "us-west-2",
		},
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, authContext)
	require.NoError(t, err)
	require.NotNil(t, capturedAuthContext)
	require.NotNil(t, capturedAuthContext.AWS)
	assert.Equal(t, "test-profile", capturedAuthContext.AWS.Profile)
	assert.Equal(t, "us-west-2", capturedAuthContext.AWS.Region)
}

func TestProvisionBackend_ProvisionerFailure(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		return errors.New("bucket creation failed: permission denied")
	}

	RegisterBackendCreate("s3", mockProvisioner)

	componentConfig := map[string]any{
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
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket creation failed")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestProvisionBackend_MultipleBackendTypes(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	s3Called := false
	gcsCalled := false

	mockS3Provisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		s3Called = true
		return nil
	}

	mockGCSProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		gcsCalled = true
		return nil
	}

	RegisterBackendCreate("s3", mockS3Provisioner)
	RegisterBackendCreate("gcs", mockGCSProvisioner)

	// Test S3 backend.
	componentConfigS3 := map[string]any{
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
	}

	err := ProvisionBackend(ctx, atmosConfig, componentConfigS3, nil)
	require.NoError(t, err)
	assert.True(t, s3Called, "S3 provisioner should have been called")
	assert.False(t, gcsCalled, "GCS provisioner should not have been called")

	// Reset flags.
	s3Called = false
	gcsCalled = false

	// Test GCS backend.
	componentConfigGCS := map[string]any{
		"backend_type": "gcs",
		"backend": map[string]any{
			"bucket": "test-bucket",
			"prefix": "terraform/state",
		},
		"provision": map[string]any{
			"backend": map[string]any{
				"enabled": true,
			},
		},
	}

	err = ProvisionBackend(ctx, atmosConfig, componentConfigGCS, nil)
	require.NoError(t, err)
	assert.False(t, s3Called, "S3 provisioner should not have been called")
	assert.True(t, gcsCalled, "GCS provisioner should have been called")
}

func TestConcurrentBackendProvisioning(t *testing.T) {
	// Reset registry before test.
	resetBackendRegistry()

	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	var callCount int
	var mu sync.Mutex

	mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	}

	RegisterBackendCreate("s3", mockProvisioner)

	// Base config template - each goroutine will get its own copy.
	baseComponentConfig := map[string]any{
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
	}

	// Run 10 concurrent provisioning operations.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Create per-goroutine copy to avoid data race if ProvisionBackend mutates the map.
			componentConfig := map[string]any{
				"backend_type": baseComponentConfig["backend_type"],
				"backend":      baseComponentConfig["backend"],
				"provision":    baseComponentConfig["provision"],
			}
			err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	// Verify all 10 calls executed.
	assert.Equal(t, 10, callCount)
}

func TestProvisionBackend_EnabledWrongType(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name            string
		enabledValue    any
		shouldProvision bool
	}{
		{
			name:            "enabled is string 'true'",
			enabledValue:    "true",
			shouldProvision: false, // Type assertion fails, treated as not enabled
		},
		{
			name:            "enabled is int 1",
			enabledValue:    1,
			shouldProvision: false, // Type assertion fails, treated as not enabled
		},
		{
			name:            "enabled is true",
			enabledValue:    true,
			shouldProvision: true,
		},
		{
			name:            "enabled is false",
			enabledValue:    false,
			shouldProvision: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset registry before test.
			resetBackendRegistry()

			provisionerCalled := false
			mockProvisioner := func(ctx context.Context, atmosConfig *schema.AtmosConfiguration, backendConfig map[string]any, authContext *schema.AuthContext) error {
				provisionerCalled = true
				return nil
			}

			RegisterBackendCreate("s3", mockProvisioner)

			componentConfig := map[string]any{
				"backend_type": "s3",
				"backend": map[string]any{
					"bucket": "test-bucket",
					"region": "us-west-2",
				},
				"provision": map[string]any{
					"backend": map[string]any{
						"enabled": tt.enabledValue,
					},
				},
			}

			err := ProvisionBackend(ctx, atmosConfig, componentConfig, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.shouldProvision, provisionerCalled)
		})
	}
}
