package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCheckComponentExcludes_SimpleFilename tests that a simple filename pattern
// like "providers.tf" correctly excludes that file.
// This is the reported bug: simple filename patterns don't work.
func TestCheckComponentExcludes_SimpleFilename(t *testing.T) {
	tests := []struct {
		name         string
		excludePaths []string
		trimmedSrc   string
		expectSkip   bool
		description  string
	}{
		{
			name:         "simple filename pattern excludes exact match",
			excludePaths: []string{"providers.tf"},
			trimmedSrc:   "providers.tf",
			expectSkip:   true,
			description:  "BUG: 'providers.tf' pattern should match file 'providers.tf'",
		},
		{
			name:         "simple filename pattern does not exclude different file",
			excludePaths: []string{"providers.tf"},
			trimmedSrc:   "main.tf",
			expectSkip:   false,
			description:  "Different filename should not be excluded",
		},
		{
			name:         "glob pattern with double-star prefix matches",
			excludePaths: []string{"**/context.tf"},
			trimmedSrc:   "context.tf",
			expectSkip:   true,
			description:  "'**/context.tf' should match 'context.tf' at root",
		},
		{
			name:         "glob pattern with double-star matches nested file",
			excludePaths: []string{"**/context.tf"},
			trimmedSrc:   "modules/iam/context.tf",
			expectSkip:   true,
			description:  "'**/context.tf' should match nested 'context.tf'",
		},
		{
			name:         "multiple exclude patterns - first matches",
			excludePaths: []string{"providers.tf", "context.tf"},
			trimmedSrc:   "providers.tf",
			expectSkip:   true,
			description:  "First pattern should match",
		},
		{
			name:         "multiple exclude patterns - second matches",
			excludePaths: []string{"providers.tf", "context.tf"},
			trimmedSrc:   "context.tf",
			expectSkip:   true,
			description:  "Second pattern should match",
		},
		{
			name:         "wildcard extension pattern",
			excludePaths: []string{"*.md"},
			trimmedSrc:   "README.md",
			expectSkip:   true,
			description:  "'*.md' should match 'README.md'",
		},
		{
			name:         "path with subdirectory - exact match",
			excludePaths: []string{"modules/iam/providers.tf"},
			trimmedSrc:   "modules/iam/providers.tf",
			expectSkip:   true,
			description:  "Exact path pattern should match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock absolute path (this simulates what the actual code does).
			// The src parameter in the real code is a full path like /tmp/atmos-vendor-xyz/providers.tf.
			// We need to test that the pattern matches against trimmedSrc, not src.
			tempDir := "/var/folders/abc/atmos-vendor-12345"
			src := filepath.Join(tempDir, tt.trimmedSrc)

			skip, err := checkComponentExcludes(tt.excludePaths, src, tt.trimmedSrc)
			require.NoError(t, err, "checkComponentExcludes should not return error")
			assert.Equal(t, tt.expectSkip, skip, tt.description)
		})
	}
}

// TestCreateComponentSkipFunc_ExcludeAndIncludeCombined tests that when both
// excluded_paths AND included_paths are specified, both filters are applied correctly.
// This is the second bug: early return skips include filtering.
func TestCreateComponentSkipFunc_ExcludeAndIncludeCombined(t *testing.T) {
	tempDir := t.TempDir()

	// Create a vendor spec with both excludes and includes.
	vendorSpec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			IncludedPaths: []string{"**/*.tf"},      // Only include .tf files
			ExcludedPaths: []string{"providers.tf"}, // But exclude providers.tf
		},
	}

	skipFunc := createComponentSkipFunc(tempDir, vendorSpec)

	tests := []struct {
		name       string
		filename   string
		expectSkip bool
		reason     string
	}{
		{
			name:       "main.tf is included (matches include, not excluded)",
			filename:   "main.tf",
			expectSkip: false,
			reason:     "main.tf matches *.tf include and is not in excludes",
		},
		{
			name:       "providers.tf is excluded (matches exclude pattern)",
			filename:   "providers.tf",
			expectSkip: true,
			reason:     "providers.tf is in excluded_paths",
		},
		{
			name:       "README.md is excluded (does not match include)",
			filename:   "README.md",
			expectSkip: true,
			reason:     "README.md does not match **/*.tf include pattern",
		},
		{
			name:       "nested main.tf is included",
			filename:   "modules/vpc/main.tf",
			expectSkip: false,
			reason:     "Nested .tf file matches include and is not excluded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the file path as it would be during vendoring.
			srcPath := filepath.Join(tempDir, tt.filename)

			// Create a mock FileInfo (we only need the path for the skip function).
			info := &mockFileInfo{name: filepath.Base(tt.filename), isDir: false}

			skip, err := skipFunc(info, srcPath, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expectSkip, skip, tt.reason)
		})
	}
}

// TestCreateComponentSkipFunc_ExcludeOnly tests that excluded_paths work correctly
// when included_paths is not specified (all files included except excluded ones).
func TestCreateComponentSkipFunc_ExcludeOnly(t *testing.T) {
	tempDir := t.TempDir()

	vendorSpec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			ExcludedPaths: []string{
				"providers.tf",
				"policy-TerraformUpdateAccess.tf",
				"policy-Identity-role-TeamAccess.tf",
			},
		},
	}

	skipFunc := createComponentSkipFunc(tempDir, vendorSpec)

	tests := []struct {
		name       string
		filename   string
		expectSkip bool
	}{
		{"main.tf included", "main.tf", false},
		{"providers.tf excluded", "providers.tf", true},
		{"policy-TerraformUpdateAccess.tf excluded", "policy-TerraformUpdateAccess.tf", true},
		{"policy-Identity-role-TeamAccess.tf excluded", "policy-Identity-role-TeamAccess.tf", true},
		{"variables.tf included", "variables.tf", false},
		{"README.md included", "README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcPath := filepath.Join(tempDir, tt.filename)
			info := &mockFileInfo{name: filepath.Base(tt.filename), isDir: false}

			skip, err := skipFunc(info, srcPath, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expectSkip, skip)
		})
	}
}

// TestCreateComponentSkipFunc_IncludeOnly tests that included_paths work correctly
// when excluded_paths is not specified.
func TestCreateComponentSkipFunc_IncludeOnly(t *testing.T) {
	tempDir := t.TempDir()

	vendorSpec := &schema.VendorComponentSpec{
		Source: schema.VendorComponentSource{
			IncludedPaths: []string{"**/*.tf", "**/*.tfvars"},
		},
	}

	skipFunc := createComponentSkipFunc(tempDir, vendorSpec)

	tests := []struct {
		name       string
		filename   string
		expectSkip bool
	}{
		{"main.tf included", "main.tf", false},
		{"vars.tfvars included", "vars.tfvars", false},
		{"README.md excluded (not in includes)", "README.md", true},
		{"nested main.tf included", "modules/vpc/main.tf", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcPath := filepath.Join(tempDir, tt.filename)
			info := &mockFileInfo{name: filepath.Base(tt.filename), isDir: false}

			skip, err := skipFunc(info, srcPath, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expectSkip, skip)
		})
	}
}

// mockFileInfo is defined in copy_glob_error_paths_test.go and reused here.

// TestShouldExcludeFile tests the shouldExcludeFile function from vendor_utils.go.
// This ensures the same fix is applied consistently to both component and general vendor logic.
func TestShouldExcludeFile(t *testing.T) {
	tests := []struct {
		name         string
		excludePaths []string
		trimmedSrc   string
		expectSkip   bool
	}{
		{
			name:         "simple filename pattern excludes exact match",
			excludePaths: []string{"providers.tf"},
			trimmedSrc:   "providers.tf",
			expectSkip:   true,
		},
		{
			name:         "glob pattern with double-star",
			excludePaths: []string{"**/context.tf"},
			trimmedSrc:   "modules/iam/context.tf",
			expectSkip:   true,
		},
		{
			name:         "wildcard extension pattern",
			excludePaths: []string{"*.md"},
			trimmedSrc:   "README.md",
			expectSkip:   true,
		},
		{
			name:         "non-matching pattern",
			excludePaths: []string{"providers.tf"},
			trimmedSrc:   "main.tf",
			expectSkip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock absolute path.
			tempDir := "/var/folders/abc/atmos-vendor-12345"
			src := filepath.Join(tempDir, tt.trimmedSrc)

			skip, err := shouldExcludeFile(src, tt.excludePaths, tt.trimmedSrc)
			require.NoError(t, err)
			assert.Equal(t, tt.expectSkip, skip)
		})
	}
}

// TestGenerateSkipFunction_ExcludeAndInclude tests the generateSkipFunction from vendor_utils.go
// to ensure both excludes and includes are applied correctly.
func TestGenerateSkipFunction_ExcludeAndInclude(t *testing.T) {
	tempDir := t.TempDir()

	source := &schema.AtmosVendorSource{
		IncludedPaths: []string{"**/*.tf"},
		ExcludedPaths: []string{"providers.tf"},
	}

	skipFunc := generateSkipFunction(tempDir, source)

	tests := []struct {
		name       string
		filename   string
		expectSkip bool
	}{
		{"main.tf included", "main.tf", false},
		{"providers.tf excluded", "providers.tf", true},
		{"README.md excluded (not in includes)", "README.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srcPath := filepath.Join(tempDir, tt.filename)
			info := &mockFileInfo{name: filepath.Base(tt.filename), isDir: false}

			skip, err := skipFunc(info, srcPath, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expectSkip, skip)
		})
	}
}
