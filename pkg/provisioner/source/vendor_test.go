package source

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveSourceURI(t *testing.T) {
	tests := []struct {
		name       string
		sourceSpec *schema.VendorComponentSource
		expected   string
	}{
		{
			name: "URI with version already in ref",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
				Version: "",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
		},
		{
			name: "URI without ref, version specified",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
				Version: "v1.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
		},
		{
			name: "URI with query params, version specified",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1",
				Version: "v1.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1&ref=v1.0.0",
		},
		{
			name: "URI with ref, version also specified (ref takes priority)",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
				Version: "v2.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
		},
		{
			name: "URI with &ref in query params",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1&ref=v1.0.0",
				Version: "v2.0.0",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc?depth=1&ref=v1.0.0",
		},
		{
			name: "empty version",
			sourceSpec: &schema.VendorComponentSource{
				Uri:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
				Version: "",
			},
			expected: "github.com/cloudposse/terraform-aws-components//modules/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveSourceURI(tt.sourceSpec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "triple slash to double slash dot",
			uri:      "github.com/cloudposse/terraform-aws-vpc///",
			expected: "github.com/cloudposse/terraform-aws-vpc//.",
		},
		{
			name:     "triple slash with query params",
			uri:      "github.com/cloudposse/terraform-aws-vpc///?ref=v1.0.0",
			expected: "github.com/cloudposse/terraform-aws-vpc//.?ref=v1.0.0",
		},
		{
			name:     "no triple slash",
			uri:      "github.com/cloudposse/terraform-aws-vpc//modules/vpc",
			expected: "github.com/cloudposse/terraform-aws-vpc//modules/vpc",
		},
		{
			name:     "empty URI",
			uri:      "",
			expected: "",
		},
		{
			name:     "multiple triple slashes (only first replaced)",
			uri:      "github.com/cloudposse/terraform-aws-vpc//////",
			expected: "github.com/cloudposse/terraform-aws-vpc//.///",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURI(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateSkipFunc(t *testing.T) {
	tests := []struct {
		name       string
		sourceSpec *schema.VendorComponentSource
		fileName   string
		isDir      bool
		expected   bool
	}{
		{
			name: "skip .git directory",
			sourceSpec: &schema.VendorComponentSource{
				Uri: "github.com/example/repo",
			},
			fileName: ".git",
			isDir:    true,
			expected: true,
		},
		{
			name: "no patterns - don't skip regular file",
			sourceSpec: &schema.VendorComponentSource{
				Uri: "github.com/example/repo",
			},
			fileName: "main.tf",
			isDir:    false,
			expected: false,
		},
		{
			name: "no patterns - don't skip regular directory",
			sourceSpec: &schema.VendorComponentSource{
				Uri: "github.com/example/repo",
			},
			fileName: "modules",
			isDir:    true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock FileInfo.
			info := &mockFileInfo{
				name:  tt.fileName,
				isDir: tt.isDir,
			}

			skipFunc := createSkipFunc("/tmp/src", tt.sourceSpec)
			result, err := skipFunc(info, "/tmp/src/"+tt.fileName, "/tmp/dst/"+tt.fileName)

			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockFileInfo implements fs.FileInfo for testing.
type mockFileInfo struct {
	name  string
	isDir bool
}

// Ensure mockFileInfo implements fs.FileInfo.
var _ fs.FileInfo = (*mockFileInfo)(nil)

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0o644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }

// TestMatchesPatterns tests glob pattern matching against file paths.
func TestMatchesPatterns(t *testing.T) {
	tests := []struct {
		name        string
		relPath     string
		patterns    []string
		patternType string
		expected    bool
	}{
		{
			name:        "exact match",
			relPath:     "main.tf",
			patterns:    []string{"main.tf"},
			patternType: "included_paths",
			expected:    true,
		},
		{
			name:        "wildcard match",
			relPath:     "main.tf",
			patterns:    []string{"*.tf"},
			patternType: "included_paths",
			expected:    true,
		},
		{
			name:        "no match",
			relPath:     "main.go",
			patterns:    []string{"*.tf"},
			patternType: "included_paths",
			expected:    false,
		},
		{
			name:        "nested path match by basename",
			relPath:     "modules/vpc/main.tf",
			patterns:    []string{"*.tf"},
			patternType: "included_paths",
			expected:    true,
		},
		{
			name:        "empty patterns",
			relPath:     "main.tf",
			patterns:    []string{},
			patternType: "included_paths",
			expected:    false,
		},
		{
			name:        "multiple patterns - first matches",
			relPath:     "main.tf",
			patterns:    []string{"*.tf", "*.go"},
			patternType: "included_paths",
			expected:    true,
		},
		{
			name:        "multiple patterns - second matches",
			relPath:     "main.go",
			patterns:    []string{"*.tf", "*.go"},
			patternType: "included_paths",
			expected:    true,
		},
		{
			name:        "multiple patterns - none match",
			relPath:     "main.py",
			patterns:    []string{"*.tf", "*.go"},
			patternType: "included_paths",
			expected:    false,
		},
		{
			name:        "pattern with directory component",
			relPath:     "modules/vpc",
			patterns:    []string{"modules/*"},
			patternType: "excluded_paths",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPatterns(tt.relPath, tt.patterns, tt.patternType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMatchesPatterns_InvalidPattern tests handling of invalid glob patterns.
func TestMatchesPatterns_InvalidPattern(t *testing.T) {
	// Invalid pattern with unclosed bracket.
	invalidPattern := "[invalid"
	result := matchesPatterns("main.tf", []string{invalidPattern}, "test_patterns")
	// Should return false (no match) and not panic.
	assert.False(t, result, "Invalid pattern should not match")
}

// TestCopyToTarget tests copying files from source to target directory.
func TestCopyToTarget(t *testing.T) {
	// Create source directory with files.
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main terraform"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "variables.tf"), []byte("# variables"), 0o644)
	require.NoError(t, err)

	// Create nested directory.
	modulesDir := filepath.Join(srcDir, "modules", "vpc")
	err = os.MkdirAll(modulesDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(modulesDir, "main.tf"), []byte("# vpc module"), 0o644)
	require.NoError(t, err)

	// Create target directory path.
	targetDir := filepath.Join(t.TempDir(), "target")

	sourceSpec := &schema.VendorComponentSource{
		Uri: "github.com/example/repo",
	}

	// Copy.
	err = copyToTarget(srcDir, targetDir, sourceSpec)
	require.NoError(t, err)

	// Verify files were copied.
	content, err := os.ReadFile(filepath.Join(targetDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# main terraform", string(content))

	content, err = os.ReadFile(filepath.Join(targetDir, "variables.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# variables", string(content))

	content, err = os.ReadFile(filepath.Join(targetDir, "modules", "vpc", "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# vpc module", string(content))
}

// TestCopyToTarget_WithExcludedPaths tests copying with exclusion patterns.
func TestCopyToTarget_WithExcludedPaths(t *testing.T) {
	// Create source directory with files.
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("test file"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "README.md"), []byte("readme"), 0o644)
	require.NoError(t, err)

	targetDir := filepath.Join(t.TempDir(), "target")

	sourceSpec := &schema.VendorComponentSource{
		Uri:           "github.com/example/repo",
		ExcludedPaths: []string{"*.txt", "*.md"},
	}

	err = copyToTarget(srcDir, targetDir, sourceSpec)
	require.NoError(t, err)

	// main.tf should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err, "main.tf should be copied")

	// test.txt should NOT be copied (excluded).
	_, err = os.Stat(filepath.Join(targetDir, "test.txt"))
	assert.True(t, os.IsNotExist(err), "test.txt should not be copied")

	// README.md should NOT be copied (excluded).
	_, err = os.Stat(filepath.Join(targetDir, "README.md"))
	assert.True(t, os.IsNotExist(err), "README.md should not be copied")
}

// TestCopyToTarget_SkipsGitDirectory tests that .git directories are skipped.
func TestCopyToTarget_SkipsGitDirectory(t *testing.T) {
	// Create source directory with .git directory.
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# main"), 0o644)
	require.NoError(t, err)

	gitDir := filepath.Join(srcDir, ".git")
	err = os.MkdirAll(gitDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main"), 0o644)
	require.NoError(t, err)

	targetDir := filepath.Join(t.TempDir(), "target")

	sourceSpec := &schema.VendorComponentSource{
		Uri: "github.com/example/repo",
	}

	err = copyToTarget(srcDir, targetDir, sourceSpec)
	require.NoError(t, err)

	// main.tf should be copied.
	_, err = os.Stat(filepath.Join(targetDir, "main.tf"))
	assert.NoError(t, err, "main.tf should be copied")

	// .git should NOT be copied.
	_, err = os.Stat(filepath.Join(targetDir, ".git"))
	assert.True(t, os.IsNotExist(err), ".git directory should not be copied")
}

// TestCopyToTarget_OverwritesExisting tests that existing target is overwritten.
func TestCopyToTarget_OverwritesExisting(t *testing.T) {
	// Create source directory.
	srcDir := t.TempDir()
	err := os.WriteFile(filepath.Join(srcDir, "main.tf"), []byte("# new content"), 0o644)
	require.NoError(t, err)

	// Create target directory with existing content.
	targetDir := filepath.Join(t.TempDir(), "target")
	err = os.MkdirAll(targetDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "main.tf"), []byte("# old content"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "old_file.tf"), []byte("# should be removed"), 0o644)
	require.NoError(t, err)

	sourceSpec := &schema.VendorComponentSource{
		Uri: "github.com/example/repo",
	}

	err = copyToTarget(srcDir, targetDir, sourceSpec)
	require.NoError(t, err)

	// main.tf should have new content.
	content, err := os.ReadFile(filepath.Join(targetDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# new content", string(content), "Content should be overwritten")

	// old_file.tf should be removed (target is replaced entirely).
	_, err = os.Stat(filepath.Join(targetDir, "old_file.tf"))
	assert.True(t, os.IsNotExist(err), "Old file should be removed")
}

// TestCreateSkipFunc_IncludedPaths tests skip function with included_paths patterns.
func TestCreateSkipFunc_IncludedPaths(t *testing.T) {
	sourceSpec := &schema.VendorComponentSource{
		Uri:           "github.com/example/repo",
		IncludedPaths: []string{"*.tf"},
	}

	skipFunc := createSkipFunc("/tmp/src", sourceSpec)

	// .tf file should NOT be skipped (matches included pattern).
	info := &mockFileInfo{name: "main.tf", isDir: false}
	skip, err := skipFunc(info, "/tmp/src/main.tf", "/tmp/dst/main.tf")
	assert.NoError(t, err)
	assert.False(t, skip, "*.tf files should not be skipped")

	// .go file SHOULD be skipped (doesn't match included pattern).
	info = &mockFileInfo{name: "main.go", isDir: false}
	skip, err = skipFunc(info, "/tmp/src/main.go", "/tmp/dst/main.go")
	assert.NoError(t, err)
	assert.True(t, skip, "*.go files should be skipped when only *.tf is included")

	// Directories should NOT be skipped (need to traverse for included files).
	info = &mockFileInfo{name: "modules", isDir: true}
	skip, err = skipFunc(info, "/tmp/src/modules", "/tmp/dst/modules")
	assert.NoError(t, err)
	assert.False(t, skip, "Directories should not be skipped with included_paths")
}

// TestCreateSkipFunc_ExcludedPaths tests skip function with excluded_paths patterns.
func TestCreateSkipFunc_ExcludedPaths(t *testing.T) {
	sourceSpec := &schema.VendorComponentSource{
		Uri:           "github.com/example/repo",
		ExcludedPaths: []string{"*.md", "*.txt"},
	}

	skipFunc := createSkipFunc("/tmp/src", sourceSpec)

	// .tf file should NOT be skipped (not in excluded patterns).
	info := &mockFileInfo{name: "main.tf", isDir: false}
	skip, err := skipFunc(info, "/tmp/src/main.tf", "/tmp/dst/main.tf")
	assert.NoError(t, err)
	assert.False(t, skip, "*.tf files should not be skipped")

	// .md file SHOULD be skipped (matches excluded pattern).
	info = &mockFileInfo{name: "README.md", isDir: false}
	skip, err = skipFunc(info, "/tmp/src/README.md", "/tmp/dst/README.md")
	assert.NoError(t, err)
	assert.True(t, skip, "*.md files should be skipped")

	// .txt file SHOULD be skipped (matches excluded pattern).
	info = &mockFileInfo{name: "notes.txt", isDir: false}
	skip, err = skipFunc(info, "/tmp/src/notes.txt", "/tmp/dst/notes.txt")
	assert.NoError(t, err)
	assert.True(t, skip, "*.txt files should be skipped")
}
