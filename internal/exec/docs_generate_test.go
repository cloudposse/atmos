// exec_test.go
package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIsLikelyRemote verifies that isLikelyRemote returns expected booleans.
func TestIsLikelyRemote(t *testing.T) {
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
		if got := isLikelyRemote(tt.input); got != tt.expected {
			t.Errorf("isLikelyRemote(%q) = %v; want %v", tt.input, got, tt.expected)
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

// TestFindSingleFileInDir tests three cases: no files, one file, and multiple files.
func TestFindSingleFileInDir(t *testing.T) {
	// Case: No files.
	emptyDir, err := os.MkdirTemp("", "test-findSingleFileNoFiles")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(emptyDir)
	_, err = findSingleFileInDir(emptyDir)
	if err == nil {
		t.Errorf("Expected error for no files, got nil")
	}

	// Case: One file.
	oneFileDir, err := os.MkdirTemp("", "test-findSingleFileOneFile")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(oneFileDir)
	filePath := filepath.Join(oneFileDir, "file.txt")
	if err := os.WriteFile(filePath, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := findSingleFileInDir(oneFileDir)
	if err != nil {
		t.Errorf("Expected no error for one file, got %v", err)
	}
	if got != filePath {
		t.Errorf("Expected %q, got %q", filePath, got)
	}

	// Case: Multiple files.
	multiDir, err := os.MkdirTemp("", "test-findSingleFileMultipleFiles")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(multiDir)
	f1 := filepath.Join(multiDir, "file1.txt")
	f2 := filepath.Join(multiDir, "file2.txt")
	if err := os.WriteFile(f1, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = findSingleFileInDir(multiDir)
	if err == nil {
		t.Errorf("Expected error for multiple files, got nil")
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
	docsGen := schema.DocsGenerateReadme{
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

// stubRenderer is a simple implementation of TemplateRenderer that returns a fixed string.
type stubRenderer struct{}

func (s stubRenderer) Render(tmplName, tmplValue string, mergedData map[string]interface{}, ignoreMissing bool) (string, error) {
	// For testing, we simply return a string that includes a value from mergedData.
	if name, ok := mergedData["name"].(string); ok {
		return "stub rendered for " + name, nil
	}
	return "stub rendered", nil
}

func TestGenerateReadme_WithInjectedRenderer(t *testing.T) {
	// Create a temporary target directory.
	targetDir, err := os.MkdirTemp("", "test-generateReadme")
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
		Settings: schema.AtmosSettings{
			Docs: schema.Docs{
				Generate: struct {
					Readme schema.DocsGenerateReadme `yaml:"readme" json:"readme" mapstructure:"readme"`
				}{
					Readme: schema.DocsGenerateReadme{
						BaseDir:  ".",
						Input:    []string{tmpYAML.Name()},
						Template: "", // Use default template.
						Output:   "TEST_README.md",
						Terraform: schema.TerraformDocsReadmeSettings{
							Enabled: false,
						},
					},
				},
			},
		},
	}

	// Call generateReadme with our injected stubRenderer.
	err = generateReadme(&atmosConfig, targetDir, &atmosConfig.Settings.Docs.Generate.Readme, stubRenderer{})
	if err != nil {
		t.Fatalf("generateReadme failed: %v", err)
	}
	outputPath := filepath.Join(targetDir, "TEST_README.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated README: %v", err)
	}
	expected := "stub rendered for TestProject"
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
