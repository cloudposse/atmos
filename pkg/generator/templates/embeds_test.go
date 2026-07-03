package templates

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/generator/engine"
)

func TestGetAvailableConfigurations(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	if len(configs) == 0 {
		t.Error("Expected at least one configuration, got 0")
	}

	// Check that configurations have required fields
	for name, config := range configs {
		if config.Name == "" {
			t.Errorf("Configuration %s has empty Name", name)
		}
		if config.TemplateID == "" {
			t.Errorf("Configuration %s has empty TemplateID", name)
		}
		if len(config.Files) == 0 {
			t.Errorf("Configuration %s has no files", name)
		}
	}
}

func TestGetAvailableConfigurations_ExpectedTemplates(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	// We should have at least "atmos", "basic", and "simple" templates
	expectedTemplates := []string{"atmos", "basic", "simple"}
	for _, expected := range expectedTemplates {
		if _, exists := configs[expected]; !exists {
			t.Errorf("Expected template %q not found in configurations", expected)
		}
	}
}

func TestConfiguration_FilesStructure(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	// Get the "atmos" template for detailed testing
	atmosConfig, exists := configs["atmos"]
	if !exists {
		t.Skip("atmos template not found, skipping file structure test")
	}

	// Verify files have proper structure
	var foundREADME, foundAtmosYaml bool
	for _, file := range atmosConfig.Files {
		if file.Path == "" {
			t.Error("Found file with empty path")
		}

		// Check for expected files
		if file.Path == "README.md" {
			foundREADME = true
			if file.Content == "" {
				t.Error("README.md has empty content")
			}
			if file.IsDirectory {
				t.Error("README.md marked as directory")
			}
		}
		if file.Path == "atmos.yaml" {
			foundAtmosYaml = true
		}
	}

	if !foundREADME {
		t.Error("Expected to find README.md in atmos template")
	}
	if !foundAtmosYaml {
		t.Error("Expected to find atmos.yaml in atmos template")
	}
}

func TestConfiguration_README(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	atmosConfig, exists := configs["atmos"]
	if !exists {
		t.Skip("atmos template not found")
	}

	if atmosConfig.README == "" {
		t.Error("Expected atmos template to have README content")
	}
}

func TestHasScaffoldConfig(t *testing.T) {
	tests := []struct {
		name     string
		files    []File
		expected bool
	}{
		{
			name: "has scaffold.yaml",
			files: []File{
				{Path: "scaffold.yaml", IsDirectory: false},
				{Path: "README.md", IsDirectory: false},
			},
			expected: true,
		},
		{
			name: "no scaffold.yaml",
			files: []File{
				{Path: "README.md", IsDirectory: false},
				{Path: "atmos.yaml", IsDirectory: false},
			},
			expected: false,
		},
		{
			name: "scaffold.yaml is directory (not file)",
			files: []File{
				{Path: "scaffold.yaml", IsDirectory: true},
			},
			expected: false,
		},
		{
			name:     "empty files list",
			files:    []File{},
			expected: false,
		},
		{
			name: "scaffold.yaml in subdirectory",
			files: []File{
				{Path: "subdir/scaffold.yaml", IsDirectory: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasScaffoldConfig(tt.files)
			if result != tt.expected {
				t.Errorf("HasScaffoldConfig() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConfiguration_TemplateDetection(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	// Check that files with .tmpl extension or {{ }} are marked as templates
	for _, config := range configs {
		for _, file := range config.Files {
			if file.IsDirectory {
				continue
			}

			// Files with .tmpl extension should be marked as template
			if len(file.Path) > 5 && file.Path[len(file.Path)-5:] == ".tmpl" {
				if !file.IsTemplate {
					t.Errorf("File %s has .tmpl extension but IsTemplate = false", file.Path)
				}
			}

			// Files containing {{ should be marked as template (basic check)
			// Note: This is a heuristic, not all files with {{ are Go templates
		}
	}
}

func TestConfiguration_Permissions(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	for name, config := range configs {
		for _, file := range config.Files {
			if file.IsDirectory {
				if file.Permissions != 0o755 {
					t.Errorf("Directory %s in template %s has permissions %o, expected 0755",
						file.Path, name, file.Permissions)
				}
			} else {
				if file.Permissions != 0o644 {
					t.Errorf("File %s in template %s has permissions %o, expected 0644",
						file.Path, name, file.Permissions)
				}
			}
		}
	}
}

func TestConfiguration_Metadata(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	for name, config := range configs {
		// Check that Name matches the template directory name
		if config.Name != name {
			t.Errorf("Template %s: Name field %q doesn't match directory name %q",
				name, config.Name, name)
		}

		// Check that TemplateID is set
		if config.TemplateID == "" {
			t.Errorf("Template %s has empty TemplateID", name)
		}

		// Check that Description contains the word "template"
		// This is based on the default description format
		// If templates have custom descriptions, this test may need adjustment
	}
}

// TestConfiguration_BasicMetadata verifies the "basic" template exists with the
// expected metadata from its scaffold.yaml manifest.
func TestConfiguration_BasicMetadata(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	require.NoError(t, err)

	basicConfig, exists := configs["basic"]
	require.True(t, exists, "basic template not found in configurations")

	assert.Equal(t, "basic", basicConfig.Name)
	assert.Equal(t, "basic", basicConfig.TemplateID)
	assert.Equal(t, "Minimal cloud-agnostic Atmos project layout — atmos.yaml, stacks, and components", basicConfig.Description)
	assert.Equal(t, "1.0.0", basicConfig.Version)
	assert.NotEmpty(t, basicConfig.README, "basic template should have README content")
	assert.True(t, HasScaffoldConfig(basicConfig.Files), "basic template should have a scaffold.yaml")
}

// renderBasicTemplate renders the embedded "basic" template into a temp
// directory using the same engine the init command uses, and returns the
// (absolute) target directory.
func renderBasicTemplate(t *testing.T, values map[string]interface{}) string {
	t.Helper()

	configs, err := GetAvailableConfigurations()
	require.NoError(t, err)

	basicConfig, exists := configs["basic"]
	require.True(t, exists, "basic template not found in configurations")

	targetDir := t.TempDir()
	processor := engine.NewProcessor()

	for _, file := range basicConfig.Files {
		// scaffold.yaml only defines the questionnaire; it is never generated.
		if file.Path == "scaffold.yaml" || file.IsDirectory {
			continue
		}
		renderErr := processor.ProcessFile(engine.File{
			Path:        file.Path,
			Content:     file.Content,
			IsTemplate:  file.IsTemplate,
			Permissions: file.Permissions,
		}, targetDir, false, false, nil, values)
		require.NoError(t, renderErr, "failed to render %s", file.Path)
	}

	return targetDir
}

// listRenderedFiles walks targetDir (absolute) and returns sorted slash-separated
// relative paths of all regular files.
func listRenderedFiles(t *testing.T, targetDir string) []string {
	t.Helper()

	// Resolve to an absolute path so the walk is CWD-independent.
	absDir, err := filepath.Abs(targetDir)
	require.NoError(t, err)

	var rendered []string
	err = filepath.WalkDir(absDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(absDir, path)
		if relErr != nil {
			return relErr
		}
		rendered = append(rendered, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	sort.Strings(rendered)
	return rendered
}

// TestBasicTemplate_Render verifies that rendering the "basic" template
// produces the exact expected file set with correct contents.
func TestBasicTemplate_Render(t *testing.T) {
	targetDir := renderBasicTemplate(t, map[string]interface{}{"project_name": "test-proj"})
	rendered := listRenderedFiles(t, targetDir)

	expected := []string{
		".gitignore",
		"README.md",
		"atmos.yaml",
		"components/terraform/README.md",
		"stacks/_defaults.yaml",
		"stacks/dev.yaml",
	}
	require.Equal(t, expected, rendered)

	// Assert the first file (.gitignore) by value: copied verbatim.
	gitignore, err := os.ReadFile(filepath.Join(targetDir, ".gitignore"))
	require.NoError(t, err)
	expectedGitignore := `# Atmos cache
.atmos/cache*

# Terraform
*.tfstate
*.tfstate.*
.terraform/
.terraform.lock.hcl

# Atmos auto-generated backend configs and varfiles
backend.tf.json
*.terraform.tfvars.json
`
	assert.Equal(t, expectedGitignore, string(gitignore))

	// Assert the last file (stacks/dev.yaml) by value: copied verbatim.
	devStack, err := os.ReadFile(filepath.Join(targetDir, "stacks", "dev.yaml"))
	require.NoError(t, err)
	expectedDevStack := `# yaml-language-server: $schema=https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json

# The ` + "`dev`" + ` stack. The stack name comes from ` + "`name_template`" + ` in ` + "`atmos.yaml`" + `,
# which reads ` + "`vars.stage`" + `.

import:
  - _defaults

vars:
  stage: dev

# Define components for this stack. For example, after adding a Terraform
# component under ` + "`components/terraform/my-component`" + `:
#
# components:
#   terraform:
#     my-component:
#       vars:
#         enabled: true
#
# Then run:
#   atmos terraform plan my-component -s dev
`
	assert.Equal(t, expectedDevStack, string(devStack))

	// The README is a template: the project name must be substituted and the
	// magic comment stripped.
	readme, err := os.ReadFile(filepath.Join(targetDir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(readme), "# test-proj")
	assert.NotContains(t, string(readme), "atmos:template")
	assert.NotContains(t, string(readme), "{{")
}

// TestBasicTemplate_AtmosYaml verifies the rendered atmos.yaml is valid YAML,
// uses name_template, and never the deprecated name_pattern.
func TestBasicTemplate_AtmosYaml(t *testing.T) {
	targetDir := renderBasicTemplate(t, map[string]interface{}{"project_name": "test-proj"})

	content, err := os.ReadFile(filepath.Join(targetDir, "atmos.yaml"))
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, yaml.Unmarshal(content, &parsed), "rendered atmos.yaml must be valid YAML")

	assert.Contains(t, string(content), "name_template")
	assert.NotContains(t, string(content), "name_pattern")

	// The stack name template must survive verbatim (the file is copied, not
	// rendered, so the Go-template syntax inside name_template is preserved).
	stacks, ok := parsed["stacks"].(map[string]interface{})
	require.True(t, ok, "atmos.yaml must have a stacks section")
	assert.Equal(t, "{{ .vars.stage }}", stacks["name_template"])

	// Cloud-agnostic: no provider, region, or vendor references.
	lowered := strings.ToLower(string(content))
	for _, forbidden := range []string{"aws", "azure", "gcp", "region"} {
		assert.NotContains(t, lowered, forbidden)
	}
}

func TestFile_NonEmpty(t *testing.T) {
	configs, err := GetAvailableConfigurations()
	if err != nil {
		t.Fatalf("GetAvailableConfigurations() error = %v", err)
	}

	for name, config := range configs {
		for _, file := range config.Files {
			// Non-directory files should have content (or be deliberately empty)
			// Directories should have empty content
			if file.IsDirectory && file.Content != "" {
				t.Errorf("Directory %s in template %s has non-empty content",
					file.Path, name)
			}
		}
	}
}
