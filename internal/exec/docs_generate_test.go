// exec_test.go
package exec

import (
	"encoding/json"
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
	baseDir := t.TempDir()

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
	targetDir := t.TempDir()

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

func TestGenerateDocument_DefaultTemplateFallbackUsesRootData(t *testing.T) {
	targetDir := t.TempDir()
	inputPath := filepath.Join(targetDir, "README.yaml")
	if err := os.WriteFile(inputPath, []byte("name: TestProject\ndescription: A test project\n"), defaultFilePermissions); err != nil {
		t.Fatalf("failed to write input YAML: %v", err)
	}

	docsGenerate := schema.DocsGenerate{
		BaseDir:  ".",
		Input:    []any{inputPath},
		Template: "./missing-template.gotmpl",
		Output:   "README.md",
		Terraform: schema.TerraformDocsReadmeSettings{
			Enabled: false,
		},
	}

	err := generateDocument(&schema.AtmosConfiguration{}, targetDir, &docsGenerate, defaultTemplateRenderer{})
	if err != nil {
		t.Fatalf("generateDocument failed: %v", err)
	}

	outputPath := filepath.Join(targetDir, "README.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated README: %v", err)
	}
	if !strings.Contains(string(data), "# TestProject") {
		t.Errorf("Expected fallback template to render root name, got %q", string(data))
	}
}

// TestGenerateDocument_DefaultTemplateFallback_RealRenderer exercises the default fallback
// template through the real defaultTemplateRenderer (gomplate), not mockRenderer. It guards
// against regressions where the fallback template references bare fields (e.g. ".name")
// instead of going through the "config" datasource, which ProcessTmplWithDatasourcesGomplate
// requires (root "." is bound to the outer Env wrapper, not mergedData).
func TestGenerateDocument_DefaultTemplateFallback_RealRenderer(t *testing.T) {
	targetDir := t.TempDir()

	tmpYAML, err := os.CreateTemp("", "test-docs-input-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpYAML.Name())
	yamlContent := "name: TestProject\ndescription: \"A test project\"\n"
	if _, err := tmpYAML.WriteString(yamlContent); err != nil {
		t.Fatal(err)
	}
	tmpYAML.Close()

	atmosConfig := schema.AtmosConfiguration{
		Docs: schema.Docs{
			Generate: map[string]schema.DocsGenerate{
				"readme": {
					BaseDir:  ".",
					Input:    []any{tmpYAML.Name()},
					Template: "", // No template configured: use the default fallback template.
					Output:   "TEST_README_DEFAULT.md",
					Terraform: schema.TerraformDocsReadmeSettings{
						Enabled: false,
					},
				},
			},
		},
	}

	docsGenerate := atmosConfig.Docs.Generate["readme"]
	if err := generateDocument(&atmosConfig, targetDir, &docsGenerate, defaultTemplateRenderer{}); err != nil {
		t.Fatalf("generateDocument failed: %v", err)
	}

	outputPath := filepath.Join(targetDir, "TEST_README_DEFAULT.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated README: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "TestProject") {
		t.Errorf("expected output to contain merged name %q, got %q", "TestProject", got)
	}
	if !strings.Contains(got, "A test project") {
		t.Errorf("expected output to contain merged description, got %q", got)
	}
}

// TestGenerateDocument_TemplateFetchFailsFallsBackToDefault verifies that when a configured
// template fails to download, generateDocument still succeeds by falling back to the default
// template rendered through the real renderer, instead of crashing.
func TestGenerateDocument_TemplateFetchFailsFallsBackToDefault(t *testing.T) {
	targetDir := t.TempDir()

	tmpYAML, err := os.CreateTemp("", "test-docs-input-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpYAML.Name())
	yamlContent := "name: TestProject\ndescription: \"A test project\"\n"
	if _, err := tmpYAML.WriteString(yamlContent); err != nil {
		t.Fatal(err)
	}
	tmpYAML.Close()

	atmosConfig := schema.AtmosConfiguration{
		Docs: schema.Docs{
			Generate: map[string]schema.DocsGenerate{
				"readme": {
					BaseDir:  ".",
					Input:    []any{tmpYAML.Name()},
					Template: filepath.Join(targetDir, "does-not-exist.gotmpl"),
					Output:   "TEST_README_FALLBACK.md",
					Terraform: schema.TerraformDocsReadmeSettings{
						Enabled: false,
					},
				},
			},
		},
	}

	docsGenerate := atmosConfig.Docs.Generate["readme"]
	if err := generateDocument(&atmosConfig, targetDir, &docsGenerate, defaultTemplateRenderer{}); err != nil {
		t.Fatalf("generateDocument failed: %v", err)
	}

	outputPath := filepath.Join(targetDir, "TEST_README_FALLBACK.md")
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read generated README: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "TestProject") {
		t.Errorf("expected fallback output to contain merged name %q, got %q", "TestProject", got)
	}
}

// TestRunTerraformDocs_Error tests runTerraformDocs by providing an invalid directory.
func TestRunTerraformDocs_Error(t *testing.T) {
	_, err := runTerraformDocs("nonexistent_directory", &schema.TerraformDocsReadmeSettings{})
	if err == nil {
		t.Errorf("Expected error for invalid terraform module directory, got nil")
	}
}

// writeTerraformDocsFixture writes a minimal module with one input, one output, and one
// resource (which implies one provider) so section-visibility settings are observable.
func writeTerraformDocsFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := `
variable "example_input" {
  type        = string
  description = "An example input"
}

output "example_output" {
  value       = var.example_input
  description = "An example output"
}

resource "null_resource" "example" {}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), defaultFilePermissions); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}
	return dir
}

// TestRunTerraformDocs_SectionsRespected verifies that ShowInputs, ShowOutputs, and
// ShowProviders actually control which sections appear in the rendered output. The
// runTerraformDocs function builds a print.Config with these settings applied, but must pass
// that same config to the formatter -- constructing the formatter with a fresh, uncustomized
// config silently ignores the settings.
func TestRunTerraformDocs_SectionsRespected(t *testing.T) {
	dir := writeTerraformDocsFixture(t)

	out, err := runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
		Enabled:       true,
		Format:        "markdown table",
		ShowInputs:    false,
		ShowOutputs:   true,
		ShowProviders: false,
	})
	if err != nil {
		t.Fatalf("runTerraformDocs failed: %v", err)
	}

	if strings.Contains(out, "example_input") {
		t.Errorf("expected Inputs section to be hidden (ShowInputs=false), got output containing it:\n%s", out)
	}
	if strings.Contains(out, "Providers") {
		t.Errorf("expected Providers section to be hidden (ShowProviders=false), got output containing it:\n%s", out)
	}
	if !strings.Contains(out, "example_output") {
		t.Errorf("expected Outputs section to be shown (ShowOutputs=true), got output missing it:\n%s", out)
	}

	// Flip every flag and confirm the output flips too.
	out, err = runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
		Enabled:       true,
		Format:        "markdown table",
		ShowInputs:    true,
		ShowOutputs:   false,
		ShowProviders: true,
	})
	if err != nil {
		t.Fatalf("runTerraformDocs failed: %v", err)
	}
	if !strings.Contains(out, "example_input") {
		t.Errorf("expected Inputs section to be shown (ShowInputs=true), got output missing it:\n%s", out)
	}
	if !strings.Contains(out, "Providers") {
		t.Errorf("expected Providers section to be shown (ShowProviders=true), got output missing it:\n%s", out)
	}
	if strings.Contains(out, "example_output") {
		t.Errorf("expected Outputs section to be hidden (ShowOutputs=false), got output containing it:\n%s", out)
	}
}

// TestRunTerraformDocs_HideEmptyRespected verifies that HideEmpty suppresses the heading and
// "No inputs." placeholder for a section with no content, instead of always rendering it.
func TestRunTerraformDocs_HideEmptyRespected(t *testing.T) {
	dir := t.TempDir()
	// No variables defined, so the Inputs section is empty.
	content := `
output "example_output" {
  value       = "x"
  description = "An example output"
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(content), defaultFilePermissions); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	out, err := runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
		Enabled:    true,
		Format:     "markdown table",
		ShowInputs: true,
		HideEmpty:  false,
	})
	if err != nil {
		t.Fatalf("runTerraformDocs failed: %v", err)
	}
	if !strings.Contains(out, "No inputs.") {
		t.Errorf("expected empty Inputs section to render the placeholder when HideEmpty=false, got:\n%s", out)
	}

	out, err = runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
		Enabled:    true,
		Format:     "markdown table",
		ShowInputs: true,
		HideEmpty:  true,
	})
	if err != nil {
		t.Fatalf("runTerraformDocs failed: %v", err)
	}
	if strings.Contains(out, "Inputs") || strings.Contains(out, "No inputs.") {
		t.Errorf("expected empty Inputs section to be fully suppressed when HideEmpty=true, got:\n%s", out)
	}
}

// TestRunTerraformDocs_IndentLevelRespected verifies that IndentLevel controls the heading depth
// of rendered sections, confirming it reaches the formatter instead of being silently dropped.
func TestRunTerraformDocs_IndentLevelRespected(t *testing.T) {
	dir := writeTerraformDocsFixture(t)

	out, err := runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
		Enabled:    true,
		Format:     "markdown table",
		ShowInputs: true,
	})
	if err != nil {
		t.Fatalf("runTerraformDocs failed: %v", err)
	}
	if !strings.Contains(out, "## Inputs") {
		t.Errorf("expected default IndentLevel to render a level-2 (##) Inputs heading, got:\n%s", out)
	}

	out, err = runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
		Enabled:     true,
		Format:      "markdown table",
		ShowInputs:  true,
		IndentLevel: 3,
	})
	if err != nil {
		t.Fatalf("runTerraformDocs failed: %v", err)
	}
	if !strings.Contains(out, "### Inputs") {
		t.Errorf("expected IndentLevel=3 to render a level-3 (###) Inputs heading, got:\n%s", out)
	}
	// "## Inputs" is a substring of "### Inputs", so anchor on a preceding newline to check for
	// an actual level-2 heading line rather than matching inside the level-3 one.
	if strings.Contains(out, "\n## Inputs") {
		t.Errorf("expected IndentLevel=3 to replace the level-2 (##) heading, got:\n%s", out)
	}
}

// TestRunTerraformDocs_FormatVariants covers the "markdown", "tfvars hcl", and "tfvars json"
// formatters (in addition to "markdown table" and the default, covered elsewhere), verifying each
// produces format-appropriate content now that they all receive the customized config.
func TestRunTerraformDocs_FormatVariants(t *testing.T) {
	dir := writeTerraformDocsFixture(t)

	t.Run("markdown", func(t *testing.T) {
		out, err := runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
			Enabled:    true,
			Format:     "markdown",
			ShowInputs: true,
		})
		if err != nil {
			t.Fatalf("runTerraformDocs failed: %v", err)
		}
		if !strings.Contains(out, "## Required Inputs") {
			t.Errorf("expected markdown (non-table) format to render a 'Required Inputs' subsection, got:\n%s", out)
		}
	})

	t.Run("tfvars hcl", func(t *testing.T) {
		out, err := runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
			Enabled:    true,
			Format:     "tfvars hcl",
			ShowInputs: true,
		})
		if err != nil {
			t.Fatalf("runTerraformDocs failed: %v", err)
		}
		if !strings.Contains(out, "example_input") {
			t.Errorf("expected tfvars hcl output to assign example_input, got:\n%s", out)
		}
	})

	t.Run("tfvars json", func(t *testing.T) {
		out, err := runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
			Enabled:    true,
			Format:     "tfvars json",
			ShowInputs: true,
		})
		if err != nil {
			t.Fatalf("runTerraformDocs failed: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(out), &parsed); err != nil {
			t.Fatalf("expected valid JSON, got error %v for output:\n%s", err, out)
		}
		if _, ok := parsed["example_input"]; !ok {
			t.Errorf("expected tfvars json output to contain key example_input, got:\n%s", out)
		}
	})

	t.Run("unrecognized format falls back to markdown table", func(t *testing.T) {
		out, err := runTerraformDocs(dir, &schema.TerraformDocsReadmeSettings{
			Enabled:    true,
			Format:     "not-a-real-format",
			ShowInputs: true,
		})
		if err != nil {
			t.Fatalf("runTerraformDocs failed: %v", err)
		}
		if !strings.Contains(out, "| Name | Description | Type | Default | Required |") {
			t.Errorf("expected unrecognized format to fall back to markdown table output, got:\n%s", out)
		}
	})
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
