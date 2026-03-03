package workdir

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestWorkdirProvisionerRegistration verifies that the workdir provisioner
// is registered with the correct hook event.
func TestWorkdirProvisionerRegistration(t *testing.T) {
	// The init() function should have registered the workdir provisioner.
	provisioners := provisioner.GetProvisionersForEvent(HookEventBeforeTerraformInit)

	// Find the workdir provisioner.
	var found bool
	for _, p := range provisioners {
		if p.Type == "workdir" {
			found = true
			assert.Equal(t, HookEventBeforeTerraformInit, p.HookEvent)
			assert.NotNil(t, p.Func)
			break
		}
	}

	assert.True(t, found, "workdir provisioner should be registered")
}

// TestProvisionWorkdir_NoActivation verifies that the provisioner does nothing
// when provision.workdir.enabled is not set.
func TestProvisionWorkdir_NoActivation(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	componentConfig := map[string]any{
		"component": "test-component",
	}

	err := ProvisionWorkdir(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	// Verify no workdir path was set.
	_, ok := componentConfig[WorkdirPathKey]
	assert.False(t, ok, "workdir path should not be set when not activated")
}

// TestProvisionWorkdir_WithProvisionWorkdirEnabled verifies that the provisioner
// activates when provision.workdir.enabled is true.
func TestProvisionWorkdir_WithProvisionWorkdirEnabled(t *testing.T) {
	// Create temp directories.
	tempDir := t.TempDir()
	componentsDir := filepath.Join(tempDir, "components", "terraform", "test-component")
	err := os.MkdirAll(componentsDir, 0o755)
	require.NoError(t, err)

	// Create a dummy terraform file.
	err = os.WriteFile(filepath.Join(componentsDir, "main.tf"), []byte("# test"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentConfig := map[string]any{
		"component":   "test-component",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	ctx := context.Background()
	err = ProvisionWorkdir(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	// Verify workdir path was set with exact stack-component naming.
	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	assert.True(t, ok, "workdir path should be set")
	expectedWorkdir := filepath.Join(tempDir, ".workdir", "terraform", "dev-test-component")
	assert.Equal(t, expectedWorkdir, workdirPath)

	// Verify the workdir was created.
	_, err = os.Stat(workdirPath)
	assert.NoError(t, err, "workdir should exist")

	// Verify the main.tf was copied.
	_, err = os.Stat(filepath.Join(workdirPath, "main.tf"))
	assert.NoError(t, err, "main.tf should be copied to workdir")

	// Verify metadata file was created in .atmos/ subdirectory with correct content.
	metadataPath := MetadataPath(workdirPath)
	metadataBytes, err := os.ReadFile(metadataPath)
	require.NoError(t, err, "metadata file should exist at %s", metadataPath)
	assert.Contains(t, string(metadataBytes), `"component": "test-component"`)
	assert.Contains(t, string(metadataBytes), `"stack": "dev"`)
	assert.Contains(t, string(metadataBytes), `"source_type": "local"`)
}

// TestService_Provision_WithMockFileSystem tests the Service using mocked dependencies.
// Note: Metadata writing now uses WriteMetadata which bypasses the mocked FileSystem,
// so we only test the file system operations that use the injected mock.
func TestService_Provision_WithMockFileSystem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockHasher)

	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	// Expected workdir uses stack-component naming (dev-vpc).
	expectedWorkdir := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc")
	componentPath := filepath.Join(tempDir, "components", "terraform", "vpc")

	// Create the workdir directory so that WriteMetadata can create .atmos/ inside it.
	err := os.MkdirAll(expectedWorkdir, 0o755)
	require.NoError(t, err)

	// Set up mock expectations.
	// SyncDir is now used instead of CopyDir for incremental sync.
	mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(componentPath).Return(true)
	mockFS.EXPECT().SyncDir(componentPath, expectedWorkdir, mockHasher).Return(true, nil)
	mockHasher.EXPECT().HashDir(expectedWorkdir).Return("abc123", nil)
	// Note: WriteMetadata uses real filesystem operations (atomic write),
	// not the mocked FileSystem, so no WriteFile expectation needed.

	ctx := context.Background()
	err = service.Provision(ctx, atmosConfig, componentConfig)
	require.NoError(t, err)

	// Verify workdir path was set with stack-component naming.
	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	assert.True(t, ok)
	assert.Equal(t, expectedWorkdir, workdirPath)

	// Verify metadata was written to the correct location.
	metadataPath := MetadataPath(expectedWorkdir)
	_, err = os.Stat(metadataPath)
	assert.NoError(t, err, "metadata file should exist at %s", metadataPath)
}

// TestService_Provision_ErrorPaths tests error handling in Service.Provision.
func TestService_Provision_ErrorPaths(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*MockFileSystem, *MockHasher, string, string)
		expectedError string
	}{
		{
			name: "MkdirAll fails",
			setupMocks: func(mockFS *MockFileSystem, _ *MockHasher, expectedWorkdir, _ string) {
				mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(os.ErrPermission)
			},
			expectedError: "permission denied",
		},
		{
			name: "SyncDir fails",
			setupMocks: func(mockFS *MockFileSystem, mockHasher *MockHasher, expectedWorkdir, componentPath string) {
				mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
				mockFS.EXPECT().Exists(componentPath).Return(true)
				mockFS.EXPECT().SyncDir(componentPath, expectedWorkdir, mockHasher).Return(false, os.ErrNotExist)
			},
			expectedError: "file does not exist",
		},
	}
	// Note: HashDir failure is handled gracefully with a warning, not an error.
	// The implementation continues to write metadata even if hash computation fails.
	// Note: WriteMetadata errors are returned, but since it uses real filesystem
	// operations (atomic write), we can't easily mock those failures.

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFS := NewMockFileSystem(ctrl)
			mockHasher := NewMockHasher(ctrl)
			service := NewServiceWithDeps(mockFS, mockHasher)

			tempDir := t.TempDir()
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: tempDir,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			}

			componentConfig := map[string]any{
				"component":   "vpc",
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			}

			expectedWorkdir := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc")
			componentPath := filepath.Join(tempDir, "components", "terraform", "vpc")

			tt.setupMocks(mockFS, mockHasher, expectedWorkdir, componentPath)

			err := service.Provision(context.Background(), atmosConfig, componentConfig)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}

// TestService_Provision_EdgeCases tests edge cases and invalid inputs.
func TestService_Provision_EdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expectError     bool
	}{
		{
			name: "missing component name",
			componentConfig: map[string]any{
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectError: true,
		},
		{
			name: "missing stack name",
			componentConfig: map[string]any{
				"component": "vpc",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectError: true,
		},
		{
			name:            "nil componentConfig fields",
			componentConfig: map[string]any{},
			expectError:     false, // Should not activate without provision.workdir.enabled
		},
		{
			name: "workdir not enabled",
			componentConfig: map[string]any{
				"component":   "vpc",
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": false,
					},
				},
			},
			expectError: false, // Should skip without error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: t.TempDir(),
			}

			err := ProvisionWorkdir(ctx, atmosConfig, tt.componentConfig, nil)

			if tt.expectError {
				require.Error(t, err)
				// Verify it's the expected error type.
				assert.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestConcurrentProvisioning tests that concurrent provisioning of different
// components doesn't cause race conditions or data corruption.
func TestConcurrentProvisioning(t *testing.T) {
	tempDir := t.TempDir()

	// Create source component directories.
	components := []string{"vpc", "rds", "s3-bucket", "lambda", "api-gateway"}
	for _, comp := range components {
		compDir := filepath.Join(tempDir, "components", "terraform", comp)
		err := os.MkdirAll(compDir, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(compDir, "main.tf"), []byte("# "+comp), 0o644)
		require.NoError(t, err)
	}

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Run provisioning concurrently for all components.
	var wg sync.WaitGroup
	errors := make(chan error, len(components))

	for _, comp := range components {
		wg.Add(1)
		go func(component string) {
			defer wg.Done()

			componentConfig := map[string]any{
				"component":   component,
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			}

			ctx := context.Background()
			if err := ProvisionWorkdir(ctx, atmosConfig, componentConfig, nil); err != nil {
				errors <- fmt.Errorf("component %s: %w", component, err)
				return
			}

			// Verify workdir was created with correct naming.
			workdirPath, ok := componentConfig[WorkdirPathKey].(string)
			if !ok {
				errors <- fmt.Errorf("component %s: workdir path not set", component)
				return
			}

			expectedName := "dev-" + component
			if !strings.Contains(workdirPath, expectedName) {
				errors <- fmt.Errorf("component %s: expected path to contain %s, got %s", component, expectedName, workdirPath)
				return
			}
		}(comp)
	}

	wg.Wait()
	close(errors)

	// Collect and report any errors.
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "concurrent provisioning errors: %v", errs)

	// Verify all workdirs were created independently.
	for _, comp := range components {
		workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-"+comp)
		_, err := os.Stat(workdirPath)
		assert.NoError(t, err, "workdir for %s should exist", comp)

		// Verify content is correct (no corruption from other goroutines).
		content, err := os.ReadFile(filepath.Join(workdirPath, "main.tf"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "# "+comp, "workdir content should match source component")
	}
}

// TestComponentInstancesWithSameBaseComponent tests that multiple component instances
// sharing the same base component (metadata.component) get unique workdirs.
// This test validates the fix for parallel apply of component instances.
func TestComponentInstancesWithSameBaseComponent(t *testing.T) {
	tempDir := t.TempDir()

	// Create a single elasticache component directory that will be shared by all instances.
	elasticacheDir := filepath.Join(tempDir, "components", "terraform", "elasticache")
	err := os.MkdirAll(elasticacheDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(elasticacheDir, "main.tf"), []byte("# elasticache component"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Create component configs for multiple instances, all sharing metadata.component: elasticache
	// but with different atmos_component values (the full instance paths).
	componentInstances := []struct {
		atmosComponent string
		baseComponent  string
	}{
		{
			atmosComponent: "elasticache-redis-cluster-1",
			baseComponent:  "elasticache",
		},
		{
			atmosComponent: "elasticache-redis-cluster-2",
			baseComponent:  "elasticache",
		},
		{
			atmosComponent: "elasticache-redis-cluster-3",
			baseComponent:  "elasticache",
		},
	}

	ctx := context.Background()
	workdirPaths := make(map[string]string)

	// Provision workdirs for all instances.
	// NOTE: No explicit "component_path" is set here. The fix ensures that extractComponentPath()
	// uses the base component name (from "component" key) to build the correct source path,
	// rather than the atmos_component (instance name) which would point to a non-existent directory.
	for _, instance := range componentInstances {
		componentConfig := map[string]any{
			"atmos_component": instance.atmosComponent,
			"component":       instance.baseComponent,
			"atmos_stack":     "dev",
			"metadata": map[string]any{
				"component": instance.baseComponent,
			},
			"provision": map[string]any{
				"workdir": map[string]any{
					"enabled": true,
				},
			},
		}

		err := ProvisionWorkdir(ctx, atmosConfig, componentConfig, nil)
		require.NoError(t, err, "provisioning should succeed for %s", instance.atmosComponent)

		// Verify workdir path was set.
		workdirPath, ok := componentConfig[WorkdirPathKey].(string)
		require.True(t, ok, "workdir path should be set for %s", instance.atmosComponent)

		// Store the workdir path for later verification.
		workdirPaths[instance.atmosComponent] = workdirPath

		// Verify the workdir uses the atmos_component name (instance name), not the base component.
		expectedName := "dev-" + instance.atmosComponent
		assert.Contains(t, workdirPath, expectedName,
			"workdir path should contain %s, got %s", expectedName, workdirPath)

		// Verify the workdir was created.
		_, err = os.Stat(workdirPath)
		assert.NoError(t, err, "workdir should exist for %s", instance.atmosComponent)

		// Verify the main.tf was copied.
		_, err = os.Stat(filepath.Join(workdirPath, "main.tf"))
		assert.NoError(t, err, "main.tf should be copied to workdir for %s", instance.atmosComponent)
	}

	// Verify all workdirs are unique (no collisions).
	uniquePaths := make(map[string]bool)
	for _, path := range workdirPaths {
		require.False(t, uniquePaths[path], "workdir paths must be unique, found duplicate: %s", path)
		uniquePaths[path] = true
	}

	// Verify we got exactly 3 unique workdirs.
	assert.Len(t, uniquePaths, 3, "should have 3 unique workdir paths")

	// Verify all expected workdirs exist.
	for _, instance := range componentInstances {
		expectedWorkdir := filepath.Join(tempDir, ".workdir", "terraform", "dev-"+instance.atmosComponent)
		_, err := os.Stat(expectedWorkdir)
		assert.NoError(t, err, "expected workdir should exist at %s", expectedWorkdir)

		// Verify metadata contains the correct component instance name.
		metadataPath := MetadataPath(expectedWorkdir)
		metadataBytes, err := os.ReadFile(metadataPath)
		require.NoError(t, err, "metadata file should exist")
		assert.Contains(t, string(metadataBytes), fmt.Sprintf(`"component": "%s"`, instance.atmosComponent),
			"metadata should contain the component instance name")
	}
}

// TestCleanWorkdir tests the CleanWorkdir function with stack-component naming.
func TestCleanWorkdir(t *testing.T) {
	tempDir := t.TempDir()

	// Create a workdir structure using stack-component naming.
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-test-component")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	// Create a file in the workdir.
	err = os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# test"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Clean the workdir using component and stack.
	err = CleanWorkdir(atmosConfig, "test-component", "dev")
	require.NoError(t, err)

	// Verify the workdir was removed.
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err), "workdir should be removed")
}

// TestCleanAllWorkdirs tests the CleanAllWorkdirs function.
func TestCleanAllWorkdirs(t *testing.T) {
	tempDir := t.TempDir()

	// Create multiple workdir structures.
	workdir1 := filepath.Join(tempDir, ".workdir", "terraform", "component1")
	workdir2 := filepath.Join(tempDir, ".workdir", "terraform", "component2")
	err := os.MkdirAll(workdir1, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(workdir2, 0o755)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Clean all workdirs.
	err = CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err)

	// Verify all workdirs were removed.
	workdirBase := filepath.Join(tempDir, ".workdir")
	_, err = os.Stat(workdirBase)
	assert.True(t, os.IsNotExist(err), "all workdirs should be removed")
}

// TestCleanAllWorkdirs_NoWorkdirsExist tests CleanAllWorkdirs when no workdir directory exists.
func TestCleanAllWorkdirs_NoWorkdirsExist(t *testing.T) {
	tempDir := t.TempDir()

	// Don't create any .workdir directory - it shouldn't exist.
	workdirBase := filepath.Join(tempDir, ".workdir")
	_, err := os.Stat(workdirBase)
	require.True(t, os.IsNotExist(err), "precondition: .workdir should not exist")

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Clean all workdirs should succeed gracefully when no workdirs exist.
	err = CleanAllWorkdirs(atmosConfig)
	require.NoError(t, err, "CleanAllWorkdirs should handle non-existent .workdir gracefully")
}

// TestAtmosComponentPriority_OverridesBaseComponent verifies the core fix:
// atmos_component takes priority over component/metadata.component for workdir path.
func TestAtmosComponentPriority_OverridesBaseComponent(t *testing.T) {
	tempDir := t.TempDir()

	// Create the shared base component directory.
	baseDir := filepath.Join(tempDir, "components", "terraform", "s3-bucket")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "main.tf"), []byte("# s3"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// atmos_component should win over component and metadata.component for workdir naming.
	// The source path should be resolved from the base component ("s3-bucket"), not the instance name.
	componentConfig := map[string]any{
		"atmos_component": "s3-bucket-logs",
		"component":       "s3-bucket",
		"atmos_stack":     "prod",
		"metadata": map[string]any{
			"component": "s3-bucket",
		},
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionWorkdir(context.Background(), atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	workdirPath := componentConfig[WorkdirPathKey].(string)
	// Must use atmos_component (s3-bucket-logs), NOT the base component (s3-bucket).
	expectedSuffix := filepath.Join("terraform", "prod-s3-bucket-logs")
	assert.True(t, strings.HasSuffix(workdirPath, expectedSuffix),
		"workdir path %s should end with %s", workdirPath, expectedSuffix)
}

// TestAtmosComponentPriority_FallsBackToComponentKey verifies that when
// atmos_component is absent, component key is used (backward compatibility).
func TestAtmosComponentPriority_FallsBackToComponentKey(t *testing.T) {
	tempDir := t.TempDir()

	compDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(compDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(compDir, "main.tf"), []byte("# vpc"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// No atmos_component — should fall back to component key.
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "staging",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionWorkdir(context.Background(), atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	workdirPath := componentConfig[WorkdirPathKey].(string)
	assert.Contains(t, workdirPath, "staging-vpc")
}

// TestAtmosComponentPriority_EmptyStringFallback verifies that an empty
// atmos_component string falls back to extractComponentName.
func TestAtmosComponentPriority_EmptyStringFallback(t *testing.T) {
	tempDir := t.TempDir()

	compDir := filepath.Join(tempDir, "components", "terraform", "rds")
	require.NoError(t, os.MkdirAll(compDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(compDir, "main.tf"), []byte("# rds"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// atmos_component is empty string — should fall back to component key.
	componentConfig := map[string]any{
		"atmos_component": "",
		"component":       "rds",
		"atmos_stack":     "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionWorkdir(context.Background(), atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	workdirPath := componentConfig[WorkdirPathKey].(string)
	assert.Contains(t, workdirPath, "dev-rds")
}

// TestAtmosComponentPriority_NonStringFallback verifies that when
// atmos_component is not a string type, it falls back to component key.
func TestAtmosComponentPriority_NonStringFallback(t *testing.T) {
	tempDir := t.TempDir()
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-lambda")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)
	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentPath := filepath.Join(tempDir, "components", "terraform", "lambda")

	mockFS.EXPECT().MkdirAll(workdirPath, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(componentPath).Return(true)
	mockFS.EXPECT().SyncDir(componentPath, workdirPath, mockHasher).Return(false, nil)
	mockHasher.EXPECT().HashDir(workdirPath).Return("hash", nil)

	// atmos_component is an int — should fall back to component key.
	componentConfig := map[string]any{
		"atmos_component": 42,
		"component":       "lambda",
		"atmos_stack":     "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := service.Provision(context.Background(), atmosConfig, componentConfig)
	require.NoError(t, err)
	assert.Contains(t, componentConfig[WorkdirPathKey], "dev-lambda")
}

// TestConcurrentComponentInstances tests parallel provisioning of component
// instances sharing the same base component (the core use case from issue #2091).
func TestConcurrentComponentInstances(t *testing.T) {
	tempDir := t.TempDir()

	// Create a single shared base component.
	baseDir := filepath.Join(tempDir, "components", "terraform", "elasticache")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "main.tf"), []byte("# elasticache"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	instances := []string{
		"elasticache-redis-cluster-1",
		"elasticache-redis-cluster-2",
		"elasticache-redis-cluster-3",
		"elasticache-redis-cluster-4",
		"elasticache-redis-cluster-5",
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(instances))
	pathCh := make(chan string, len(instances))

	for _, inst := range instances {
		wg.Add(1)
		go func(instanceName string) {
			defer wg.Done()

			// NOTE: No explicit "component_path" is set. The fix ensures source path
			// is resolved from "component" key (base component), not atmos_component (instance).
			componentConfig := map[string]any{
				"atmos_component": instanceName,
				"component":       "elasticache",
				"atmos_stack":     "prod",
				"metadata": map[string]any{
					"component": "elasticache",
				},
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			}

			ctx := context.Background()
			if err := ProvisionWorkdir(ctx, atmosConfig, componentConfig, nil); err != nil {
				errCh <- fmt.Errorf("instance %s: %w", instanceName, err)
				return
			}

			wdPath, ok := componentConfig[WorkdirPathKey].(string)
			if !ok {
				errCh <- fmt.Errorf("instance %s: workdir path not set", instanceName)
				return
			}

			// Verify instance name is in path (not base component name).
			if !strings.Contains(wdPath, "prod-"+instanceName) {
				errCh <- fmt.Errorf("instance %s: expected path with prod-%s, got %s", instanceName, instanceName, wdPath)
				return
			}

			pathCh <- wdPath
		}(inst)
	}

	wg.Wait()
	close(errCh)
	close(pathCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	require.Empty(t, errs, "concurrent provisioning errors: %v", errs)

	// Verify all paths are unique.
	paths := make(map[string]bool)
	for p := range pathCh {
		require.False(t, paths[p], "duplicate workdir path: %s", p)
		paths[p] = true
	}
	assert.Len(t, paths, len(instances), "should have %d unique workdir paths", len(instances))
}

// TestSyncDir_DeletesRemovedFiles verifies that SyncDir removes files from dest
// that no longer exist in source.
func TestSyncDir_DeletesRemovedFiles(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	dstDir := filepath.Join(t.TempDir(), "dst")

	// Initial state: both have file_a.tf and file_b.tf.
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file_a.tf"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file_b.tf"), []byte("b"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "file_a.tf"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "file_b.tf"), []byte("b"), 0o644))
	// Extra file in dst only (should be removed by sync).
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "old_file.tf"), []byte("old"), 0o644))

	fs := NewDefaultFileSystem()
	hasher := NewDefaultHasher()

	changed, err := fs.SyncDir(srcDir, dstDir, hasher)
	require.NoError(t, err)
	assert.True(t, changed, "sync should report changes when files are deleted")

	// old_file.tf should be gone.
	_, err = os.Stat(filepath.Join(dstDir, "old_file.tf"))
	assert.True(t, os.IsNotExist(err), "old_file.tf should be deleted")

	// Existing files should remain.
	_, err = os.Stat(filepath.Join(dstDir, "file_a.tf"))
	assert.NoError(t, err, "file_a.tf should still exist")
	_, err = os.Stat(filepath.Join(dstDir, "file_b.tf"))
	assert.NoError(t, err, "file_b.tf should still exist")
}

// TestSyncDir_NoChanges_Integration verifies that SyncDir returns false when files are identical.
func TestSyncDir_NoChanges_Integration(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	dstDir := filepath.Join(t.TempDir(), "dst")

	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("same"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dstDir, "main.tf"), []byte("same"), 0o644))

	fs := NewDefaultFileSystem()
	hasher := NewDefaultHasher()

	changed, err := fs.SyncDir(srcDir, dstDir, hasher)
	require.NoError(t, err)
	assert.False(t, changed, "sync should report no changes when files are identical")
}

// TestSyncDir_PreservesAtmosMetadata verifies that SyncDir preserves the .atmos/ metadata directory.
func TestSyncDir_PreservesAtmosMetadata(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "src")
	dstDir := filepath.Join(t.TempDir(), "dst")

	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.MkdirAll(dstDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("content"), 0o644))

	// Create .atmos/ metadata in dst (should not be deleted by sync).
	atmosDir := filepath.Join(dstDir, AtmosDir)
	require.NoError(t, os.MkdirAll(atmosDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(atmosDir, "metadata.json"), []byte("{}"), 0o644))

	fs := NewDefaultFileSystem()
	hasher := NewDefaultHasher()

	_, err := fs.SyncDir(srcDir, dstDir, hasher)
	require.NoError(t, err)

	// .atmos/metadata.json should still exist.
	_, err = os.Stat(filepath.Join(atmosDir, "metadata.json"))
	assert.NoError(t, err, ".atmos/metadata.json should be preserved during sync")
}

// TestFileNeedsCopy_PermissionChange verifies that permission changes are detected.
func TestFileNeedsCopy_PermissionChange(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.sh")
	dstFile := filepath.Join(tmpDir, "dst.sh")

	content := []byte("#!/bin/bash\necho hello")
	require.NoError(t, os.WriteFile(srcFile, content, 0o755)) // Executable.
	require.NoError(t, os.WriteFile(dstFile, content, 0o644)) // Not executable.

	hasher := NewDefaultHasher()

	// Same content but different permissions — should need copy.
	assert.True(t, fileNeedsCopy(srcFile, dstFile, hasher),
		"file should need copy when permissions differ")
}

// TestFileNeedsCopy_IdenticalFiles verifies that identical files don't need copy.
func TestFileNeedsCopy_IdenticalFiles(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.tf")
	dstFile := filepath.Join(tmpDir, "dst.tf")

	content := []byte("resource \"aws_instance\" \"main\" {}")
	require.NoError(t, os.WriteFile(srcFile, content, 0o644))
	require.NoError(t, os.WriteFile(dstFile, content, 0o644))

	hasher := NewDefaultHasher()

	assert.False(t, fileNeedsCopy(srcFile, dstFile, hasher),
		"identical files should not need copy")
}

// TestFileNeedsCopy_DestMissing verifies that missing destination triggers copy.
func TestFileNeedsCopy_DestMissing(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "src.tf")
	dstFile := filepath.Join(tmpDir, "nonexistent.tf")

	require.NoError(t, os.WriteFile(srcFile, []byte("content"), 0o644))

	hasher := NewDefaultHasher()

	assert.True(t, fileNeedsCopy(srcFile, dstFile, hasher),
		"missing destination should trigger copy")
}

// TestGetModTimeFromEntry_Error verifies the zero-value fallback.
func TestGetModTimeFromEntry_Error(t *testing.T) {
	result := getModTimeFromEntry(&errorDirEntry{})
	assert.True(t, result.IsZero(), "should return zero time on Info() error")
}

// errorDirEntry is a mock DirEntry that returns an error from Info().
type errorDirEntry struct{}

func (e *errorDirEntry) Name() string               { return "error" }
func (e *errorDirEntry) IsDir() bool                { return false }
func (e *errorDirEntry) Type() os.FileMode          { return 0 }
func (e *errorDirEntry) Info() (os.FileInfo, error) { return nil, os.ErrPermission }

// TestProductionFlowWithComponentInfo tests the production flow where component_info.component_path
// is set (as done by internal/exec/utils.go) but top-level component_path is NOT set.
// This validates that extractComponentPath() correctly reads the nested path.
func TestProductionFlowWithComponentInfo(t *testing.T) {
	tempDir := t.TempDir()

	// Create the shared base component directory.
	baseDir := filepath.Join(tempDir, "components", "terraform", "elasticache")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "main.tf"), []byte("# elasticache"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Simulate the production ComponentSection as built by internal/exec/utils.go:
	// - atmos_component = instance name (from CLI arg)
	// - component = base component (from metadata inheritance)
	// - component_info.component_path = resolved path (set by constructTerraformComponentWorkingDir)
	// - NO top-level component_path
	componentConfig := map[string]any{
		"atmos_component": "elasticache-redis-cluster-1",
		"component":       "elasticache",
		"atmos_stack":     "prod",
		"metadata": map[string]any{
			"component": "elasticache",
		},
		"component_info": map[string]any{
			"component_path": baseDir,
			"component_type": "terraform",
		},
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionWorkdir(context.Background(), atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	require.True(t, ok, "workdir path should be set")

	// Verify workdir uses the instance name (atmos_component).
	expectedSuffix := filepath.Join("terraform", "prod-elasticache-redis-cluster-1")
	assert.True(t, strings.HasSuffix(workdirPath, expectedSuffix),
		"workdir path %s should end with %s", workdirPath, expectedSuffix)

	// Verify main.tf was copied from the base component directory.
	content, err := os.ReadFile(filepath.Join(workdirPath, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# elasticache", string(content))

	// Verify metadata records the instance name.
	metadataPath := MetadataPath(workdirPath)
	metadataBytes, err := os.ReadFile(metadataPath)
	require.NoError(t, err)
	assert.Contains(t, string(metadataBytes), `"component": "elasticache-redis-cluster-1"`)
}

// TestSourcePathUsesBaseComponentNotInstance verifies the critical fix:
// the source path is resolved from the base component name, not the instance name.
// Without this fix, the provisioner would look for components/terraform/elasticache-redis-cluster-1
// which doesn't exist - the actual module is at components/terraform/elasticache.
func TestSourcePathUsesBaseComponentNotInstance(t *testing.T) {
	tempDir := t.TempDir()

	// Create ONLY the base component directory (not the instance name directory).
	baseDir := filepath.Join(tempDir, "components", "terraform", "rds")
	require.NoError(t, os.MkdirAll(baseDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(baseDir, "main.tf"), []byte("# rds module"), 0o644))

	// Do NOT create components/terraform/rds-primary - this is the instance name, not a directory.

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	componentConfig := map[string]any{
		"atmos_component": "rds-primary",
		"component":       "rds",
		"atmos_stack":     "dev",
		"metadata": map[string]any{
			"component": "rds",
		},
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionWorkdir(context.Background(), atmosConfig, componentConfig, nil)
	require.NoError(t, err, "should succeed using base component path, not instance name path")

	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	require.True(t, ok)

	// Workdir should be named after the instance.
	assert.Contains(t, workdirPath, "dev-rds-primary")

	// Content should come from the base component directory.
	content, err := os.ReadFile(filepath.Join(workdirPath, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# rds module", string(content))
}

// TestSourceComponentFallsBackToWorkdirComponent verifies that when no "component" key
// is set (non-inherited component), the source path falls back to using atmos_component.
func TestSourceComponentFallsBackToWorkdirComponent(t *testing.T) {
	tempDir := t.TempDir()

	// Create the component directory matching the atmos_component name.
	compDir := filepath.Join(tempDir, "components", "terraform", "simple-vpc")
	require.NoError(t, os.MkdirAll(compDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(compDir, "main.tf"), []byte("# simple vpc"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Non-inherited component: atmos_component and component are the same.
	componentConfig := map[string]any{
		"atmos_component": "simple-vpc",
		"component":       "simple-vpc",
		"atmos_stack":     "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := ProvisionWorkdir(context.Background(), atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	require.True(t, ok)
	assert.Contains(t, workdirPath, "dev-simple-vpc")
}

// Note: Tests for WorkdirPathKey extraction logic (used to override component path
// in terraform execution) are in internal/exec/terraform_shell_test.go:TestWorkdirPathKeyExtraction.
// That test covers all edge cases: path set, not set, empty string, nil, and wrong type.
