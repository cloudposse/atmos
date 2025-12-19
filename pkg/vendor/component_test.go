package vendor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHandleLocalFileScheme(t *testing.T) {
	tempDir := t.TempDir()

	// Create a local file for testing.
	localFile := filepath.Join(tempDir, "local-component.tf")
	err := os.WriteFile(localFile, []byte("# test"), 0o644)
	require.NoError(t, err)

	// Create a subdirectory.
	subDir := filepath.Join(tempDir, "subdir")
	err = os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)

	tests := []struct {
		name                  string
		componentPath         string
		uri                   string
		expectedUseLocalFS    bool
		expectedSourceIsLocal bool
		checkURIContains      string
	}{
		{
			name:                  "relative path to existing file",
			componentPath:         tempDir,
			uri:                   "local-component.tf",
			expectedUseLocalFS:    true,
			expectedSourceIsLocal: true,
			checkURIContains:      "local-component.tf",
		},
		{
			name:                  "relative path to existing directory",
			componentPath:         tempDir,
			uri:                   "subdir",
			expectedUseLocalFS:    true,
			expectedSourceIsLocal: false,
			checkURIContains:      "subdir",
		},
		{
			name:                  "file:// scheme",
			componentPath:         tempDir,
			uri:                   "file:///some/path/to/component",
			expectedUseLocalFS:    true,
			expectedSourceIsLocal: false,
			checkURIContains:      "some/path/to/component",
		},
		{
			name:                  "remote URI unchanged",
			componentPath:         tempDir,
			uri:                   "github.com/cloudposse/components.git//modules/vpc",
			expectedUseLocalFS:    false,
			expectedSourceIsLocal: false,
			checkURIContains:      "github.com/cloudposse/components.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultURI, useLocalFS, sourceIsLocal := handleLocalFileScheme(tt.componentPath, tt.uri)

			assert.Equal(t, tt.expectedUseLocalFS, useLocalFS, "useLocalFileSystem mismatch")
			assert.Equal(t, tt.expectedSourceIsLocal, sourceIsLocal, "sourceIsLocalFile mismatch")
			// Normalize path separators for cross-platform comparison.
			normalizedURI := filepath.ToSlash(resultURI)
			assert.Contains(t, normalizedURI, tt.checkURIContains, "URI should contain expected string")
		})
	}
}

func TestProcessComponentMixins(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		spec          *schema.VendorComponentSpec
		componentPath string
		expectedCount int
		expectError   error
	}{
		{
			name: "empty mixins",
			spec: &schema.VendorComponentSpec{
				Mixins: []schema.VendorComponentMixins{},
			},
			componentPath: tempDir,
			expectedCount: 0,
		},
		{
			name: "single mixin",
			spec: &schema.VendorComponentSpec{
				Mixins: []schema.VendorComponentMixins{
					{
						Uri:      "github.com/cloudposse/mixins.git//context.tf?ref=1.0.0",
						Filename: "context.tf",
						Version:  "1.0.0",
					},
				},
			},
			componentPath: tempDir,
			expectedCount: 1,
		},
		{
			name: "multiple mixins",
			spec: &schema.VendorComponentSpec{
				Mixins: []schema.VendorComponentMixins{
					{
						Uri:      "github.com/cloudposse/mixins.git//context.tf?ref=1.0.0",
						Filename: "context.tf",
						Version:  "1.0.0",
					},
					{
						Uri:      "github.com/cloudposse/mixins.git//provider.tf?ref=1.0.0",
						Filename: "provider.tf",
						Version:  "1.0.0",
					},
				},
			},
			componentPath: tempDir,
			expectedCount: 2,
		},
		{
			name: "missing URI",
			spec: &schema.VendorComponentSpec{
				Mixins: []schema.VendorComponentMixins{
					{
						Uri:      "",
						Filename: "context.tf",
					},
				},
			},
			componentPath: tempDir,
			expectError:   ErrMissingMixinURI,
		},
		{
			name: "missing filename",
			spec: &schema.VendorComponentSpec{
				Mixins: []schema.VendorComponentMixins{
					{
						Uri:      "github.com/cloudposse/mixins.git//context.tf",
						Filename: "",
					},
				},
			},
			componentPath: tempDir,
			expectError:   ErrMissingMixinFilename,
		},
		{
			name: "OCI scheme mixin",
			spec: &schema.VendorComponentSpec{
				Mixins: []schema.VendorComponentMixins{
					{
						Uri:      "oci://ghcr.io/cloudposse/mixins:1.0.0",
						Filename: "context.tf",
						Version:  "1.0.0",
					},
				},
			},
			componentPath: tempDir,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, err := processComponentMixins(tt.spec, tt.componentPath)

			if tt.expectError != nil {
				assert.ErrorIs(t, err, tt.expectError)
			} else {
				assert.NoError(t, err)
				assert.Len(t, packages, tt.expectedCount)
			}
		})
	}
}

func TestParseMixinURI(t *testing.T) {
	tests := []struct {
		name        string
		mixin       *schema.VendorComponentMixins
		expectedURI string
		expectError bool
	}{
		{
			name: "no version - returns URI as-is",
			mixin: &schema.VendorComponentMixins{
				Uri:      "github.com/cloudposse/mixins.git//context.tf",
				Filename: "context.tf",
			},
			expectedURI: "github.com/cloudposse/mixins.git//context.tf",
		},
		{
			name: "with version template",
			mixin: &schema.VendorComponentMixins{
				Uri:      "github.com/cloudposse/mixins.git//context.tf?ref={{.Version}}",
				Filename: "context.tf",
				Version:  "1.0.0",
			},
			expectedURI: "github.com/cloudposse/mixins.git//context.tf?ref=1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseMixinURI(tt.mixin)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedURI, result)
			}
		})
	}
}

func TestCheckComponentExcludes(t *testing.T) {
	tests := []struct {
		name         string
		excludePaths []string
		src          string
		trimmedSrc   string
		shouldSkip   bool
	}{
		{
			name:         "no excludes - include all",
			excludePaths: []string{},
			src:          "/tmp/main.tf",
			trimmedSrc:   "main.tf",
			shouldSkip:   false,
		},
		{
			name:         "file matches exclude pattern with doublestar",
			excludePaths: []string{"**/*.md"},
			src:          "/tmp/README.md",
			trimmedSrc:   "README.md",
			shouldSkip:   true,
		},
		{
			name:         "file does not match exclude pattern",
			excludePaths: []string{"**/*.md"},
			src:          "/tmp/main.tf",
			trimmedSrc:   "main.tf",
			shouldSkip:   false,
		},
		{
			name:         "doublestar pattern excludes nested",
			excludePaths: []string{"**/test/**"},
			src:          "/tmp/test/main_test.go",
			trimmedSrc:   "test/main_test.go",
			shouldSkip:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := checkComponentExcludes(tt.excludePaths, tt.src, tt.trimmedSrc)
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldSkip, skip)
		})
	}
}

func TestCheckComponentIncludes(t *testing.T) {
	tests := []struct {
		name         string
		includePaths []string
		src          string
		trimmedSrc   string
		shouldSkip   bool
	}{
		{
			name:         "file matches include pattern with doublestar",
			includePaths: []string{"**/*.tf"},
			src:          "/tmp/main.tf",
			trimmedSrc:   "main.tf",
			shouldSkip:   false, // Should include.
		},
		{
			name:         "file does not match include pattern",
			includePaths: []string{"**/*.tf"},
			src:          "/tmp/README.md",
			trimmedSrc:   "README.md",
			shouldSkip:   true, // Should exclude.
		},
		{
			name:         "matches one of multiple doublestar patterns",
			includePaths: []string{"**/*.tf", "**/*.go"},
			src:          "/tmp/main.go",
			trimmedSrc:   "main.go",
			shouldSkip:   false, // Should include.
		},
		{
			name:         "doublestar includes nested files",
			includePaths: []string{"**/*.tf"},
			src:          "/tmp/modules/vpc/main.tf",
			trimmedSrc:   "modules/vpc/main.tf",
			shouldSkip:   false, // Should include.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := checkComponentIncludes(tt.includePaths, tt.src, tt.trimmedSrc)
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldSkip, skip)
		})
	}
}

func TestCreateComponentSkipFunc(t *testing.T) {
	tempDir := t.TempDir()

	// Test that .git is always skipped.
	spec := &schema.VendorComponentSpec{}
	skipFunc := createComponentSkipFunc(tempDir, spec)

	gitInfo := mockFileInfo{name: ".git", isDir: true}
	skip, err := skipFunc(gitInfo, filepath.Join(tempDir, ".git"), "")
	assert.NoError(t, err)
	assert.True(t, skip, ".git should always be skipped")

	// Test normal file is not skipped when no includes/excludes.
	normalInfo := mockFileInfo{name: "main.tf", isDir: false}
	skip, err = skipFunc(normalInfo, filepath.Join(tempDir, "main.tf"), "")
	assert.NoError(t, err)
	assert.False(t, skip, "Normal files should not be skipped")
}

func TestCreateTempDir(t *testing.T) {
	tempDir, err := createTempDir()
	require.NoError(t, err)
	assert.NotEmpty(t, tempDir)

	// Verify directory exists.
	info, err := os.Stat(tempDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify permissions are restricted (Unix only - Windows has different permission model).
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o700)|os.ModeDir, info.Mode())
	}

	// Clean up.
	err = os.RemoveAll(tempDir)
	assert.NoError(t, err)
}

func TestRemoveTempDir(t *testing.T) {
	// Create a temp directory.
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	err := os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)

	// Create a file in the subdirectory.
	testFile := filepath.Join(subDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0o644)
	require.NoError(t, err)

	// Remove should work without error.
	removeTempDir(subDir)

	// Verify directory was removed.
	_, err = os.Stat(subDir)
	assert.True(t, os.IsNotExist(err))
}

func TestRemoveTempDir_NonExistent(t *testing.T) {
	// Removing a non-existent directory should not panic.
	removeTempDir("/non/existent/path/that/does/not/exist")
	// If we reach here, it didn't panic.
}

func TestDeterminePackageType_Component(t *testing.T) {
	tests := []struct {
		name               string
		useOciScheme       bool
		useLocalFileSystem bool
		expected           pkgType
	}{
		{
			name:               "OCI scheme",
			useOciScheme:       true,
			useLocalFileSystem: false,
			expected:           pkgTypeOci,
		},
		{
			name:               "local file system",
			useOciScheme:       false,
			useLocalFileSystem: true,
			expected:           pkgTypeLocal,
		},
		{
			name:               "remote",
			useOciScheme:       false,
			useLocalFileSystem: false,
			expected:           pkgTypeRemote,
		},
		{
			name:               "both flags - OCI takes precedence",
			useOciScheme:       true,
			useLocalFileSystem: true,
			expected:           pkgTypeOci,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determinePackageType(tt.useOciScheme, tt.useLocalFileSystem)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateComponentSkipFunc_WithPaths(t *testing.T) {
	tests := []struct {
		name          string
		includedPaths []string
		excludedPaths []string
		files         []struct {
			filename   string
			content    string
			shouldSkip bool
			desc       string
		}
	}{
		{
			name:          "with included paths",
			includedPaths: []string{"**/*.tf"},
			excludedPaths: nil,
			files: []struct {
				filename   string
				content    string
				shouldSkip bool
				desc       string
			}{
				{"main.tf", "resource", false, ".tf file should be included"},
				{"README.md", "readme", true, ".md file should be excluded when not in included_paths"},
			},
		},
		{
			name:          "with excluded paths",
			includedPaths: nil,
			excludedPaths: []string{"**/*_test.go"},
			files: []struct {
				filename   string
				content    string
				shouldSkip bool
				desc       string
			}{
				{"main.tf", "resource", false, ".tf file should be included"},
				{"main_test.go", "package test", true, "test file should be excluded"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Create test files.
			for _, f := range tt.files {
				filePath := filepath.Join(tempDir, f.filename)
				err := os.WriteFile(filePath, []byte(f.content), 0o644)
				require.NoError(t, err)
			}

			spec := &schema.VendorComponentSpec{
				Source: schema.VendorComponentSource{
					IncludedPaths: tt.includedPaths,
					ExcludedPaths: tt.excludedPaths,
				},
			}
			skipFunc := createComponentSkipFunc(tempDir, spec)

			// Check each file.
			for _, f := range tt.files {
				filePath := filepath.Join(tempDir, f.filename)
				info := mockFileInfo{name: f.filename, isDir: false}
				skip, err := skipFunc(info, filePath, "")
				assert.NoError(t, err)
				assert.Equal(t, f.shouldSkip, skip, f.desc)
			}
		})
	}
}

// mockFileInfo implements os.FileInfo for testing.
type mockFileInfo struct {
	name  string
	isDir bool
}

func (m mockFileInfo) Name() string           { return m.name }
func (m mockFileInfo) Size() int64            { return 0 }
func (m mockFileInfo) Mode() os.FileMode      { return 0 }
func (m mockFileInfo) ModTime() (t time.Time) { return time.Time{} }
func (m mockFileInfo) IsDir() bool            { return m.isDir }
func (m mockFileInfo) Sys() any               { return nil }
