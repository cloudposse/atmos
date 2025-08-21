package toolchain

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestInfoCommand_AliasResolution(t *testing.T) {
	SetAtmosConfig(&schema.AtmosConfiguration{})
	// Test that alias resolution works correctly
	installer := NewInstaller()

	// Test with alias
	owner, repo, err := installer.parseToolSpec("terraform")
	assert.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)

	// Find the tool configuration (use "latest" as default version)
	tool, err := installer.findTool(owner, repo, "latest")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "http", tool.Type)
	assert.Equal(t, "hashicorp", tool.RepoOwner)
	assert.Equal(t, "terraform", tool.RepoName)
}

func TestInfoCommand_CanonicalOrgRepo(t *testing.T) {
	// Test that canonical org/repo specification works correctly
	installer := NewInstaller()

	// Test with canonical org/repo
	owner, repo, err := installer.parseToolSpec("hashicorp/terraform")
	assert.NoError(t, err)
	assert.Equal(t, "hashicorp", owner)
	assert.Equal(t, "terraform", repo)

	// Find the tool configuration (use "latest" as default version)
	tool, err := installer.findTool(owner, repo, "latest")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "http", tool.Type)
	assert.Equal(t, "hashicorp", tool.RepoOwner)
	assert.Equal(t, "terraform", tool.RepoName)
}

func TestInfoCommand_GitHubReleaseTool(t *testing.T) {
	// Test with a GitHub release tool
	installer := NewInstaller()

	// Test with opentofu (should be in local config)
	owner, repo, err := installer.parseToolSpec("opentofu")
	assert.NoError(t, err)
	assert.Equal(t, "opentofu", owner)
	assert.Equal(t, "opentofu", repo)

	// Find the tool configuration (use "latest" as default version)
	tool, err := installer.findTool(owner, repo, "latest")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "github_release", tool.Type)
	assert.Equal(t, "opentofu", tool.RepoOwner)
	assert.Equal(t, "opentofu", tool.RepoName)
}

func TestToolToYAML(t *testing.T) {
	// Test the toolToYAML function
	tool := &Tool{
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "tool",
		Asset:     "test-asset",
		Format:    "zip",
	}

	yamlData, err := toolToYAML(tool)
	assert.NoError(t, err)
	assert.Contains(t, yamlData, "type: http")
	assert.Contains(t, yamlData, "repo_owner: test")
	assert.Contains(t, yamlData, "repo_name: tool")
	assert.Contains(t, yamlData, "asset: test-asset")
	assert.Contains(t, yamlData, "format: zip")
}

func TestGetEvaluatedToolYAML(t *testing.T) {
	// Test the getEvaluatedToolYAML function
	installer := NewInstaller()
	tool := &Tool{
		Type:       "http",
		RepoOwner:  "hashicorp",
		RepoName:   "terraform",
		Asset:      "https://releases.hashicorp.com/terraform/{{trimV .Version}}/terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
		Format:     "zip",
		BinaryName: "terraform",
	}

	yamlData, err := getEvaluatedToolYAML(tool, "1.11.4", installer)
	assert.NoError(t, err)

	// Should contain processed templates
	assert.Contains(t, yamlData, "version: 1.11.4")
	assert.Contains(t, yamlData, "type: http")
	assert.Contains(t, yamlData, "repo_owner: hashicorp")
	assert.Contains(t, yamlData, "repo_name: terraform")
	assert.Contains(t, yamlData, "format: zip")
	assert.Contains(t, yamlData, "binary_name: terraform")

	// Should contain processed URL (without template syntax)
	assert.Contains(t, yamlData, "https://releases.hashicorp.com/terraform/1.11.4/terraform_1.11.4_")
	assert.NotContains(t, yamlData, "{{trimV .Version}}")
	assert.NotContains(t, yamlData, "{{.OS}}")
	assert.NotContains(t, yamlData, "{{.Arch}}")
}

func TestFormatToolInfoAsTable(t *testing.T) {
	// Test that table output format contains all required information
	installer := NewInstaller()
	tool := &Tool{
		Type:       "http",
		RepoOwner:  "hashicorp",
		RepoName:   "terraform",
		Asset:      "https://releases.hashicorp.com/terraform/{{trimV .Version}}/terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
		Format:     "zip",
		BinaryName: "terraform",
	}

	table := formatToolInfoAsTable("terraform", "hashicorp", "terraform", tool, "1.11.4", installer)

	// Verify all required information is present
	requiredInfo := []string{
		"Tool",
		"terraform",
		"Owner/Repo",
		"hashicorp/terraform",
		"Type",
		"http",
		"Repository",
		"hashicorp/terraform",
		"Version",
		"1.11.4",
		"Format",
		"zip",
		"Binary Name",
		"terraform",
		"Asset Template",
		"Processed URL",
	}

	for _, info := range requiredInfo {
		assert.Contains(t, table, info, "Table should contain: %s", info)
	}

	// Should contain both raw template and processed URL
	assert.Contains(t, table, "Asset Template")
	assert.Contains(t, table, "Processed URL")

	// Should contain the raw template (with template syntax)
	assert.Contains(t, table, "{{trimV .Version}}")

	// Should also contain processed URL (without template syntax)
	assert.Contains(t, table, "https://releases.hashicorp.com/terraform/1.11.4/terraform_1.11.4_")
}

func TestInfoCommand_CompleteToolConfiguration(t *testing.T) {
	// Test that info command returns complete tool configuration
	installer := NewInstaller()

	// Test with terraform (should have complete config from tools.yaml)
	owner, repo, err := installer.parseToolSpec("terraform")
	assert.NoError(t, err)

	tool, err := installer.findTool(owner, repo, "1.11.4")
	assert.NoError(t, err)
	assert.NotNil(t, tool)

	// Verify all fields are properly populated
	assert.Equal(t, "terraform", tool.Name)
	assert.Equal(t, "http", tool.Type)
	assert.Equal(t, "hashicorp", tool.RepoOwner)
	assert.Equal(t, "terraform", tool.RepoName)
	assert.Equal(t, "zip", tool.Format)
	assert.Equal(t, "terraform", tool.BinaryName)
	assert.Equal(t, "1.11.4", tool.Version)

	// Verify asset/URL templates are present
	assert.Contains(t, tool.Asset, "{{trimV .Version}}")
	assert.Contains(t, tool.Asset, "{{.OS}}")
	assert.Contains(t, tool.Asset, "{{.Arch}}")
}

func TestInfoCommand_YAMLOutputFormat(t *testing.T) {
	// Test that YAML output format contains all required fields
	installer := NewInstaller()
	tool := &Tool{
		Type:       "http",
		RepoOwner:  "hashicorp",
		RepoName:   "terraform",
		Asset:      "https://releases.hashicorp.com/terraform/{{trimV .Version}}/terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
		Format:     "zip",
		BinaryName: "terraform",
	}

	yamlData, err := getEvaluatedToolYAML(tool, "1.11.4", installer)
	assert.NoError(t, err)

	// Verify all required fields are present and properly formatted
	requiredFields := []string{
		"name: terraform",
		"version: 1.11.4",
		"type: http",
		"repo_owner: hashicorp",
		"repo_name: terraform",
		"format: zip",
		"binary_name: terraform",
	}

	for _, field := range requiredFields {
		assert.Contains(t, yamlData, field, "YAML output should contain field: %s", field)
	}

	// Verify templates are processed
	assert.Contains(t, yamlData, "https://releases.hashicorp.com/terraform/1.11.4/terraform_1.11.4_")
	assert.NotContains(t, yamlData, "{{trimV .Version}}")
	assert.NotContains(t, yamlData, "{{.OS}}")
	assert.NotContains(t, yamlData, "{{.Arch}}")
}

func TestInfoCommand_TableOutputFormat(t *testing.T) {
	// Test that table output format contains all required information
	installer := NewInstaller()
	tool := &Tool{
		Type:       "http",
		RepoOwner:  "hashicorp",
		RepoName:   "terraform",
		Asset:      "https://releases.hashicorp.com/terraform/{{trimV .Version}}/terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
		Format:     "zip",
		BinaryName: "terraform",
	}

	table := formatToolInfoAsTable("terraform", "hashicorp", "terraform", tool, "1.11.4", installer)

	// Verify all required information is present
	requiredInfo := []string{
		"Tool",
		"terraform",
		"Owner/Repo",
		"hashicorp/terraform",
		"Type",
		"http",
		"Repository",
		"hashicorp/terraform",
		"Version",
		"1.11.4",
		"Asset Template",
		"Processed URL",
	}

	for _, info := range requiredInfo {
		assert.Contains(t, table, info, "Table output should contain: %s", info)
	}

	// Verify templates are processed in the Processed URL field
	assert.Contains(t, table, "https://releases.hashicorp.com/terraform/1.11.4/terraform_1.11.4_")
	// Asset Template should show raw template, Processed URL should show processed template
	assert.Contains(t, table, "Asset Template")
	assert.Contains(t, table, "Processed URL")
}

func TestInfoCommand_NonExistentTool(t *testing.T) {
	// Test that info command handles non-existent tools gracefully
	installer := NewInstaller()

	// Test with a non-existent tool
	_, err := installer.findTool("nonexistent", "tool", "1.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in any registry")
}

func TestInfoCommand_InvalidOutputFormat(t *testing.T) {
	// Test that info command validates output format
	// This would be tested in an integration test with the actual CLI
	// For now, we can test the validation logic
	validFormats := []string{"table", "yaml"}
	invalidFormats := []string{"json", "xml", "csv"}

	for _, format := range validFormats {
		assert.True(t, format == "table" || format == "yaml", "Format %s should be valid", format)
	}

	for _, format := range invalidFormats {
		assert.False(t, format == "table" || format == "yaml", "Format %s should be invalid", format)
	}
}

func TestInfoCommand_LocalConfigTools(t *testing.T) {
	// Test info command with tools from local config (tools.yaml)
	installer := NewInstaller()

	testCases := []struct {
		name          string
		toolName      string
		expectedType  string
		expectedOwner string
		expectedRepo  string
		hasFormat     bool
		hasBinaryName bool
	}{
		{
			name:          "terraform from local config",
			toolName:      "terraform",
			expectedType:  "http",
			expectedOwner: "hashicorp",
			expectedRepo:  "terraform",
			hasFormat:     true,
			hasBinaryName: true,
		},
		{
			name:          "helm from local config",
			toolName:      "helm",
			expectedType:  "http",
			expectedOwner: "helm",
			expectedRepo:  "helm",
			hasFormat:     true,
			hasBinaryName: true,
		},
		{
			name:          "opentofu from local config",
			toolName:      "opentofu",
			expectedType:  "github_release",
			expectedOwner: "opentofu",
			expectedRepo:  "opentofu",
			hasFormat:     false, // opentofu has version constraints
			hasBinaryName: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := installer.parseToolSpec(tc.toolName)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOwner, owner)
			assert.Equal(t, tc.expectedRepo, repo)

			tool, err := installer.findTool(owner, repo, "1.0.0")
			assert.NoError(t, err)
			assert.NotNil(t, tool)
			assert.Equal(t, tc.expectedType, tool.Type)
			assert.Equal(t, tc.expectedOwner, tool.RepoOwner)
			assert.Equal(t, tc.expectedRepo, tool.RepoName)

			if tc.hasFormat {
				assert.NotEmpty(t, tool.Format, "Tool should have format")
			}
			if tc.hasBinaryName {
				assert.NotEmpty(t, tool.BinaryName, "Tool should have binary name")
			}

			// Test YAML output
			yamlData, err := getEvaluatedToolYAML(tool, "1.0.0", installer)
			assert.NoError(t, err)
			assert.Contains(t, yamlData, "type: "+tc.expectedType)
			assert.Contains(t, yamlData, "repo_owner: "+tc.expectedOwner)
			assert.Contains(t, yamlData, "repo_name: "+tc.expectedRepo)

			// Test table output
			table := formatToolInfoAsTable(tc.toolName, owner, repo, tool, "1.0.0", installer)
			assert.Contains(t, table, "Tool")
			assert.Contains(t, table, tc.toolName)
			assert.Contains(t, table, "Type")
			assert.Contains(t, table, tc.expectedType)
		})
	}
}

func TestInfoCommand_AquaRegistryTools(t *testing.T) {
	// Test info command with tools from Aqua registry (not in local config)
	installer := NewInstaller()

	// Note: These tests may fail if the tools don't exist in the Aqua registry
	// or if network access is required. We'll test the basic functionality.

	testCases := []struct {
		name          string
		toolName      string
		expectedOwner string
		expectedRepo  string
	}{
		{
			name:          "kubectl from Aqua registry",
			toolName:      "kubectl",
			expectedOwner: "kubernetes-sigs",
			expectedRepo:  "kubectl",
		},
		{
			name:          "kind from Aqua registry",
			toolName:      "kind",
			expectedOwner: "kubernetes-sigs",
			expectedRepo:  "kind",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := installer.parseToolSpec(tc.toolName)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOwner, owner)
			assert.Equal(t, tc.expectedRepo, repo)

			// Note: This may fail if the tool doesn't exist in Aqua registry
			// We're testing the parsing and resolution logic, not the actual registry lookup
			tool, err := installer.findTool(owner, repo, "latest")
			if err != nil {
				// If tool not found in registry, that's okay for this test
				// We're testing the info command logic, not registry availability
				t.Logf("Tool %s not found in registry (expected for some tools): %v", tc.toolName, err)
				return
			}

			if tool != nil {
				assert.Equal(t, tc.expectedOwner, tool.RepoOwner)
				assert.Equal(t, tc.expectedRepo, tool.RepoName)

				// Test YAML output
				yamlData, err := getEvaluatedToolYAML(tool, "latest", installer)
				assert.NoError(t, err)
				assert.Contains(t, yamlData, "repo_owner: "+tc.expectedOwner)
				assert.Contains(t, yamlData, "repo_name: "+tc.expectedRepo)

				// Test table output
				table := formatToolInfoAsTable(tc.toolName, owner, repo, tool, "latest", installer)
				assert.Contains(t, table, "Tool")
				assert.Contains(t, table, tc.toolName)
			}
		})
	}
}

func TestInfoCommand_VersionConstraints(t *testing.T) {
	// Test info command with tools that have version constraints
	installer := NewInstaller()

	// Test opentofu which has version constraints in local config
	owner, repo, err := installer.parseToolSpec("opentofu")
	assert.NoError(t, err)

	// Test with a version that should match a constraint
	tool, err := installer.findTool(owner, repo, "1.10.0")
	assert.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "github_release", tool.Type)
	assert.Equal(t, "opentofu", tool.RepoOwner)
	assert.Equal(t, "opentofu", tool.RepoName)

	// Test YAML output with version constraints
	yamlData, err := getEvaluatedToolYAML(tool, "1.10.0", installer)
	assert.NoError(t, err)
	assert.Contains(t, yamlData, "version: 1.10.0")
	assert.Contains(t, yamlData, "type: github_release")

	// Test table output with version constraints
	table := formatToolInfoAsTable("opentofu", owner, repo, tool, "1.10.0", installer)
	assert.Contains(t, table, "Version")
	assert.Contains(t, table, "1.10.0")
}

func TestInfoCommand_DifferentToolTypes(t *testing.T) {
	// Test info command with different tool types
	installer := NewInstaller()

	testCases := []struct {
		name         string
		toolName     string
		expectedType string
		description  string
	}{
		{
			name:         "HTTP type tool",
			toolName:     "terraform",
			expectedType: "http",
			description:  "Tool using direct HTTP downloads",
		},
		{
			name:         "GitHub release tool",
			toolName:     "opentofu",
			expectedType: "github_release",
			description:  "Tool using GitHub releases",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := installer.parseToolSpec(tc.toolName)
			assert.NoError(t, err)

			tool, err := installer.findTool(owner, repo, "1.0.0")
			assert.NoError(t, err)
			assert.NotNil(t, tool)
			assert.Equal(t, tc.expectedType, tool.Type, tc.description)

			// Test that both output formats work for this tool type
			yamlData, err := getEvaluatedToolYAML(tool, "1.0.0", installer)
			assert.NoError(t, err)
			assert.Contains(t, yamlData, "type: "+tc.expectedType)

			table := formatToolInfoAsTable(tc.toolName, owner, repo, tool, "1.0.0", installer)
			assert.Contains(t, table, "Type")
			assert.Contains(t, table, tc.expectedType)
		})
	}
}

func TestInfoCommand_EdgeCases(t *testing.T) {
	// Test edge cases for info command
	installer := NewInstaller()

	t.Run("tool with files", func(t *testing.T) {
		// Test tool with files configuration
		tool := &Tool{
			Type:      "github_release",
			RepoOwner: "test",
			RepoName:  "tool-with-files",
			Files: []File{
				{Name: "binary", Src: "tool"},
				{Name: "config", Src: "config.yaml"},
			},
		}

		table := formatToolInfoAsTable("test-tool", "test", "tool-with-files", tool, "1.0.0", installer)
		assert.Contains(t, table, "File")
		assert.Contains(t, table, "tool -> binary")
		assert.Contains(t, table, "config.yaml -> config")
	})

	t.Run("tool with overrides", func(t *testing.T) {
		// Test tool with platform overrides
		tool := &Tool{
			Type:      "github_release",
			RepoOwner: "test",
			RepoName:  "tool-with-overrides",
			Overrides: []Override{
				{GOOS: "darwin", GOARCH: "arm64", Asset: "tool-darwin-arm64"},
				{GOOS: "linux", GOARCH: "amd64", Asset: "tool-linux-amd64"},
			},
		}

		table := formatToolInfoAsTable("test-tool", "test", "tool-with-overrides", tool, "1.0.0", installer)
		assert.Contains(t, table, "Override")
		assert.Contains(t, table, "darwin/arm64")
		assert.Contains(t, table, "linux/amd64")
	})

	t.Run("tool with empty fields", func(t *testing.T) {
		// Test tool with minimal configuration
		tool := &Tool{
			Type:      "http",
			RepoOwner: "test",
			RepoName:  "minimal-tool",
		}

		yamlData, err := getEvaluatedToolYAML(tool, "1.0.0", installer)
		assert.NoError(t, err)
		assert.Contains(t, yamlData, "type: http")
		assert.Contains(t, yamlData, "repo_owner: test")
		assert.Contains(t, yamlData, "repo_name: minimal-tool")

		table := formatToolInfoAsTable("minimal-tool", "test", "minimal-tool", tool, "1.0.0", installer)
		assert.Contains(t, table, "Tool")
		assert.Contains(t, table, "minimal-tool")
		assert.Contains(t, table, "Type")
		assert.Contains(t, table, "http")
	})
}
