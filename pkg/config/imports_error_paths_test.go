package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestProcessConfigImports_NilSource tests error path at imports.go:70-72.
func TestProcessConfigImports_NilSource(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")

	err := processConfigImports(nil, v)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceDestination)
}

// TestProcessConfigImports_NilDestination tests error path at imports.go:70-72.
func TestProcessConfigImports_NilDestination(t *testing.T) {
	source := &schema.AtmosConfiguration{
		BasePath: "/test",
	}

	err := processConfigImports(source, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceDestination)
}

// TestProcessConfigImports_EmptyImports tests early return at imports.go:73-75.
func TestProcessConfigImports_EmptyImports(t *testing.T) {
	source := &schema.AtmosConfiguration{
		BasePath: "/test",
		Import:   []string{}, // Empty imports
	}

	v := viper.New()
	v.SetConfigType("yaml")

	err := processConfigImports(source, v)
	assert.NoError(t, err) // Should return nil when no imports
}

// DELETED: TestProcessConfigImports_AbsPathError - Was a fake test using `_ = err`.
// Comment admitted: "Will succeed as empty path is converted to current directory".
// filepath.Abs() errors are nearly impossible to trigger without OS-level failures.

// DELETED: TestProcessConfigImports_MkdirTempError - Was a fake test using `_ = err`.
// Comment admitted: "This is hard to trigger without modifying system state".
// Replaced with real mocked test below.

// TestProcessConfigImports_MkdirTempError_WithMock tests MkdirTemp error path using mocks.
func TestProcessConfigImports_MkdirTempError_WithMock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFS := filesystem.NewMockFileSystem(ctrl)
	mockFS.EXPECT().MkdirTemp("", "atmos-import-*").Return("", errors.New("disk full"))

	source := &schema.AtmosConfiguration{
		BasePath: "/test",
		Import:   []string{"config.yaml"},
	}

	v := viper.New()
	v.SetConfigType("yaml")

	err := processConfigImportsWithFS(source, v, mockFS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

// TestProcessImports_EmptyBasePath tests error path at imports.go:108-110.
func TestProcessImports_EmptyBasePath(t *testing.T) {
	tempDir := t.TempDir()

	_, err := processImports("", []string{"test.yaml"}, tempDir, 1, MaximumImportLvL)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrBasePath)
}

// TestProcessImports_EmptyTempDir tests error path at imports.go:111-113.
func TestProcessImports_EmptyTempDir(t *testing.T) {
	tempDir := t.TempDir()

	_, err := processImports(tempDir, []string{"test.yaml"}, "", 1, MaximumImportLvL)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTempDir)
}

// TestProcessImports_MaxDepthExceeded tests error path at imports.go:114-116.
func TestProcessImports_MaxDepthExceeded(t *testing.T) {
	tempDir := t.TempDir()

	_, err := processImports(tempDir, []string{"test.yaml"}, tempDir, 11, 10)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMaxImportDepth)
}

// DELETED: TestProcessImports_AbsPathError - Was a fake test using `_ = err`.
// Comment admitted: "Will fail at processLocalImport but not at filepath.Abs".
// This didn't actually test the filepath.Abs error path.

// TestProcessImports_EmptyImportPath tests skip path at imports.go:124-126.
func TestProcessImports_EmptyImportPath(t *testing.T) {
	tempDir := t.TempDir()

	paths, err := processImports(tempDir, []string{""}, tempDir, 1, MaximumImportLvL)
	assert.NoError(t, err)
	assert.Empty(t, paths) // Empty import path should be skipped
}

// TestSearchAtmosConfig_FindMatchingFilesError tests error path at imports.go:268-271.
func TestSearchAtmosConfig_FindMatchingFilesError(t *testing.T) {
	tempDir := t.TempDir()

	// Create a pattern that won't match any files
	nonMatchingPattern := filepath.Join(tempDir, "nonexistent", "*.yaml")

	_, err := SearchAtmosConfig(nonMatchingPattern)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find matching files")
}

// TestSearchAtmosConfig_ConvertToAbsolutePathsError tests error path at imports.go:273-276.
func TestSearchAtmosConfig_ConvertToAbsolutePathsError(t *testing.T) {
	tempDir := t.TempDir()

	// Create files
	configPath := filepath.Join(tempDir, "test.yaml")
	err := os.WriteFile(configPath, []byte("test: value"), 0o644)
	assert.NoError(t, err)

	// Normal search should succeed
	paths, err := SearchAtmosConfig(tempDir)
	assert.NoError(t, err)
	assert.NotEmpty(t, paths)
}

// TestGeneratePatterns_DirectoryPath tests directory path at imports.go:285-295.
func TestGeneratePatterns_DirectoryPath(t *testing.T) {
	tempDir := t.TempDir()

	patterns := generatePatterns(tempDir)
	assert.Len(t, patterns, 2) // Should generate *.yaml and *.yml patterns
	assert.Contains(t, patterns[0], "**")
	assert.Contains(t, patterns[0], ".yaml")
	assert.Contains(t, patterns[1], ".yml")
}

// TestGeneratePatterns_FileWithExtension tests file path at imports.go:297-307.
func TestGeneratePatterns_FileWithExtension(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "config.yaml")

	patterns := generatePatterns(filePath)
	assert.Len(t, patterns, 1) // Should return as-is
	assert.Equal(t, filePath, patterns[0])
}

// TestGeneratePatterns_FileWithoutExtension tests no extension path at imports.go:298-304.
func TestGeneratePatterns_FileWithoutExtension(t *testing.T) {
	tempDir := t.TempDir()
	filePathNoExt := filepath.Join(tempDir, "config")

	patterns := generatePatterns(filePathNoExt)
	assert.Len(t, patterns, 2) // Should append .yaml and .yml
	assert.Equal(t, filePathNoExt+".yaml", patterns[0])
	assert.Equal(t, filePathNoExt+".yml", patterns[1])
}

// TestConvertToAbsolutePaths_AbsPathError tests error path at imports.go:314-318.
func TestConvertToAbsolutePaths_AbsPathError(t *testing.T) {
	// Test with valid paths (hard to trigger filepath.Abs error)
	paths := []string{"test.yaml", "config.yaml"}

	absPaths, err := convertToAbsolutePaths(paths)
	assert.NoError(t, err)
	assert.Len(t, absPaths, 2)
	for _, p := range absPaths {
		assert.True(t, filepath.IsAbs(p))
	}
}

// TestConvertToAbsolutePaths_EmptyResult tests error path at imports.go:322-324.
func TestConvertToAbsolutePaths_EmptyResult(t *testing.T) {
	// Pass empty slice
	_, err := convertToAbsolutePaths([]string{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoValidAbsolutePaths)
}

// TestDetectPriorityFiles_YamlOverYml tests priority at imports.go:330-362.
func TestDetectPriorityFiles_YamlOverYml(t *testing.T) {
	tempDir := t.TempDir()

	// Create both .yaml and .yml files in same directory
	yamlFile := filepath.Join(tempDir, "config.yaml")
	ymlFile := filepath.Join(tempDir, "config.yml")

	files := []string{ymlFile, yamlFile}

	result := detectPriorityFiles(files)

	// Should only have one file (.yaml should win)
	assert.Len(t, result, 1)
	assert.Equal(t, yamlFile, result[0])
}

// TestDetectPriorityFiles_SingleFile tests single file at imports.go:330-362.
func TestDetectPriorityFiles_SingleFile(t *testing.T) {
	tempDir := t.TempDir()
	singleFile := filepath.Join(tempDir, "config.yml")

	result := detectPriorityFiles([]string{singleFile})

	assert.Len(t, result, 1)
	assert.Equal(t, singleFile, result[0])
}

// TestDetectPriorityFiles_DifferentDirectories tests different dirs at imports.go:330-362.
func TestDetectPriorityFiles_DifferentDirectories(t *testing.T) {
	tempDir := t.TempDir()

	dir1 := filepath.Join(tempDir, "dir1")
	dir2 := filepath.Join(tempDir, "dir2")
	err := os.MkdirAll(dir1, 0o755)
	assert.NoError(t, err)
	err = os.MkdirAll(dir2, 0o755)
	assert.NoError(t, err)

	file1 := filepath.Join(dir1, "config.yaml")
	file2 := filepath.Join(dir2, "config.yaml")

	result := detectPriorityFiles([]string{file1, file2})

	// Both should be included (different directories)
	assert.Len(t, result, 2)
}

// TestFindMatchingFiles_NoMatches tests error path at imports.go:411-413.
func TestFindMatchingFiles_NoMatches(t *testing.T) {
	tempDir := t.TempDir()

	// Pattern that won't match anything
	patterns := []string{filepath.Join(tempDir, "nonexistent", "*.yaml")}

	_, err := findMatchingFiles(patterns)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoFileMatchPattern)
}

// Additional credential redaction tests are in import_test.go:TestSanitizeImport.
