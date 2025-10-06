// exec_test.go
package exec

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsRemoteSource verifies that IsRemoteSource returns expected booleans.
func TestIsRemoteSource(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"git::some/url", true},
		{"github.com/foo", true},
		{"git@github.com:foo", true},
		{"local/file/path", false},
		{"C:\\path\\to\\file", false},
	}
	for _, tt := range tests {
		if got := isRemoteSource(tt.input); got != tt.expected {
			t.Errorf("IsRemoteSource(%q) = %v; want %v", tt.input, got, tt.expected)
		}
	}
}

// TestGetTerraformSource tests getTerraformSource in valid, invalid, and empty cases.
func TestGetTerraformSource(t *testing.T) {
	// Create a temporary base directory.
	baseDir, err := os.MkdirTemp("", "test-getTerraformSource")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)

	// Create a valid subdirectory.
	validDir := filepath.Join(baseDir, "valid")
	if err := os.Mkdir(validDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Valid case.
	got, err := getTerraformSource(baseDir, "valid")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if got != validDir {
		t.Errorf("Expected %q, got %q", validDir, got)
	}

	// Invalid case: directory does not exist.
	_, err = getTerraformSource(baseDir, "nonexistent")
	if err == nil {
		t.Errorf("Expected error for nonexistent directory, got nil")
	}
	if !strings.Contains(err.Error(), "source directory does not exist") {
		t.Errorf("Unexpected error message: %v", err)
	}

	// Empty source should return baseDir.
	got, err = getTerraformSource(baseDir, "")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if got != baseDir {
		t.Errorf("Expected %q, got %q", baseDir, got)
	}
}

// TestParseYAMLAndBytes tests both parseYAML and parseYAMLBytes.
func TestParseYAMLAndBytes(t *testing.T) {
	content := "key: value\nnumber: 123"
	tmpFile, err := os.CreateTemp("", "test-parseYAML")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	data, err := parseYAML(tmpFile.Name())
	if err != nil {
		t.Errorf("parseYAML returned error: %v", err)
	}
	if data["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", data["key"])
	}

	data2, err := parseYAMLBytes([]byte(content))
	if err != nil {
		t.Errorf("parseYAMLBytes returned error: %v", err)
	}
	if data2["number"] != 123 {
		t.Errorf("Expected number 123, got %v", data2["number"])
	}
}

// TestDownloadSource_Success tests downloadSource using a local file.
// We supply a temporary file as source so that getter.GetFile copies it.
func TestDownloadSource_Success(t *testing.T) {
	// Create a temporary file to act as our source.
	tmpSrc, err := os.CreateTemp("", "test-downloadSource")
	if err != nil {
		t.Fatal(err)
	}
	content := "dummy content"
	if _, err := tmpSrc.WriteString(content); err != nil {
		t.Fatal(err)
	}
	tmpSrc.Close()

	// Call downloadSource with an absolute path (so no joining happens).
	atmosConfig := schema.AtmosConfiguration{}
	localPath, tempDir, err := downloadSource(&atmosConfig, tmpSrc.Name(), "")
	if err != nil {
		t.Fatalf("downloadSource failed: %v", err)
	}
	defer os.RemoveAll(tempDir)
	data, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(data) != content {
		t.Errorf("Expected content %q, got %q", content, string(data))
	}
}

// TestGetTemplateContent tests getTemplateContent using a local file.
func TestGetTemplateContent(t *testing.T) {
	// Create a temporary file with known content.
	tmpFile, err := os.CreateTemp("", "test-template")
	if err != nil {
		t.Fatal(err)
	}
	templateContent := "This is a template."
	if _, err := tmpFile.WriteString(templateContent); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	atmosConfig := schema.AtmosConfiguration{}
	got, err := getTemplateContent(&atmosConfig, tmpFile.Name(), "")
	if err != nil {
		t.Fatalf("getTemplateContent failed: %v", err)
	}
	if got != templateContent {
		t.Errorf("Expected template %q, got %q", templateContent, got)
	}
}

// TestApplyTerraformDocs_Disabled tests that when terraform docs are disabled, nothing is added.
func TestApplyTerraformDocs_Disabled(t *testing.T) {
	docsGen := schema.DocsGenerate{
		Terraform: schema.TerraformDocsReadmeSettings{
			Enabled: false,
		},
	}
	mergedData := make(map[string]any)
	err := applyTerraformDocs("", &docsGen, mergedData)
	if err != nil {
		t.Errorf("Expected no error when terraform docs disabled, got %v", err)
	}
	if _, ok := mergedData["terraform_docs"]; ok {
		t.Errorf("Expected no terraform_docs key when disabled")
	}
}

// mockRenderer is a mock implementation of TemplateRenderer for testing.
// It uses dependency injection to replace the real renderer and returns
// a simplified string incorporating test data from `mergedData`.
type mockRenderer struct{}

func (s mockRenderer) Render(tmplName, tmplValue string, mergedData map[string]interface{}, ignoreMissing bool) (string, error) {
	// For testing, we simply return a string that includes a value from mergedData.
	if name, ok := mergedData["name"].(string); ok {
		return "mock rendered for " + name, nil
	}
	return "mock rendered", nil
}

func TestGenerateDocument_WithInjectedRenderer(t *testing.T) {
	// Create a temporary target directory.
	targetDir, err := os.MkdirTemp("", "test-generateDocument")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(targetDir)

	// Create a temporary YAML file to act as docs input.
	tmpYAML, err := os.CreateTemp("", "test-docs-input")
	if err != nil {
		t.Fatal(err)
	}
	// Minimal YAML content with "name" and "description" fields.
	yamlContent := "name: TestProject\ndescription: \"A test project\"\n"
	if _, err := tmpYAML.WriteString(yamlContent); err != nil {
		t.Fatal(err)
	}
	tmpYAML.Close()

	// Create a dummy atmosConfig with minimal Docs configuration.
	atmosConfig := schema.AtmosConfiguration{
		Docs: schema.Docs{
			Generate: map[string]schema.DocsGenerate{
				"readme": {
					BaseDir:  ".",
					Input:    []any{tmpYAML.Name()},
					Template: "", // Use default template.
					Output:   "TEST_README.md",
					Terraform: schema.TerraformDocsReadmeSettings{
						Enabled: false,
					},
				},
			},
		},
	}

	docsGenerate := atmosConfig.Docs.Generate["readme"]
	// Call generateDocument with our injected stubRenderer.
	err = generateDocument(&atmosConfig, targetDir, &docsGenerate, mockRenderer{})
	if err != nil {
		t.Fatalf("generateDocument failed: %v", err)
	}
	outputPath := filepath.Join(targetDir, "TEST_README.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated README: %v", err)
	}
	expected := "mock rendered for TestProject"
	if !strings.Contains(string(data), expected) {
		t.Errorf("Expected output to contain %q, got %q", expected, string(data))
	}
}

// TestRunTerraformDocs_Error tests runTerraformDocs by providing an invalid directory.
func TestRunTerraformDocs_Error(t *testing.T) {
	_, err := runTerraformDocs("nonexistent_directory", &schema.TerraformDocsReadmeSettings{})
	if err == nil {
		t.Errorf("Expected error for invalid terraform module directory, got nil")
	}
}

// TestResolvePath tests the resolvePath function with various path types.
func TestResolvePath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		baseDir     string
		expectError bool
		checkAbs    bool
	}{
		{
			name:        "empty path returns error",
			path:        "",
			baseDir:     "/tmp",
			expectError: true,
		},
		{
			name:        "absolute path",
			path:        "/absolute/path/to/file",
			baseDir:     "/tmp",
			expectError: false,
			checkAbs:    true,
		},
		{
			name:        "explicit relative with ./",
			path:        "./relative/path",
			baseDir:     "/tmp",
			expectError: false,
			checkAbs:    true,
		},
		{
			name:        "explicit relative with ../",
			path:        "../relative/path",
			baseDir:     "/tmp",
			expectError: false,
			checkAbs:    true,
		},
		{
			name:        "implicit relative path joins with baseDir",
			path:        "relative/file.txt",
			baseDir:     "/tmp/base",
			expectError: false,
			checkAbs:    true,
		},
		{
			name:        "simple filename joins with baseDir",
			path:        "file.txt",
			baseDir:     "/tmp/base",
			expectError: false,
			checkAbs:    true,
		},
		{
			name:        "path with spaces",
			path:        "path with spaces/file.txt",
			baseDir:     "/tmp/base",
			expectError: false,
			checkAbs:    true,
		},
		{
			name:        "current directory as path",
			path:        ".",
			baseDir:     "/tmp/base",
			expectError: false,
			checkAbs:    true,
		},
		{
			name:        "parent directory as path",
			path:        "..",
			baseDir:     "/tmp/base",
			expectError: false,
			checkAbs:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolvePath(tt.path, tt.baseDir, "")

			// Check error expectation
			if tt.expectError && err == nil {
				t.Errorf("Expected error, got nil")
				return
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got %v", err)
				return
			}

			// Check absolute path if required
			if tt.checkAbs && result != "" && !filepath.IsAbs(result) {
				t.Errorf("Expected absolute path, got %q", result)
			}
		})
	}
}

// TestResolvePath_JoinBehavior tests that implicit relative paths are correctly joined with baseDir.
func TestResolvePath_JoinBehavior(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		baseDir        string
		expectedSuffix string // What the path should contain (platform-agnostic)
	}{
		{
			name:           "implicit relative joins with baseDir",
			path:           "foo/bar.txt",
			baseDir:        "/tmp/base",
			expectedSuffix: filepath.Join("base", "foo", "bar.txt"),
		},
		{
			name:           "simple filename joins with baseDir",
			path:           "file.txt",
			baseDir:        "/tmp/base",
			expectedSuffix: filepath.Join("base", "file.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolvePath(tt.path, tt.baseDir, "")
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			// Normalize both paths to use forward slashes for comparison
			normalizedResult := filepath.ToSlash(result)
			normalizedExpected := filepath.ToSlash(tt.expectedSuffix)
			if !strings.Contains(normalizedResult, normalizedExpected) {
				t.Errorf("Expected result to contain %q, got %q", normalizedExpected, normalizedResult)
			}
		})
	}
}

// TestMergeInputs covers various merge scenarios: local only, remote overrides local, inline overrides all.
func TestMergeInputs(t *testing.T) {
	tests := []struct {
		name          string
		localContent  string
		remoteContent string
		inlineMap     map[string]any
		expected      map[string]any
	}{
		{
			name: "only local input",
			localContent: `a: 1
common: local
`,
			// no remote, no inline
			remoteContent: "",
			inlineMap:     nil,
			expected: map[string]any{
				"a":      1,
				"common": "local",
			},
		},
		{
			name: "remote overrides local",
			localContent: `a: 1
common: local
`,
			remoteContent: `a: 2
extra: r
common: remote
`,
			inlineMap: nil,
			expected: map[string]any{
				"a":      2,
				"extra":  "r",
				"common": "remote",
			},
		},
		{
			name: "inline overrides all",
			localContent: `a: 1
common: local
`,
			remoteContent: `a: 2
b: 3
common: remote
`,
			inlineMap: map[string]any{
				"common": "inline",
				"c":      4,
			},
			expected: map[string]any{
				"a":      2,
				"b":      3,
				"common": "inline",
				"c":      4,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare base directory
			baseDir := t.TempDir()
			// Write local YAML if any
			var localPath string
			if tc.localContent != "" {
				localPath = filepath.Join(baseDir, "local.yaml")
				if err := os.WriteFile(localPath, []byte(tc.localContent), 0o644); err != nil {
					t.Fatalf("failed to write local: %v", err)
				}
			}

			// Setup remote HTTP server if remoteContent provided
			var remoteURL string
			if tc.remoteContent != "" {
				remoteDir := t.TempDir()
				remoteFile := filepath.Join(remoteDir, "remote.yaml")
				if err := os.WriteFile(remoteFile, []byte(tc.remoteContent), 0o644); err != nil {
					t.Fatalf("failed to write remote: %v", err)
				}
				srv := httptest.NewServer(http.FileServer(http.Dir(remoteDir)))
				defer srv.Close()
				remoteURL = srv.URL + "/remote.yaml"
			}

			// Build Inputs
			var inputs []any
			if localPath != "" {
				inputs = append(inputs, localPath)
			}
			if remoteURL != "" {
				inputs = append(inputs, remoteURL)
			}
			if tc.inlineMap != nil {
				inputs = append(inputs, tc.inlineMap)
			}

			dg := &schema.DocsGenerate{Input: inputs}
			merged, err := mergeInputs(&schema.AtmosConfiguration{}, baseDir, dg)
			if err != nil {
				t.Fatalf("mergeInputs error: %v", err)
			}

			// Verify expected
			for key, exp := range tc.expected {
				got, ok := merged[key]
				if !ok {
					t.Errorf("missing key %q", key)
					continue
				}
				if got != exp {
					t.Errorf("merged[%q] = %v; expected %v", key, got, exp)
				}
			}
		})
	}
}
