package workdir

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIsWorkdirEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "no provision",
			config:   map[string]any{},
			expected: false,
		},
		{
			name: "provision without workdir",
			config: map[string]any{
				"provision": map[string]any{},
			},
			expected: false,
		},
		{
			name: "workdir without enabled",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{},
				},
			},
			expected: false,
		},
		{
			name: "enabled false",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name: "enabled true",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "enabled as string (invalid)",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": "true",
					},
				},
			},
			expected: false,
		},
		{
			name: "workdir as bool instead of map (invalid)",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": true,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkdirEnabled(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractComponentName(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected string
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: "",
		},
		{
			name: "component in root",
			config: map[string]any{
				"component": "vpc",
			},
			expected: "vpc",
		},
		{
			name: "component in metadata",
			config: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expected: "vpc",
		},
		{
			name: "component in vars (fallback)",
			config: map[string]any{
				"vars": map[string]any{
					"component": "vpc",
				},
			},
			expected: "vpc",
		},
		{
			name: "root takes precedence",
			config: map[string]any{
				"component": "root-vpc",
				"metadata": map[string]any{
					"component": "metadata-vpc",
				},
			},
			expected: "root-vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultPathFilter_Match(t *testing.T) {
	filter := NewDefaultPathFilter()

	tests := []struct {
		name     string
		path     string
		included []string
		excluded []string
		expected bool
	}{
		{
			name:     "no patterns includes all",
			path:     "main.tf",
			included: nil,
			excluded: nil,
			expected: true,
		},
		{
			name:     "matches include pattern",
			path:     "main.tf",
			included: []string{"*.tf"},
			excluded: nil,
			expected: true,
		},
		{
			name:     "does not match include pattern",
			path:     "README.md",
			included: []string{"*.tf"},
			excluded: nil,
			expected: false,
		},
		{
			name:     "matches exclude pattern",
			path:     "test.tf",
			included: []string{"*.tf"},
			excluded: []string{"test*"},
			expected: false,
		},
		{
			name:     "no include but matches exclude",
			path:     "test.tf",
			included: nil,
			excluded: []string{"test*"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filter.Match(tt.path, tt.included, tt.excluded)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultPathFilter_Match_InvalidPatterns(t *testing.T) {
	filter := NewDefaultPathFilter()

	tests := []struct {
		name     string
		path     string
		included []string
		excluded []string
	}{
		{
			name:     "invalid include pattern",
			path:     "main.tf",
			included: []string{"["},
			excluded: nil,
		},
		{
			name:     "invalid exclude pattern",
			path:     "main.tf",
			included: nil,
			excluded: []string{"["},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := filter.Match(tt.path, tt.included, tt.excluded)
			assert.Error(t, err)
		})
	}
}

// Tests for Service.Provision.

func TestServiceProvision_WorkdirDisabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: "/tmp"}
	componentConfig := map[string]any{
		"component": "vpc",
		// No provision.workdir.enabled
	}

	err := service.Provision(context.Background(), atmosConfig, componentConfig)
	require.NoError(t, err)
	// No FS calls should have been made.
}

func TestServiceProvision_MissingComponentName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: "/tmp"}
	componentConfig := map[string]any{
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
		// No component field
	}

	err := service.Provision(context.Background(), atmosConfig, componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
}

func TestServiceProvision_MkdirFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(errors.New("mkdir failed"))

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: "/tmp"}
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := service.Provision(context.Background(), atmosConfig, componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirCreation)
}

func TestServiceProvision_SourceNotExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(gomock.Any()).Return(false)

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: "/tmp"}
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := service.Provision(context.Background(), atmosConfig, componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
}

func TestServiceProvision_SyncDirFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(gomock.Any(), gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(gomock.Any()).Return(true)
	mockFS.EXPECT().SyncDir(gomock.Any(), gomock.Any(), mockHasher).Return(false, errors.New("sync failed"))

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: "/tmp"}
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := service.Provision(context.Background(), atmosConfig, componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirSync)
}

func TestServiceProvision_HashDirFails_ContinuesSuccessfully(t *testing.T) {
	// Create a temp dir so WriteMetadata can write the file.
	tempDir := t.TempDir()
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(workdirPath, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(gomock.Any()).Return(true)
	mockFS.EXPECT().SyncDir(gomock.Any(), workdirPath, mockHasher).Return(true, nil)
	mockHasher.EXPECT().HashDir(workdirPath).Return("", errors.New("hash failed"))
	// Note: WriteMetadata uses real filesystem with atomic write, not mocked FileSystem.

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	// Hash failure is a warning, not an error.
	err = service.Provision(context.Background(), atmosConfig, componentConfig)
	require.NoError(t, err)
	// Verify workdir path was set.
	assert.NotEmpty(t, componentConfig[WorkdirPathKey])
}

func TestServiceProvision_WriteMetadataFails(t *testing.T) {
	// Create a temp dir with a read-only .atmos subdirectory to force WriteMetadata to fail.
	tempDir := t.TempDir()
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)
	// Create .atmos directory as read-only to prevent writing metadata.
	atmosDir := filepath.Join(workdirPath, AtmosDir)
	err = os.MkdirAll(atmosDir, 0o555)
	require.NoError(t, err)
	t.Cleanup(func() {
		// Restore permissions so cleanup can delete the directory.
		_ = os.Chmod(atmosDir, 0o755)
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(workdirPath, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(gomock.Any()).Return(true)
	mockFS.EXPECT().SyncDir(gomock.Any(), workdirPath, mockHasher).Return(true, nil)
	mockHasher.EXPECT().HashDir(workdirPath).Return("abc123", nil)
	// Note: WriteMetadata uses real filesystem with atomic write, not mocked FileSystem.
	// The read-only .atmos dir will cause the write to fail.

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err = service.Provision(context.Background(), atmosConfig, componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

func TestServiceProvision_Success(t *testing.T) {
	// Create a temp dir so WriteMetadata can write the file.
	tempDir := t.TempDir()
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(workdirPath, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(gomock.Any()).Return(true)
	mockFS.EXPECT().SyncDir(gomock.Any(), workdirPath, mockHasher).Return(true, nil)
	mockHasher.EXPECT().HashDir(workdirPath).Return("abc123def456", nil)
	// Note: WriteMetadata uses real filesystem with atomic write, not mocked FileSystem.

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err = service.Provision(context.Background(), atmosConfig, componentConfig)
	require.NoError(t, err)
	assert.NotEmpty(t, componentConfig[WorkdirPathKey])
}

func TestServiceProvision_ComponentPathFromConfig(t *testing.T) {
	// Create a temp dir so WriteMetadata can write the file.
	tempDir := t.TempDir()
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(workdirPath, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists("/custom/path/to/component").Return(true)
	mockFS.EXPECT().SyncDir("/custom/path/to/component", workdirPath, mockHasher).Return(true, nil)
	mockHasher.EXPECT().HashDir(workdirPath).Return("abc123", nil)
	// Note: WriteMetadata uses real filesystem with atomic write, not mocked FileSystem.

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}
	componentConfig := map[string]any{
		"component":      "vpc",
		"atmos_stack":    "dev",
		"component_path": "/custom/path/to/component",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err = service.Provision(context.Background(), atmosConfig, componentConfig)
	require.NoError(t, err)
}

func TestServiceProvision_EmptyBasePath(t *testing.T) {
	// Create a temp dir as current working directory for this test.
	// When BasePath is empty, it defaults to ".".
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	// Create the workdir path since WriteMetadata needs it.
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	// The workdir path will be relative since BasePath is empty (defaults to ".").
	relativeWorkdirPath := filepath.Join(".workdir", "terraform", "dev-vpc")
	mockFS.EXPECT().MkdirAll(relativeWorkdirPath, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(gomock.Any()).Return(true)
	mockFS.EXPECT().SyncDir(gomock.Any(), relativeWorkdirPath, mockHasher).Return(true, nil)
	mockHasher.EXPECT().HashDir(relativeWorkdirPath).Return("abc123", nil)
	// Note: WriteMetadata uses real filesystem with atomic write, not mocked FileSystem.

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: ""}
	componentConfig := map[string]any{
		"component":   "vpc",
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err = service.Provision(context.Background(), atmosConfig, componentConfig)
	require.NoError(t, err)
}

// Tests for extractComponentPath.

func TestExtractComponentPath(t *testing.T) {
	// Use filepath.FromSlash for expected paths to ensure cross-platform compatibility.
	// On Windows, paths use backslashes; on Unix, they use forward slashes.
	tests := []struct {
		name            string
		atmosConfig     *schema.AtmosConfiguration
		componentConfig map[string]any
		component       string
		expected        string
	}{
		{
			name: "uses component_path from config",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: filepath.FromSlash("/project"),
			},
			componentConfig: map[string]any{
				"component_path": filepath.FromSlash("/custom/path"),
			},
			component: "vpc",
			expected:  filepath.FromSlash("/custom/path"),
		},
		{
			name: "builds default path with custom components base",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: filepath.FromSlash("/project"),
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: filepath.FromSlash("custom/components"),
					},
				},
			},
			componentConfig: map[string]any{},
			component:       "vpc",
			expected:        filepath.FromSlash("/project/custom/components/vpc"),
		},
		{
			name: "builds default path with default components base",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: filepath.FromSlash("/project"),
			},
			componentConfig: map[string]any{},
			component:       "vpc",
			expected:        filepath.FromSlash("/project/components/terraform/vpc"),
		},
		{
			name: "empty basepath uses dot",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "",
			},
			componentConfig: map[string]any{},
			component:       "s3-bucket",
			expected:        filepath.FromSlash("components/terraform/s3-bucket"),
		},
		{
			name: "empty component_path uses default",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: filepath.FromSlash("/project"),
			},
			componentConfig: map[string]any{
				"component_path": "", // Empty string should fallback.
			},
			component: "vpc",
			expected:  filepath.FromSlash("/project/components/terraform/vpc"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentPath(tt.atmosConfig, tt.componentConfig, tt.component)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for ProvisionWorkdir entry point.

func TestProvisionWorkdir_DisabledReturnsNil(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: "/tmp"}
	componentConfig := map[string]any{
		"component": "vpc",
		// No provision.workdir.enabled
	}

	err := ProvisionWorkdir(context.Background(), atmosConfig, componentConfig, nil)
	require.NoError(t, err)
}

// Tests for NewService and NewServiceWithDeps.

func TestNewService(t *testing.T) {
	service := NewService()
	assert.NotNil(t, service)
	assert.NotNil(t, service.fs)
	assert.NotNil(t, service.hasher)
}

func TestNewServiceWithDeps(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockHasher)
	assert.NotNil(t, service)
	assert.Equal(t, mockFS, service.fs)
	assert.Equal(t, mockHasher, service.hasher)
}

// Tests for DefaultFileSystem methods.

func TestDefaultFileSystem_MkdirAll(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	path := filepath.Join(tmpDir, "a", "b", "c")
	err := fs.MkdirAll(path, 0o755)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestDefaultFileSystem_RemoveAll(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	// Create directory with contents.
	path := filepath.Join(tmpDir, "todelete")
	require.NoError(t, os.MkdirAll(path, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(path, "file.txt"), []byte("test"), 0o644))

	err := fs.RemoveAll(path)
	require.NoError(t, err)

	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestDefaultFileSystem_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	// Test existing.
	existingFile := filepath.Join(tmpDir, "exists.txt")
	require.NoError(t, os.WriteFile(existingFile, []byte("test"), 0o644))
	assert.True(t, fs.Exists(existingFile))

	// Test non-existing.
	assert.False(t, fs.Exists(filepath.Join(tmpDir, "nonexistent")))
}

func TestDefaultFileSystem_ReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	content := []byte("hello world")
	filePath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, content, 0o644))

	read, err := fs.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, read)
}

func TestDefaultFileSystem_ReadFile_NotFound(t *testing.T) {
	fs := NewDefaultFileSystem()
	_, err := fs.ReadFile("/nonexistent/file.txt")
	assert.Error(t, err)
}

func TestDefaultFileSystem_WriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	filePath := filepath.Join(tmpDir, "output.txt")
	content := []byte("written content")

	err := fs.WriteFile(filePath, content, 0o644)
	require.NoError(t, err)

	read, err := os.ReadFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, content, read)
}

func TestDefaultFileSystem_CopyDir(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	// Create source directory.
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("test"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "nested.txt"), []byte("nested"), 0o644))

	// Copy.
	dstDir := filepath.Join(tmpDir, "dst")
	err := fs.CopyDir(srcDir, dstDir)
	require.NoError(t, err)

	// Verify.
	assert.True(t, fs.Exists(filepath.Join(dstDir, "file.txt")))
	assert.True(t, fs.Exists(filepath.Join(dstDir, "subdir", "nested.txt")))
}

func TestDefaultFileSystem_Walk(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	// Create directory structure.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "a", "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a", "file2.txt"), []byte("2"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a", "b", "file3.txt"), []byte("3"), 0o644))

	var files []string
	err := fs.Walk(tmpDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, files, 3)
}

func TestDefaultFileSystem_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	filePath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644))

	info, err := fs.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, "test.txt", info.Name())
	assert.False(t, info.IsDir())
}

func TestDefaultFileSystem_Stat_NotFound(t *testing.T) {
	fs := NewDefaultFileSystem()
	_, err := fs.Stat("/nonexistent/file.txt")
	assert.Error(t, err)
}

// Tests for DefaultHasher.

func TestDefaultHasher_HashFile(t *testing.T) {
	tmpDir := t.TempDir()
	hasher := NewDefaultHasher()

	filePath := filepath.Join(tmpDir, "test.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("hello world"), 0o644))

	hash, err := hasher.HashFile(filePath)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA256 hex string is 64 chars.

	// Same content should produce same hash.
	hash2, err := hasher.HashFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)
}

func TestDefaultHasher_HashFile_NotFound(t *testing.T) {
	hasher := NewDefaultHasher()
	_, err := hasher.HashFile("/nonexistent/file.txt")
	assert.Error(t, err)
}

func TestDefaultHasher_HashDir(t *testing.T) {
	tmpDir := t.TempDir()
	hasher := NewDefaultHasher()

	// Create directory with files.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("aaa"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "b.txt"), []byte("bbb"), 0o644))

	hash, err := hasher.HashDir(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// Same content should produce same hash.
	hash2, err := hasher.HashDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, hash, hash2)
}

func TestDefaultHasher_HashDir_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	hasher := NewDefaultHasher()

	hash, err := hasher.HashDir(tmpDir)
	require.NoError(t, err)
	// Empty dir should produce consistent hash.
	assert.NotEmpty(t, hash)
}

func TestDefaultHasher_HashDir_NotFound(t *testing.T) {
	hasher := NewDefaultHasher()
	_, err := hasher.HashDir("/nonexistent/dir")
	assert.Error(t, err)
}

func TestDefaultHasher_HashDir_Deterministic(t *testing.T) {
	tmpDir := t.TempDir()
	hasher := NewDefaultHasher()

	// Create files in non-alphabetical order.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "z.txt"), []byte("zzz"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("aaa"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "m.txt"), []byte("mmm"), 0o644))

	// Hash should be deterministic regardless of file creation order.
	hash1, err := hasher.HashDir(tmpDir)
	require.NoError(t, err)

	hash2, err := hasher.HashDir(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, hash1, hash2)
}

// TestServiceProvision_ComponentNameSources tests component name extraction from
// different sources (metadata vs vars).
func TestServiceProvision_ComponentNameSources(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expectedInPath  string
	}{
		{
			name: "component from metadata",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "vpc-from-metadata",
				},
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedInPath: "dev-vpc-from-metadata",
		},
		{
			name: "component from vars fallback",
			componentConfig: map[string]any{
				"vars": map[string]any{
					"component": "vpc-from-vars",
				},
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedInPath: "dev-vpc-from-vars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temp dir so WriteMetadata can write the file.
			tempDir := t.TempDir()
			workdirPath := filepath.Join(tempDir, ".workdir", "terraform", tt.expectedInPath)
			err := os.MkdirAll(workdirPath, 0o755)
			require.NoError(t, err)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFS := NewMockFileSystem(ctrl)
			mockHasher := NewMockHasher(ctrl)

			mockFS.EXPECT().MkdirAll(workdirPath, gomock.Any()).Return(nil)
			mockFS.EXPECT().Exists(gomock.Any()).Return(true)
			mockFS.EXPECT().SyncDir(gomock.Any(), workdirPath, mockHasher).Return(true, nil)
			mockHasher.EXPECT().HashDir(workdirPath).Return("abc123", nil)
			// Note: WriteMetadata uses real filesystem with atomic write, not mocked FileSystem.

			service := NewServiceWithDeps(mockFS, mockHasher)
			atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}

			err = service.Provision(context.Background(), atmosConfig, tt.componentConfig)
			require.NoError(t, err)
			assert.Contains(t, tt.componentConfig[WorkdirPathKey], tt.expectedInPath)
		})
	}
}

// TestServiceProvision_MissingStackName tests error when atmos_stack is missing.
func TestServiceProvision_MissingStackName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: "/tmp"}
	componentConfig := map[string]any{
		"component": "vpc",
		// Missing atmos_stack
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err := service.Provision(context.Background(), atmosConfig, componentConfig)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirProvision)
}

// TestServiceProvision_ComponentKeyNotString tests when component key exists but is not a string.
func TestServiceProvision_ComponentKeyNotString(t *testing.T) {
	// Create a temp dir so WriteMetadata can write the file.
	tempDir := t.TempDir()
	workdirPath := filepath.Join(tempDir, ".workdir", "terraform", "dev-vpc-fallback")
	err := os.MkdirAll(workdirPath, 0o755)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := NewMockFileSystem(ctrl)
	mockHasher := NewMockHasher(ctrl)

	mockFS.EXPECT().MkdirAll(workdirPath, gomock.Any()).Return(nil)
	mockFS.EXPECT().Exists(gomock.Any()).Return(true)
	mockFS.EXPECT().SyncDir(gomock.Any(), workdirPath, mockHasher).Return(true, nil)
	mockHasher.EXPECT().HashDir(workdirPath).Return("abc123", nil)
	// Note: WriteMetadata uses real filesystem with atomic write, not mocked FileSystem.

	service := NewServiceWithDeps(mockFS, mockHasher)

	atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}
	// Component key is an int, not string - should fallback to metadata.
	componentConfig := map[string]any{
		"component": 123, // Not a string.
		"metadata": map[string]any{
			"component": "vpc-fallback",
		},
		"atmos_stack": "dev",
		"provision": map[string]any{
			"workdir": map[string]any{
				"enabled": true,
			},
		},
	}

	err = service.Provision(context.Background(), atmosConfig, componentConfig)
	require.NoError(t, err)
	assert.Contains(t, componentConfig[WorkdirPathKey], "dev-vpc-fallback")
}

// TestDefaultHasher_HashDir_WithSubdirectories tests hashing with nested directories.
func TestDefaultHasher_HashDir_WithSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()
	hasher := NewDefaultHasher()

	// Create nested directory structure.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "sub1", "sub2"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.txt"), []byte("root"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "sub1", "file1.txt"), []byte("file1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "sub1", "sub2", "file2.txt"), []byte("file2"), 0o644))

	hash, err := hasher.HashDir(tmpDir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 64) // SHA256 hex is 64 chars.
}

// TestExtractComponentName_EmptyStrings tests extraction with empty string values.
func TestExtractComponentName_EmptyStrings(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected string
	}{
		{
			name: "empty component string in root",
			config: map[string]any{
				"component": "",
				"metadata": map[string]any{
					"component": "from-metadata",
				},
			},
			expected: "from-metadata",
		},
		{
			name: "empty component in metadata, falls back to vars",
			config: map[string]any{
				"metadata": map[string]any{
					"component": "",
				},
				"vars": map[string]any{
					"component": "from-vars",
				},
			},
			expected: "from-vars",
		},
		{
			name: "all empty returns empty",
			config: map[string]any{
				"component": "",
				"metadata": map[string]any{
					"component": "",
				},
				"vars": map[string]any{
					"component": "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractComponentName_InvalidTypes tests extraction with wrong types.
func TestExtractComponentName_InvalidTypes(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected string
	}{
		{
			name: "metadata is not a map",
			config: map[string]any{
				"metadata": "not-a-map",
				"vars": map[string]any{
					"component": "from-vars",
				},
			},
			expected: "from-vars",
		},
		{
			name: "vars is not a map",
			config: map[string]any{
				"vars": "not-a-map",
			},
			expected: "",
		},
		{
			name: "component in metadata is not a string",
			config: map[string]any{
				"metadata": map[string]any{
					"component": 123,
				},
				"vars": map[string]any{
					"component": "from-vars",
				},
			},
			expected: "from-vars",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDefaultFileSystem_CopyDir_NotFound tests CopyDir with non-existent source.
func TestDefaultFileSystem_CopyDir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	err := fs.CopyDir("/nonexistent/source", filepath.Join(tmpDir, "dst"))
	assert.Error(t, err)
}

// TestDefaultFileSystem_Walk_Error tests Walk with error callback.
func TestDefaultFileSystem_Walk_Error(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewDefaultFileSystem()

	// Create some files.
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0o644))

	// Walk with an error-returning callback.
	expectedErr := errors.New("walk error")
	err := fs.Walk(tmpDir, func(path string, d os.DirEntry, err error) error {
		if !d.IsDir() {
			return expectedErr
		}
		return nil
	})
	assert.ErrorIs(t, err, expectedErr)
}
