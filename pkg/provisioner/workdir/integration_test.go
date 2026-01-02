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

	// Verify metadata file was created with correct content.
	metadataPath := filepath.Join(workdirPath, WorkdirMetadataFile)
	metadataBytes, err := os.ReadFile(metadataPath)
	require.NoError(t, err, "metadata file should exist")
	assert.Contains(t, string(metadataBytes), `"component": "test-component"`)
	assert.Contains(t, string(metadataBytes), `"stack": "dev"`)
	assert.Contains(t, string(metadataBytes), `"source_type": "local"`)
}

// TestService_Provision_WithMockFileSystem tests the Service using mocked dependencies.
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

	// Set up mock expectations with metadata content verification.
	mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(componentPath).Return(true)
	mockFS.EXPECT().CopyDir(componentPath, expectedWorkdir).Return(nil)
	mockHasher.EXPECT().HashDir(expectedWorkdir).Return("abc123", nil)
	mockFS.EXPECT().WriteFile(
		filepath.Join(expectedWorkdir, WorkdirMetadataFile),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(path string, data []byte, perm os.FileMode) error {
		// Verify metadata content contains expected fields.
		content := string(data)
		assert.Contains(t, content, `"component": "vpc"`)
		assert.Contains(t, content, `"stack": "dev"`)
		assert.Contains(t, content, `"content_hash": "abc123"`)
		return nil
	})

	ctx := context.Background()
	err := service.Provision(ctx, atmosConfig, componentConfig)
	require.NoError(t, err)

	// Verify workdir path was set with stack-component naming.
	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	assert.True(t, ok)
	assert.Equal(t, expectedWorkdir, workdirPath)
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
			name: "CopyDir fails",
			setupMocks: func(mockFS *MockFileSystem, _ *MockHasher, expectedWorkdir, componentPath string) {
				mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
				mockFS.EXPECT().Exists(componentPath).Return(true)
				mockFS.EXPECT().CopyDir(componentPath, expectedWorkdir).Return(os.ErrNotExist)
			},
			expectedError: "file does not exist",
		},
		{
			name: "WriteFile fails",
			setupMocks: func(mockFS *MockFileSystem, mockHasher *MockHasher, expectedWorkdir, componentPath string) {
				mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
				mockFS.EXPECT().Exists(componentPath).Return(true)
				mockFS.EXPECT().CopyDir(componentPath, expectedWorkdir).Return(nil)
				mockHasher.EXPECT().HashDir(expectedWorkdir).Return("abc123", nil)
				mockFS.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).Return(os.ErrPermission)
			},
			expectedError: "permission denied",
		},
	}
	// Note: HashDir failure is handled gracefully with a warning, not an error.
	// The implementation continues to write metadata even if hash computation fails.

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

// Note: Tests for WorkdirPathKey extraction logic (used to override component path
// in terraform execution) are in internal/exec/terraform_shell_test.go:TestWorkdirPathKeyExtraction.
// That test covers all edge cases: path set, not set, empty string, nil, and wrong type.
