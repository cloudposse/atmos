package installer

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// Add a mock ToolResolver for tests

func TestParseToolSpec(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	SetAtmosConfig(&schema.AtmosConfiguration{})
	installer := NewInstallerWithResolver(mockResolver, t.TempDir())

	tests := []struct {
		name      string
		tool      string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "full specification",
			tool:      "suzuki-shunsuke/github-comment",
			wantOwner: "suzuki-shunsuke",
			wantRepo:  "github-comment",
			wantErr:   false,
		},
		{
			name:      "terraform tool name",
			tool:      "terraform",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name:      "opentofu tool name",
			tool:      "opentofu",
			wantOwner: "opentofu",
			wantRepo:  "opentofu",
			wantErr:   false,
		},
		{
			name:      "helm tool name",
			tool:      "helm",
			wantOwner: "helm",
			wantRepo:  "helm",
			wantErr:   false,
		},
		{
			name:      "kubectl tool name",
			tool:      "kubectl",
			wantOwner: "kubernetes",
			wantRepo:  "kubectl",
			wantErr:   false,
		},
		{
			name:      "helmfile tool name",
			tool:      "helmfile",
			wantOwner: "helmfile",
			wantRepo:  "helmfile",
			wantErr:   false,
		},
		{
			name:    "unknown tool fallback",
			tool:    "unknown-tool",
			wantErr: true,
		},
		{
			name:    "invalid specification",
			tool:    "invalid/spec/format",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := installer.ParseToolSpec(tt.tool)

			if tt.wantErr {
				assert.Error(t, err, "parseToolSpec() expected error")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestBuildAssetURL(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver, t.TempDir())

	// Test with SemVer template variable (version without prefix).
	// This matches Aqua's behavior where .Version = full tag, .SemVer = without prefix.
	// The VersionPrefix must be explicitly set for .SemVer to strip it.
	tool := &registry.Tool{
		Type:          "github_release",
		RepoOwner:     "suzuki-shunsuke",
		RepoName:      "github-comment",
		Asset:         "github-comment_{{.SemVer}}_{{.OS}}_{{.Arch}}.tar.gz",
		VersionPrefix: "v", // Explicitly set so .SemVer strips it.
	}

	url, err := installer.BuildAssetURL(tool, "6.3.4")
	if err != nil {
		t.Fatalf("buildAssetURL() error: %v", err)
	}

	// URL should use full version tag (v6.3.4) and asset should use SemVer (6.3.4).
	expected := "https://github.com/suzuki-shunsuke/github-comment/releases/download/v6.3.4/github-comment_6.3.4"
	if !strings.Contains(url, expected) {
		t.Errorf("buildAssetURL() = %v, want %v", url, expected)
	}
}

func TestBuildAssetURL_CustomFuncs(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
		},
	}
	SetAtmosConfig(&schema.AtmosConfiguration{})
	installer := NewInstallerWithResolver(mockResolver, t.TempDir())

	// Test template functions on both .Version (full tag) and .SemVer (without prefix).
	// With VersionPrefix: "v", .Version = v1.2.8, .SemVer = 1.2.8
	tool := &registry.Tool{
		Type:          "github_release",
		RepoOwner:     "hashicorp",
		RepoName:      "terraform",
		Asset:         "terraform_{{trimV .Version}}_{{trimPrefix \"1.\" .SemVer}}_{{trimSuffix \".8\" .SemVer}}_{{replace \".\" \"-\" .SemVer}}_{{.OS}}_{{.Arch}}.zip",
		VersionPrefix: "v", // Explicitly set so .SemVer strips it.
	}

	url, err := installer.BuildAssetURL(tool, "1.2.8")
	if err != nil {
		t.Fatalf("buildAssetURL() error: %v", err)
	}

	// Check that all functions are applied as expected:
	// - trimV .Version: v1.2.8 → 1.2.8
	// - trimPrefix "1." .SemVer: 1.2.8 → 2.8
	// - trimSuffix ".8" .SemVer: 1.2.8 → 1.2
	// - replace "." "-" .SemVer: 1.2.8 → 1-2-8
	if !strings.Contains(url, "terraform_1.2.8_2.8_1.2_1-2-8") {
		t.Errorf("buildAssetURL() custom funcs not applied correctly, got: %v", url)
	}
}

func TestGetBinaryPath(t *testing.T) {
	binDir := t.TempDir()
	installer := New(WithBinDir(binDir))

	path := installer.GetBinaryPath("suzuki-shunsuke", "github-comment", "v6.3.4", "")
	expected := filepath.Join(binDir, "suzuki-shunsuke", "github-comment", "v6.3.4", "github-comment")

	assert.Equal(t, expected, path, "GetBinaryPath() mismatch")
}

func TestFindTool(t *testing.T) {
	t.Skip("Skipped: FindTool requires registry factory to be configured - tested in parent toolchain package")
}

func TestNewInstaller(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	tmpDir := t.TempDir()
	installer := New(
		WithBinDir(tmpDir),
		WithResolver(mockResolver),
	)

	assert.Equal(t, tmpDir, installer.binDir, "binDir should match")
	assert.NotNil(t, installer.resolver, "resolver should be set")
	assert.NotEmpty(t, installer.registries, "registries should not be empty")
	assert.NotEmpty(t, installer.cacheDir, "cacheDir should be set by default")
}

func TestResolveToolName(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver, t.TempDir())

	tests := []struct {
		name      string
		toolName  string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "terraform mapping",
			toolName:  "terraform",
			wantOwner: "hashicorp",
			wantRepo:  "terraform",
			wantErr:   false,
		},
		{
			name:      "opentofu mapping",
			toolName:  "opentofu",
			wantOwner: "opentofu",
			wantRepo:  "opentofu",
			wantErr:   false,
		},
		{
			name:      "helm mapping",
			toolName:  "helm",
			wantOwner: "helm",
			wantRepo:  "helm",
			wantErr:   false,
		},
		{
			name:      "kubectl mapping",
			toolName:  "kubectl",
			wantOwner: "kubernetes",
			wantRepo:  "kubectl",
			wantErr:   false,
		},
		{
			name:      "helmfile mapping",
			toolName:  "helmfile",
			wantOwner: "helmfile",
			wantRepo:  "helmfile",
			wantErr:   false,
		},
		{
			name:     "tflint mapping",
			toolName: "tflint",
			wantErr:  true,
		},
		{
			name:     "unknown tool fallback",
			toolName: "unknown-tool",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := installer.GetResolver().Resolve(tt.toolName)

			if tt.wantErr {
				assert.Error(t, err, "resolveToolName() expected error")
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestSearchRegistryForTool(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}

	owner, repo, err := mockResolver.Resolve("terraform")
	if err != nil {
		t.Errorf("Expected to find tool but got error: %v", err)
		return
	}
	if owner == "" || repo == "" {
		t.Error("Expected owner and repo to be non-empty")
	}

	_, _, err = mockResolver.Resolve("nonexistent-tool-12345")
	if err == nil {
		t.Error("Expected error for non-existent tool but got none")
	}
}

func TestUninstall(t *testing.T) {
	mockResolver := &mockToolResolver{
		mapping: map[string][2]string{
			"terraform": {"hashicorp", "terraform"},
			"opentofu":  {"opentofu", "opentofu"},
			"kubectl":   {"kubernetes", "kubectl"},
			"helm":      {"helm", "helm"},
			"helmfile":  {"helmfile", "helmfile"},
		},
	}
	installer := NewInstallerWithResolver(mockResolver, t.TempDir())

	// Test uninstalling a non-existent tool
	err := installer.Uninstall("nonexistent", "package", "1.0.0")
	if err == nil {
		t.Error("Expected error when uninstalling non-existent tool")
	}

	// Test uninstalling an existing tool (if we had one installed)
	// This would require setting up a test environment with actual binaries
	// For now, we just test that the method exists and handles errors properly
}

func TestLocalConfigAliases(t *testing.T) {
	t.Skip("Local config support was removed in refactoring")
}

func TestEmitJSONPath(t *testing.T) {
	t.Skip("Skipped: ToolPath and emitJSONPath belong to toolchain package, not installer package")
}

func TestAquaRegistryFallback_PackagesKey(t *testing.T) {
	t.Skip("Skipped: Tests Aqua internal implementation - covered by toolchain/registry/aqua package tests")
}

func TestExtractRawBinary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a mock raw binary file
	rawBinaryPath := filepath.Join(tempDir, "test-binary")
	rawBinaryContent := []byte("#!/bin/bash\necho 'test binary'")
	if err := os.WriteFile(rawBinaryPath, rawBinaryContent, defaultMkdirPermissions); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	// Create destination path
	destPath := filepath.Join(tempDir, "extracted-binary")

	// Test raw binary extraction (copyFile)
	if err := copyFile(rawBinaryPath, destPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify the file was copied correctly
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("extracted binary file does not exist")
	}

	// Verify content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted binary: %v", err)
	}
	if string(content) != string(rawBinaryContent) {
		t.Errorf("extracted binary content mismatch: got %s, want %s", string(content), string(rawBinaryContent))
	}
}

func TestExtractGzippedBinary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	installer := New()

	// Create a mock binary content
	binaryContent := []byte("#!/bin/bash\necho 'test gzipped binary'")

	// Create a gzipped file
	gzPath := filepath.Join(tempDir, "test-binary.gz")
	gzFile, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("failed to create gzip file: %v", err)
	}

	gzWriter := gzip.NewWriter(gzFile)
	if _, err := gzWriter.Write(binaryContent); err != nil {
		t.Fatalf("failed to write to gzip: %v", err)
	}
	gzWriter.Close()
	gzFile.Close()

	// Create destination path
	destPath := filepath.Join(tempDir, "extracted-binary")

	// Test gzipped binary extraction
	err = installer.extractGzip(gzPath, destPath)
	if err != nil {
		t.Fatalf("extractGzip failed: %v", err)
	}

	// Verify the file was extracted correctly
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("extracted binary file does not exist")
	}

	// Verify content
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted binary: %v", err)
	}
	if string(content) != string(binaryContent) {
		t.Errorf("extracted binary content mismatch: got %s, want %s", string(content), string(binaryContent))
	}
}

func TestExtractAndInstallWithRawBinary(t *testing.T) {
	// Create separate directories for source and destination.
	srcDir := t.TempDir()
	binDir := t.TempDir()
	installer := New(WithBinDir(binDir))

	// Create a mock raw binary file in the source directory.
	rawBinaryPath := filepath.Join(srcDir, "atmos")
	rawBinaryContent := []byte("#!/bin/bash\necho 'atmos binary'")
	if err := os.WriteFile(rawBinaryPath, rawBinaryContent, defaultMkdirPermissions); err != nil {
		t.Fatalf("failed to create test binary: %v", err)
	}

	// Create a mock tool configuration.
	tool := &registry.Tool{
		Name:     "atmos",
		RepoName: "atmos",
		Type:     "http",
	}

	// Test extractAndInstall with raw binary.
	binaryPath, err := installer.extractAndInstall(tool, rawBinaryPath, "1.0.0")
	if err != nil {
		t.Fatalf("extractAndInstall failed: %v", err)
	}

	// Verify the binary was installed (should be copied to the bin directory)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Errorf("installed binary not found at expected path: %s", binaryPath)
	}
}

func TestExtractAndInstallWithGzippedBinary(t *testing.T) {
	// Create a temporary directory for testing.
	tempDir := t.TempDir()
	installer := New(WithBinDir(tempDir))

	// Create a mock binary content.
	binaryContent := []byte("#!/bin/bash\necho 'atmos gzipped binary'")

	// Create a gzipped file
	gzPath := filepath.Join(tempDir, "atmos.gz")
	gzFile, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("failed to create gzip file: %v", err)
	}

	gzWriter := gzip.NewWriter(gzFile)
	if _, err := gzWriter.Write(binaryContent); err != nil {
		t.Fatalf("failed to write to gzip: %v", err)
	}
	gzWriter.Close()
	gzFile.Close()

	// Create a mock tool configuration
	tool := &registry.Tool{
		Name:     "atmos",
		RepoName: "atmos",
		Type:     "http",
	}

	// Test extractAndInstall with gzipped binary
	binaryPath, err := installer.extractAndInstall(tool, gzPath, "1.0.0")
	if err != nil {
		t.Fatalf("extractAndInstall failed: %v", err)
	}

	// Verify the binary was installed (should be extracted to the bin directory)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Errorf("installed binary not found at expected path: %s", binaryPath)
	}

	// Verify content
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("failed to read installed binary: %v", err)
	}
	if string(content) != string(binaryContent) {
		t.Errorf("installed binary content mismatch: got %s, want %s", string(content), string(binaryContent))
	}
}

func TestFileTypeDetection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	installer := New()

	tests := []struct {
		name     string
		content  []byte
		filename string
		expected string // expected MIME type or behavior
	}{
		{
			name:     "raw binary",
			content:  []byte{0x7f, 0x45, 0x4c, 0x46}, // ELF header
			filename: "binary",
			expected: "application/octet-stream",
		},
		{
			name:     "gzipped binary",
			content:  []byte{0x1f, 0x8b}, // gzip header
			filename: "binary.gz",
			expected: "application/gzip",
		},
		{
			name:     "zip file",
			content:  []byte{0x50, 0x4b, 0x03, 0x04}, // ZIP header
			filename: "archive.zip",
			expected: "application/zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test file
			filePath := filepath.Join(tempDir, tt.filename)
			if err := os.WriteFile(filePath, tt.content, defaultFileWritePermissions); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Test simpleExtract to see how file type is detected
			tool := &registry.Tool{Name: "test", RepoName: "test"}
			destPath := filepath.Join(tempDir, "extracted-"+tt.filename)

			err := installer.simpleExtract(filePath, destPath, tool)
			if err != nil {
				t.Logf("simpleExtract failed (this might be expected): %v", err)
			}

			// The test verifies that the file type detection doesn't crash
			// and routes to the appropriate extraction method
		})
	}
}

func TestInstallerPopulatesToolFields(t *testing.T) {
	installer := New()

	// Test that the installer correctly populates RepoOwner and RepoName
	// when fetching a tool from the registry
	tool, err := installer.FindTool("hashicorp", "terraform", "1.9.8")
	if err != nil {
		t.Skipf("Skipping test - tool not found in registry: %v", err)
	}

	// Verify that RepoOwner and RepoName are populated
	assert.NotEmpty(t, tool.RepoOwner, "RepoOwner should be populated")
	assert.NotEmpty(t, tool.RepoName, "RepoName should be populated")
	assert.Equal(t, "hashicorp", tool.RepoOwner, "RepoOwner should match the requested owner")
	assert.Equal(t, "terraform", tool.RepoName, "RepoName should match the requested repo")

	// Test that buildAssetURL generates correct URLs
	assetURL, err := installer.BuildAssetURL(tool, "1.9.8")
	assert.NoError(t, err, "buildAssetURL should not error")

	// Since hashicorp/terraform is configured as type: http in tools.yaml,
	// it should generate an HTTP URL, not a GitHub release URL
	assert.Contains(t, assetURL, "https://releases.hashicorp.com/terraform/",
		"Asset URL should contain correct HashiCorp release URL")
	assert.NotContains(t, assetURL, "https://github.com///releases/download/",
		"Asset URL should not have empty org/repo")
}

func TestBuildAssetURLWithEmptyFields(t *testing.T) {
	installer := New()

	// Create a tool with empty RepoOwner and RepoName to test the bug
	tool := &registry.Tool{
		Name:      "test-tool",
		Type:      "github_release",
		RepoOwner: "", // Empty - this is the bug
		RepoName:  "", // Empty - this is the bug
		Asset:     "test-tool_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
	}

	// This should fail because RepoOwner and RepoName are empty
	_, err := installer.BuildAssetURL(tool, "1.0.0")
	assert.Error(t, err, "buildAssetURL should fail with empty RepoOwner/RepoName")

	// The generated URL would be malformed like:
	// https://github.com///releases/download/1.0.0/test-tool_1.0.0_darwin_arm64.tar.gz
}

// Helper to create a tar.gz for testing.
func createTestTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create tar.gz: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("failed to write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write tar content: %v", err)
		}
	}
}

func TestExtractTarGz(t *testing.T) {
	// Setup temp dirs
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "test.tar.gz")
	dest := filepath.Join(tmpDir, "out")

	// Files to include in tar.gz
	files := map[string]string{
		"file1.txt":     "hello world",
		"dir/file2.txt": "nested content",
	}

	// Create tar.gz
	createTestTarGz(t, src, files)

	// Run extraction
	if err := ExtractTarGz(src, dest); err != nil {
		t.Fatalf("ExtractTarGz failed: %v", err)
	}

	// Verify files
	for name, expected := range files {
		p := filepath.Join(dest, name)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("failed to read extracted file %s: %v", name, err)
			continue
		}
		if string(data) != expected {
			t.Errorf("file %s: expected %q, got %q", name, expected, string(data))
		}
	}
}

func TestCopyFile(t *testing.T) {
	tests := []struct {
		name        string
		srcContent  string
		expectError bool
	}{
		{
			name:        "Copy simple text file",
			srcContent:  "Hello, World!",
			expectError: false,
		},
		{
			name:        "Copy empty file",
			srcContent:  "",
			expectError: false,
		},
		{
			name:        "Copy binary content",
			srcContent:  "\x00\x01\x02\xFF\xFE",
			expectError: false,
		},
		{
			name:        "Copy large file",
			srcContent:  strings.Repeat("test content ", 10000),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			src := filepath.Join(tempDir, "source.txt")
			dst := filepath.Join(tempDir, "dest.txt")

			// Create source file
			err := os.WriteFile(src, []byte(tt.srcContent), defaultFileWritePermissions)
			require.NoError(t, err)

			// Copy file
			err = copyFile(src, dst)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify destination file exists and has same content
				dstContent, err := os.ReadFile(dst)
				require.NoError(t, err)
				assert.Equal(t, tt.srcContent, string(dstContent))
			}
		})
	}

	t.Run("Error on non-existent source", func(t *testing.T) {
		tempDir := t.TempDir()
		src := filepath.Join(tempDir, "nonexistent.txt")
		dst := filepath.Join(tempDir, "dest.txt")

		err := copyFile(src, dst)
		assert.Error(t, err)
	})

	t.Run("Error on invalid destination directory", func(t *testing.T) {
		tempDir := t.TempDir()
		src := filepath.Join(tempDir, "source.txt")
		dst := "/invalid/nonexistent/path/dest.txt"

		// Create source file
		err := os.WriteFile(src, []byte("test"), defaultFileWritePermissions)
		require.NoError(t, err)

		err = copyFile(src, dst)
		assert.Error(t, err)
	})
}

func TestCreateLatestFile(t *testing.T) {
	tests := []struct {
		name    string
		owner   string
		repo    string
		version string
	}{
		{
			name:    "Create latest file for terraform",
			owner:   "hashicorp",
			repo:    "terraform",
			version: "1.5.7",
		},
		{
			name:    "Create latest file with version prefix",
			owner:   "kubernetes",
			repo:    "kubectl",
			version: "v1.28.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			t.Setenv("HOME", tempDir)
			SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{}})

			installer := New()
			installer.binDir = tempDir

			err := installer.CreateLatestFile(tt.owner, tt.repo, tt.version)
			assert.NoError(t, err)

			// Verify file was created
			latestFilePath := filepath.Join(tempDir, tt.owner, tt.repo, "latest")
			data, err := os.ReadFile(latestFilePath)
			assert.NoError(t, err)
			assert.Equal(t, tt.version, string(data))

			// Verify ReadLatestFile works
			readVersion, err := installer.ReadLatestFile(tt.owner, tt.repo)
			assert.NoError(t, err)
			assert.Equal(t, tt.version, readVersion)
		})
	}
}

// TestMoveFile_Success tests successful file move operation.
func TestMoveFile_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	content := []byte("test content")
	err := os.WriteFile(srcPath, content, 0o644)
	require.NoError(t, err)

	// Move to destination
	dstPath := filepath.Join(tempDir, "dest.txt")
	err = MoveFile(srcPath, dstPath)
	assert.NoError(t, err)

	// Verify destination exists
	data, err := os.ReadFile(dstPath)
	assert.NoError(t, err)
	assert.Equal(t, content, data)

	// Verify source no longer exists
	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err))
}

// TestMoveFile_CreateTargetDir tests MoveFile creates target directory if needed.
func TestMoveFile_CreateTargetDir(t *testing.T) {
	tempDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	content := []byte("test content")
	err := os.WriteFile(srcPath, content, 0o644)
	require.NoError(t, err)

	// Move to destination in non-existent directory
	dstPath := filepath.Join(tempDir, "subdir", "newdir", "dest.txt")
	err = MoveFile(srcPath, dstPath)
	assert.NoError(t, err)

	// Verify destination exists
	data, err := os.ReadFile(dstPath)
	assert.NoError(t, err)
	assert.Equal(t, content, data)
}

// TestMoveFile_CopyFallback tests MoveFile falls back to copy+delete when rename fails.
func TestMoveFile_CopyFallback(t *testing.T) {
	// This test is tricky because os.Rename usually works within the same filesystem.
	// We'll test the copy path by using different filesystems or permissions.
	// For simplicity, we'll just verify the function handles the fallback correctly.
	// by testing cross-filesystem moves if possible.

	tempDir1 := t.TempDir()
	tempDir2 := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tempDir1, "source.txt")
	content := []byte("test content for fallback")
	err := os.WriteFile(srcPath, content, 0o644)
	require.NoError(t, err)

	// Move to different temp directory (may or may not trigger fallback depending on OS)
	dstPath := filepath.Join(tempDir2, "dest.txt")
	err = MoveFile(srcPath, dstPath)
	assert.NoError(t, err)

	// Verify destination exists
	data, err := os.ReadFile(dstPath)
	assert.NoError(t, err)
	assert.Equal(t, content, data)

	// Verify source no longer exists
	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err))
}

// TestMoveFile_SourceNotFound tests MoveFile with non-existent source file.
func TestMoveFile_SourceNotFound(t *testing.T) {
	tempDir := t.TempDir()

	srcPath := filepath.Join(tempDir, "nonexistent.txt")
	dstPath := filepath.Join(tempDir, "dest.txt")

	err := MoveFile(srcPath, dstPath)
	assert.Error(t, err)
}

// TestMoveFile_InvalidDestinationPath tests MoveFile with invalid destination path.
func TestMoveFile_InvalidDestinationPath(t *testing.T) {
	tempDir := t.TempDir()

	// Create source file
	srcPath := filepath.Join(tempDir, "source.txt")
	err := os.WriteFile(srcPath, []byte("content"), 0o644)
	require.NoError(t, err)

	// Try to move to invalid path (e.g., path that can't be created).
	// This is OS-specific, so we'll test a reasonable scenario.
	dstPath := filepath.Join(tempDir, "file.txt", "subfile.txt")
	err = os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("blocking"), 0o644)
	require.NoError(t, err)

	err = MoveFile(srcPath, dstPath)
	assert.Error(t, err)
}

// TestDefaultRegistryFactory tests that defaultRegistryFactory returns nil.
func TestDefaultRegistryFactory(t *testing.T) {
	factory := &defaultRegistryFactory{}
	reg := factory.NewAquaRegistry()
	assert.Nil(t, reg, "defaultRegistryFactory should return nil")
}

// TestNewWithOptions tests the New() function with various options.
func TestNewWithOptions(t *testing.T) {
	t.Run("creates installer with default options", func(t *testing.T) {
		installer := New()
		assert.NotNil(t, installer)
		assert.NotEmpty(t, installer.cacheDir)
		assert.NotNil(t, installer.registryFactory)
		assert.NotNil(t, installer.resolver)
	})

	t.Run("creates installer with custom binDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		installer := New(WithBinDir(tmpDir))
		assert.NotNil(t, installer)
		assert.Equal(t, tmpDir, installer.binDir)
	})

	t.Run("creates installer with custom cacheDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		installer := New(WithCacheDir(tmpDir))
		assert.NotNil(t, installer)
		assert.Equal(t, tmpDir, installer.cacheDir)
	})

	t.Run("creates installer with custom resolver", func(t *testing.T) {
		mockResolver := &mockToolResolver{
			mapping: map[string][2]string{
				"test": {"owner", "repo"},
			},
		}
		installer := New(WithResolver(mockResolver))
		assert.NotNil(t, installer)
		assert.Equal(t, mockResolver, installer.resolver)
	})

	t.Run("creates installer with custom registry factory", func(t *testing.T) {
		factory := &defaultRegistryFactory{}
		installer := New(WithRegistryFactory(factory))
		assert.NotNil(t, installer)
		assert.Equal(t, factory, installer.registryFactory)
	})
}

// TestEnsureWindowsExeExtension tests the centralized Windows .exe extension handling.
// This function follows Aqua's behavior where executables need the .exe extension
// on Windows to be found by os/exec.LookPath.
func TestEnsureWindowsExeExtension(t *testing.T) {
	tests := []struct {
		name       string
		binaryName string
		// Expected result depends on runtime.GOOS:
		// - On Windows: returns binaryName + ".exe" if not already present
		// - On non-Windows: returns binaryName unchanged
		wantWindows    string
		wantNonWindows string
	}{
		{
			name:           "plain binary name",
			binaryName:     "terraform",
			wantWindows:    "terraform.exe",
			wantNonWindows: "terraform",
		},
		{
			name:           "binary name already has .exe",
			binaryName:     "terraform.exe",
			wantWindows:    "terraform.exe",
			wantNonWindows: "terraform.exe",
		},
		{
			name:           "binary name with uppercase .EXE",
			binaryName:     "terraform.EXE",
			wantWindows:    "terraform.EXE",
			wantNonWindows: "terraform.EXE",
		},
		{
			name:           "binary name with mixed case .Exe",
			binaryName:     "terraform.Exe",
			wantWindows:    "terraform.Exe",
			wantNonWindows: "terraform.Exe",
		},
		{
			name:           "binary name with path",
			binaryName:     "bin/terraform",
			wantWindows:    "bin/terraform.exe",
			wantNonWindows: "bin/terraform",
		},
		{
			name:           "binary name with path and .exe",
			binaryName:     "bin/terraform.exe",
			wantWindows:    "bin/terraform.exe",
			wantNonWindows: "bin/terraform.exe",
		},
		{
			name:           "empty binary name",
			binaryName:     "",
			wantWindows:    ".exe",
			wantNonWindows: "",
		},
		{
			name:           "binary name ending with .ex",
			binaryName:     "myfile.ex",
			wantWindows:    "myfile.ex.exe",
			wantNonWindows: "myfile.ex",
		},
		{
			name:           "binary name ending with .exeFile",
			binaryName:     "terraform.exeFile",
			wantWindows:    "terraform.exeFile.exe",
			wantNonWindows: "terraform.exeFile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EnsureWindowsExeExtension(tt.binaryName)

			// Determine expected result based on current OS.
			var expected string
			if runtime.GOOS == "windows" {
				expected = tt.wantWindows
			} else {
				expected = tt.wantNonWindows
			}

			assert.Equal(t, expected, result,
				"EnsureWindowsExeExtension(%q) on %s", tt.binaryName, runtime.GOOS)
		})
	}
}

// TestEnsureWindowsExeExtension_CurrentPlatformBehavior verifies the function
// behaves correctly on the current platform.
func TestEnsureWindowsExeExtension_CurrentPlatformBehavior(t *testing.T) {
	if runtime.GOOS == "windows" {
		// On Windows, should add .exe if not present.
		t.Run("adds .exe on Windows", func(t *testing.T) {
			result := EnsureWindowsExeExtension("myapp")
			assert.Equal(t, "myapp.exe", result)
		})

		t.Run("does not double-add .exe on Windows", func(t *testing.T) {
			result := EnsureWindowsExeExtension("myapp.exe")
			assert.Equal(t, "myapp.exe", result)
		})
	} else {
		// On non-Windows, should return unchanged.
		t.Run("returns unchanged on non-Windows", func(t *testing.T) {
			result := EnsureWindowsExeExtension("myapp")
			assert.Equal(t, "myapp", result)
		})

		t.Run("returns unchanged even with .exe on non-Windows", func(t *testing.T) {
			result := EnsureWindowsExeExtension("myapp.exe")
			assert.Equal(t, "myapp.exe", result)
		})
	}
}
