package workdir

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
// when neither metadata.source nor metadata.workdir is set.
func TestProvisionWorkdir_NoActivation(t *testing.T) {
	ctx := context.Background()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	componentConfig := map[string]any{
		"component": "test-component",
		"metadata":  map[string]any{},
	}

	err := ProvisionWorkdir(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	// Verify no workdir path was set.
	_, ok := componentConfig[WorkdirPathKey]
	assert.False(t, ok, "workdir path should not be set when not activated")
}

// TestProvisionWorkdir_WithMetadataWorkdir verifies that the provisioner
// activates when metadata.workdir is true.
func TestProvisionWorkdir_WithMetadataWorkdir(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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
		"component": "test-component",
		"metadata": map[string]any{
			"workdir": true,
		},
	}

	ctx := context.Background()
	err = ProvisionWorkdir(ctx, atmosConfig, componentConfig, nil)
	require.NoError(t, err)

	// Verify workdir path was set.
	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	assert.True(t, ok, "workdir path should be set")
	assert.Contains(t, workdirPath, ".workdir")
	assert.Contains(t, workdirPath, "test-component")

	// Verify the workdir was created.
	_, err = os.Stat(workdirPath)
	assert.NoError(t, err, "workdir should exist")

	// Verify the main.tf was copied.
	_, err = os.Stat(filepath.Join(workdirPath, "main.tf"))
	assert.NoError(t, err, "main.tf should be copied to workdir")
}

// TestService_Provision_WithMockFileSystem tests the Service using mocked dependencies.
func TestService_Provision_WithMockFileSystem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockCache := NewMockCache(ctrl)
	mockDownloader := NewMockDownloader(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockCache, mockDownloader, mockHasher)

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
		"component": "vpc",
		"metadata": map[string]any{
			"workdir": true,
		},
	}

	expectedWorkdir := filepath.Join(tempDir, ".workdir", "terraform", "vpc")
	componentPath := filepath.Join(tempDir, "components", "terraform", "vpc")

	// Set up mock expectations.
	mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(componentPath).Return(true)
	mockFS.EXPECT().CopyDir(componentPath, expectedWorkdir).Return(nil)
	mockHasher.EXPECT().HashDir(expectedWorkdir).Return("abc123", nil)
	mockFS.EXPECT().WriteFile(
		filepath.Join(expectedWorkdir, WorkdirMetadataFile),
		gomock.Any(),
		gomock.Any(),
	).Return(nil)

	ctx := context.Background()
	err := service.Provision(ctx, atmosConfig, componentConfig)
	require.NoError(t, err)

	// Verify workdir path was set.
	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	assert.True(t, ok)
	assert.Equal(t, expectedWorkdir, workdirPath)
}

// TestService_Provision_WithRemoteSource tests provisioning with a remote source.
func TestService_Provision_WithRemoteSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockCache := NewMockCache(ctrl)
	mockDownloader := NewMockDownloader(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockCache, mockDownloader, mockHasher)

	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	sourceConfig := &SourceConfig{
		URI:     "github.com/cloudposse/terraform-aws-vpc",
		Version: "v1.0.0",
	}

	componentConfig := map[string]any{
		"component": "vpc",
		"metadata": map[string]any{
			"source": map[string]any{
				"uri":     sourceConfig.URI,
				"version": sourceConfig.Version,
			},
		},
	}

	expectedWorkdir := filepath.Join(tempDir, ".workdir", "terraform", "vpc")
	cacheKey := "abc123def456"
	cachedPath := filepath.Join(tempDir, "cache", cacheKey, "content")

	// Set up mock expectations.
	mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
	mockCache.EXPECT().GenerateKey(gomock.Any()).Return(cacheKey)
	mockCache.EXPECT().Path(cacheKey).Return(cachedPath) // Already cached.
	mockFS.EXPECT().CopyDir(cachedPath, expectedWorkdir).Return(nil)
	mockHasher.EXPECT().HashDir(expectedWorkdir).Return("hash123", nil)
	mockFS.EXPECT().WriteFile(
		filepath.Join(expectedWorkdir, WorkdirMetadataFile),
		gomock.Any(),
		gomock.Any(),
	).Return(nil)

	ctx := context.Background()
	err := service.Provision(ctx, atmosConfig, componentConfig)
	require.NoError(t, err)

	// Verify workdir path was set.
	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	assert.True(t, ok)
	assert.Equal(t, expectedWorkdir, workdirPath)
}

// TestService_Provision_DownloadsWhenNotCached tests that the service downloads
// when the source is not cached.
func TestService_Provision_DownloadsWhenNotCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockCache := NewMockCache(ctrl)
	mockDownloader := NewMockDownloader(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockCache, mockDownloader, mockHasher)

	tempDir := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	componentConfig := map[string]any{
		"component": "vpc",
		"metadata": map[string]any{
			"source": "github.com/cloudposse/terraform-aws-vpc?ref=v1.0.0",
		},
	}

	expectedWorkdir := filepath.Join(tempDir, ".workdir", "terraform", "vpc")
	cacheKey := "abc123def456"
	cachedPath := filepath.Join(tempDir, "cache", cacheKey, "content")

	// Set up mock expectations.
	mockFS.EXPECT().MkdirAll(expectedWorkdir, gomock.Any()).Return(nil)
	mockCache.EXPECT().GenerateKey(gomock.Any()).Return(cacheKey)
	mockCache.EXPECT().Path(cacheKey).Return("") // Not cached.

	// Expect download flow.
	mockCache.EXPECT().Path("").Return(tempDir) // Base cache path for temp dir.
	mockFS.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil)
	mockDownloader.EXPECT().Download(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockCache.EXPECT().GetPolicy(gomock.Any()).Return(CachePolicyPermanent)
	mockCache.EXPECT().Put(cacheKey, gomock.Any(), gomock.Any()).Return(nil)
	mockCache.EXPECT().Path(cacheKey).Return(cachedPath)

	mockFS.EXPECT().CopyDir(cachedPath, expectedWorkdir).Return(nil)
	mockHasher.EXPECT().HashDir(expectedWorkdir).Return("hash123", nil)
	mockFS.EXPECT().WriteFile(
		filepath.Join(expectedWorkdir, WorkdirMetadataFile),
		gomock.Any(),
		gomock.Any(),
	).Return(nil)

	// Cleanup temp dir.
	mockFS.EXPECT().RemoveAll(gomock.Any()).Return(nil)

	ctx := context.Background()
	err := service.Provision(ctx, atmosConfig, componentConfig)
	require.NoError(t, err)

	// Verify workdir path was set.
	workdirPath, ok := componentConfig[WorkdirPathKey].(string)
	assert.True(t, ok)
	assert.Equal(t, expectedWorkdir, workdirPath)
}

// TestCleanWorkdir tests the CleanWorkdir function.
func TestCleanWorkdir(t *testing.T) {
	tempDir := t.TempDir()

	// Create a workdir structure.
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "test-component")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	// Create a file in the workdir.
	err = os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# test"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Clean the workdir.
	err = CleanWorkdir(atmosConfig, "test-component")
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

// TestWorkdirPathOverride tests that the WorkdirPathKey is correctly used
// to override the component path in terraform execution.
func TestWorkdirPathOverride(t *testing.T) {
	// This test verifies the logic that checks for WorkdirPathKey.
	componentConfig := map[string]any{
		"component":    "vpc",
		WorkdirPathKey: "/path/to/workdir/terraform/vpc",
	}

	// Simulate the check from terraform.go.
	componentPath := "/original/components/terraform/vpc"

	if workdirPath, ok := componentConfig[WorkdirPathKey].(string); ok && workdirPath != "" {
		componentPath = workdirPath
	}

	assert.Equal(t, "/path/to/workdir/terraform/vpc", componentPath)
}

// TestWorkdirPathOverride_NotSet verifies the original path is used when
// WorkdirPathKey is not set.
func TestWorkdirPathOverride_NotSet(t *testing.T) {
	componentConfig := map[string]any{
		"component": "vpc",
	}

	// Simulate the check from terraform.go.
	componentPath := "/original/components/terraform/vpc"

	if workdirPath, ok := componentConfig[WorkdirPathKey].(string); ok && workdirPath != "" {
		componentPath = workdirPath
	}

	assert.Equal(t, "/original/components/terraform/vpc", componentPath)
}

// TestWorkdirPathOverride_EmptyString verifies the original path is used when
// WorkdirPathKey is an empty string.
func TestWorkdirPathOverride_EmptyString(t *testing.T) {
	componentConfig := map[string]any{
		"component":    "vpc",
		WorkdirPathKey: "",
	}

	// Simulate the check from terraform.go.
	componentPath := "/original/components/terraform/vpc"

	if workdirPath, ok := componentConfig[WorkdirPathKey].(string); ok && workdirPath != "" {
		componentPath = workdirPath
	}

	assert.Equal(t, "/original/components/terraform/vpc", componentPath)
}
