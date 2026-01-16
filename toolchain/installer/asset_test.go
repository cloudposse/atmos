package installer

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/toolchain/registry"
)

func TestBuildTemplateData_NoReplacements(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Format:    "zip",
		// No VersionPrefix - version should be used as-is (Aqua behavior).
	}

	data := buildTemplateData(tool, "2.15.0")

	// With no VersionPrefix, version is used as-is without adding "v" prefix.
	assert.Equal(t, "2.15.0", data.Version)
	assert.Equal(t, "2.15.0", data.SemVer)
	assert.Equal(t, runtime.GOOS, data.OS)
	assert.Equal(t, runtime.GOARCH, data.Arch)
	assert.Equal(t, "aws", data.RepoOwner)
	assert.Equal(t, "aws-cli", data.RepoName)
	assert.Equal(t, "zip", data.Format)
}

func TestBuildTemplateData_WithReplacements(t *testing.T) {
	// This test verifies that replacements are applied to OS and Arch.
	// The actual OS/Arch replacement depends on the current runtime.
	tool := &registry.Tool{
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Format:    "zip",
		Replacements: map[string]string{
			runtime.GOOS:   "replaced-os",
			runtime.GOARCH: "replaced-arch",
		},
	}

	data := buildTemplateData(tool, "2.15.0")

	assert.Equal(t, "replaced-os", data.OS)
	assert.Equal(t, "replaced-arch", data.Arch)
}

func TestBuildTemplateData_PartialReplacements(t *testing.T) {
	// Only OS is replaced, not arch.
	tool := &registry.Tool{
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Replacements: map[string]string{
			runtime.GOOS: "replaced-os",
		},
	}

	data := buildTemplateData(tool, "2.15.0")

	assert.Equal(t, "replaced-os", data.OS)
	assert.Equal(t, runtime.GOARCH, data.Arch) // Unchanged.
}

func TestBuildTemplateData_UnusedReplacements(t *testing.T) {
	// Replacements that don't match current OS/Arch are ignored.
	tool := &registry.Tool{
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Replacements: map[string]string{
			"nonexistent-os":   "should-not-apply",
			"nonexistent-arch": "should-not-apply",
		},
	}

	data := buildTemplateData(tool, "2.15.0")

	assert.Equal(t, runtime.GOOS, data.OS)
	assert.Equal(t, runtime.GOARCH, data.Arch)
}

func TestBuildTemplateData_AWSCLIReplacements(t *testing.T) {
	// Test the actual replacements used by AWS CLI in Aqua registry.
	tool := &registry.Tool{
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Format:    "zip",
		Replacements: map[string]string{
			"amd64": "x86_64",
			"arm64": "aarch64",
		},
	}

	data := buildTemplateData(tool, "2.15.0")

	// Verify replacements are applied correctly based on current arch.
	switch runtime.GOARCH {
	case "amd64":
		assert.Equal(t, "x86_64", data.Arch)
	case "arm64":
		assert.Equal(t, "aarch64", data.Arch)
	default:
		// For other architectures, no replacement should apply.
		assert.Equal(t, runtime.GOARCH, data.Arch)
	}
}

func TestBuildTemplateData_VersionPrefix(t *testing.T) {
	tests := []struct {
		name            string
		version         string
		versionPrefix   string
		expectedVersion string
		expectedSemVer  string
	}{
		{
			// CRITICAL: This test prevents the regression - empty prefix should NOT add "v".
			// Following Aqua behavior where version_prefix defaults to empty.
			name:            "empty prefix does not add v (Aqua behavior)",
			version:         "2.15.0",
			versionPrefix:   "",
			expectedVersion: "2.15.0",
			expectedSemVer:  "2.15.0",
		},
		{
			name:            "version with v prefix stays unchanged when no prefix configured",
			version:         "v2.15.0",
			versionPrefix:   "",
			expectedVersion: "v2.15.0",
			expectedSemVer:  "v2.15.0",
		},
		{
			name:            "explicit v prefix adds v",
			version:         "2.15.0",
			versionPrefix:   "v",
			expectedVersion: "v2.15.0",
			expectedSemVer:  "2.15.0",
		},
		{
			name:            "version already has explicit v prefix",
			version:         "v2.15.0",
			versionPrefix:   "v",
			expectedVersion: "v2.15.0",
			expectedSemVer:  "2.15.0",
		},
		{
			name:            "custom prefix jq-",
			version:         "1.7.1",
			versionPrefix:   "jq-",
			expectedVersion: "jq-1.7.1",
			expectedSemVer:  "1.7.1",
		},
		{
			name:            "version already has custom prefix",
			version:         "jq-1.7.1",
			versionPrefix:   "jq-",
			expectedVersion: "jq-1.7.1",
			expectedSemVer:  "1.7.1",
		},
		{
			name:            "custom prefix release-",
			version:         "2.15.0",
			versionPrefix:   "release-",
			expectedVersion: "release-2.15.0",
			expectedSemVer:  "2.15.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &registry.Tool{
				RepoOwner:     "test",
				RepoName:      "test",
				VersionPrefix: tt.versionPrefix,
			}

			data := buildTemplateData(tool, tt.version)

			assert.Equal(t, tt.expectedVersion, data.Version)
			assert.Equal(t, tt.expectedSemVer, data.SemVer)
		})
	}
}

func TestExecuteAssetTemplate(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner: "aws",
		RepoName:  "aws-cli",
	}
	data := &assetTemplateData{
		Version:   "v2.15.0",
		SemVer:    "2.15.0",
		OS:        "linux",
		Arch:      "x86_64",
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Format:    "zip",
	}

	tests := []struct {
		name        string
		template    string
		expected    string
		expectError bool
	}{
		{
			name:     "simple URL template",
			template: "https://awscli.amazonaws.com/awscli-exe-{{.OS}}-{{.Arch}}-{{.SemVer}}.zip",
			expected: "https://awscli.amazonaws.com/awscli-exe-linux-x86_64-2.15.0.zip",
		},
		{
			name:     "template with trimV",
			template: "tool-{{trimV .Version}}-{{.OS}}.tar.gz",
			expected: "tool-2.15.0-linux.tar.gz",
		},
		{
			name:     "template with format",
			template: "tool-{{.SemVer}}.{{.Format}}",
			expected: "tool-2.15.0.zip",
		},
		{
			name:     "template with repo info",
			template: "{{.RepoOwner}}/{{.RepoName}}/releases/{{.Version}}/tool.tar.gz",
			expected: "aws/aws-cli/releases/v2.15.0/tool.tar.gz",
		},
		{
			name:        "invalid template syntax",
			template:    "tool-{{.Invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executeAssetTemplate(tt.template, tool, data)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBuildAssetURL_HTTPType(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "http",
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Asset:     "https://awscli.amazonaws.com/awscli-exe-{{.OS}}-{{.Arch}}-{{.SemVer}}.zip",
		Replacements: map[string]string{
			"amd64": "x86_64",
			"arm64": "aarch64",
		},
	}

	url, err := installer.BuildAssetURL(tool, "2.15.0")
	require.NoError(t, err)

	// Verify URL contains expected parts.
	assert.Contains(t, url, "https://awscli.amazonaws.com/awscli-exe-")
	assert.Contains(t, url, "-2.15.0.zip")

	// Verify replacement was applied.
	switch runtime.GOARCH {
	case "amd64":
		assert.Contains(t, url, "x86_64")
	case "arm64":
		assert.Contains(t, url, "aarch64")
	}
}

func TestBuildAssetURL_GitHubReleaseType(t *testing.T) {
	installer := &Installer{}

	// Terraform uses "v" prefix in release tags - must be explicitly configured.
	tool := &registry.Tool{
		Type:          "github_release",
		RepoOwner:     "hashicorp",
		RepoName:      "terraform",
		Asset:         "terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
		VersionPrefix: "v", // Terraform releases use v prefix.
	}

	url, err := installer.BuildAssetURL(tool, "1.5.7")
	require.NoError(t, err)

	assert.Contains(t, url, "https://github.com/hashicorp/terraform/releases/download/v1.5.7/")
	assert.Contains(t, url, "terraform_1.5.7_")
}

func TestBuildAssetURL_MissingAsset(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "http",
		RepoOwner: "test",
		RepoName:  "test",
		// Asset is empty.
	}

	_, err := installer.BuildAssetURL(tool, "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Asset URL template is required")
}

func TestBuildAssetURL_UnsupportedType(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "unsupported",
		RepoOwner: "test",
		RepoName:  "test",
	}

	_, err := installer.BuildAssetURL(tool, "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported tool type")
}

func TestBuildAssetURL_GitHubRelease_MissingOwner(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:     "github_release",
		RepoName: "terraform",
		// RepoOwner is empty.
	}

	_, err := installer.BuildAssetURL(tool, "1.5.7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RepoOwner and RepoName must be set")
}

func TestBuildAssetURL_GitHubRelease_MissingName(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "github_release",
		RepoOwner: "hashicorp",
		// RepoName is empty.
	}

	_, err := installer.BuildAssetURL(tool, "1.5.7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RepoOwner and RepoName must be set")
}

func TestBuildAssetURL_GitHubRelease_DefaultAssetTemplate(t *testing.T) {
	installer := &Installer{}

	// Without VersionPrefix configured, version is used as-is (no automatic "v").
	tool := &registry.Tool{
		Type:      "github_release",
		RepoOwner: "hashicorp",
		RepoName:  "terraform",
		// Asset is empty, should use default template.
		// VersionPrefix is empty, so version is used as-is.
	}

	url, err := installer.BuildAssetURL(tool, "1.5.7")
	require.NoError(t, err)

	// Default template: {{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz
	// Without version_prefix, version is used as-is (no "v" added).
	assert.Contains(t, url, "https://github.com/hashicorp/terraform/releases/download/1.5.7/")
	assert.Contains(t, url, "terraform_1.5.7_")
	assert.Contains(t, url, ".tar.gz")
}

func TestAssetTemplateFuncs(t *testing.T) {
	funcs := assetTemplateFuncs()

	t.Run("trimV removes v prefix", func(t *testing.T) {
		fn := funcs["trimV"].(func(string) string)
		assert.Equal(t, "1.2.3", fn("v1.2.3"))
		assert.Equal(t, "1.2.3", fn("1.2.3"))
	})

	t.Run("trimPrefix removes any prefix", func(t *testing.T) {
		fn := funcs["trimPrefix"].(func(string, string) string)
		assert.Equal(t, "1.2.3", fn("release-", "release-1.2.3"))
		assert.Equal(t, "1.2.3", fn("v", "v1.2.3"))
	})

	t.Run("trimSuffix removes any suffix", func(t *testing.T) {
		fn := funcs["trimSuffix"].(func(string, string) string)
		assert.Equal(t, "file", fn(".txt", "file.txt"))
		assert.Equal(t, "archive", fn(".tar.gz", "archive.tar.gz"))
	})

	t.Run("replace replaces all occurrences", func(t *testing.T) {
		fn := funcs["replace"].(func(string, string, string) string)
		assert.Equal(t, "bar-bar", fn("foo", "bar", "foo-foo"))
	})

	t.Run("eq compares equality", func(t *testing.T) {
		fn := funcs["eq"].(func(string, string) bool)
		assert.True(t, fn("a", "a"))
		assert.False(t, fn("a", "b"))
	})

	t.Run("ne compares inequality", func(t *testing.T) {
		fn := funcs["ne"].(func(string, string) bool)
		assert.True(t, fn("a", "b"))
		assert.False(t, fn("a", "a"))
	})

	t.Run("ternary returns conditional value", func(t *testing.T) {
		fn := funcs["ternary"].(func(bool, string, string) string)
		assert.Equal(t, "yes", fn(true, "yes", "no"))
		assert.Equal(t, "no", fn(false, "yes", "no"))
	})
}

func TestExecuteAssetTemplate_TemplateFunctions(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner: "test",
		RepoName:  "test",
	}
	data := &assetTemplateData{
		Version:   "v2.15.0",
		SemVer:    "2.15.0",
		OS:        "linux",
		Arch:      "amd64",
		RepoOwner: "test",
		RepoName:  "test",
	}

	tests := []struct {
		name     string
		template string
		expected string
	}{
		{
			name:     "trimPrefix function",
			template: "{{trimPrefix \"release-\" .Version}}",
			expected: "v2.15.0",
		},
		{
			name:     "trimSuffix function",
			template: "{{trimSuffix \".0\" .SemVer}}",
			expected: "2.15",
		},
		{
			name:     "replace function",
			template: "{{replace \"amd64\" \"x86_64\" .Arch}}",
			expected: "x86_64",
		},
		{
			name:     "eq function true case",
			template: "{{if eq .OS \"linux\"}}tux{{else}}other{{end}}",
			expected: "tux",
		},
		{
			name:     "ne function true case",
			template: "{{if ne .Arch \"arm64\"}}intel{{else}}arm{{end}}",
			expected: "intel",
		},
		{
			name:     "ternary function",
			template: "{{ternary (eq .OS \"linux\") \"linux.tar.gz\" \"darwin.tar.gz\"}}",
			expected: "linux.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executeAssetTemplate(tt.template, tool, data)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteAssetTemplate_ExecutionError(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner: "test",
		RepoName:  "test",
	}
	data := &assetTemplateData{
		Version: "v1.0.0",
	}

	// Template that references non-existent field causes execution error.
	template := "{{.NonExistent.Field}}"
	_, err := executeAssetTemplate(template, tool, data)
	require.Error(t, err)
}

// =============================================================================
// REGRESSION TESTS: Real-world tool URL generation
// These tests ensure the actual tools that failed continue to work correctly.
// =============================================================================

func TestBuildAssetURL_AWSCLINoVersionPrefix(t *testing.T) {
	// CRITICAL REGRESSION TEST: AWS CLI uses http type without version prefix.
	// URL should NOT include "v" prefix - this was the root cause of the bootstrap failure.
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "http",
		RepoOwner: "aws",
		RepoName:  "aws-cli",
		Asset:     "https://awscli.amazonaws.com/AWSCLIV2-{{.Version}}.pkg",
		// VersionPrefix intentionally empty - this is how Aqua registry defines it.
	}

	url, err := installer.BuildAssetURL(tool, "2.32.31")
	require.NoError(t, err)

	// CRITICAL: URL must NOT contain "v2.32.31" - that causes 404.
	assert.Equal(t, "https://awscli.amazonaws.com/AWSCLIV2-2.32.31.pkg", url)
	assert.NotContains(t, url, "v2.32.31", "AWS CLI URLs must not have v prefix")
}

func TestBuildAssetURL_JQWithExplicitVersionPrefix(t *testing.T) {
	// jq uses explicit version_prefix: jq-.
	installer := &Installer{}

	tool := &registry.Tool{
		Type:          "github_release",
		RepoOwner:     "jqlang",
		RepoName:      "jq",
		Asset:         "jq-{{.OS}}-{{.Arch}}",
		VersionPrefix: "jq-", // Explicitly set in Aqua registry.
	}

	url, err := installer.BuildAssetURL(tool, "1.7.1")
	require.NoError(t, err)

	// Release tag should use the explicit prefix.
	assert.Contains(t, url, "/download/jq-1.7.1/")
}

func TestBuildAssetURL_GumWithExplicitVPrefixAndTrimV(t *testing.T) {
	// gum uses {{trimV .Version}} in asset template with explicit v prefix.
	installer := &Installer{}

	tool := &registry.Tool{
		Type:          "github_release",
		RepoOwner:     "charmbracelet",
		RepoName:      "gum",
		Asset:         "gum_{{trimV .Version}}_{{.OS}}_{{.Arch}}.tar.gz",
		VersionPrefix: "v", // Explicit v prefix for release tag.
		Replacements: map[string]string{
			"darwin": "Darwin",
			"amd64":  "x86_64",
		},
	}

	url, err := installer.BuildAssetURL(tool, "0.17.0")
	require.NoError(t, err)

	// Release tag should have v prefix (v0.17.0).
	assert.Contains(t, url, "/download/v0.17.0/")
	// Asset filename should NOT have v prefix (trimV removes it).
	assert.Contains(t, url, "gum_0.17.0_")
}

func TestBuildAssetURL_HTTPTypePreservesVersionAsIs(t *testing.T) {
	// Generic test: HTTP type tools should use version exactly as provided.
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "http",
		RepoOwner: "example",
		RepoName:  "tool",
		Asset:     "https://example.com/tool-{{.Version}}.tar.gz",
		// No VersionPrefix - version should be used as-is.
	}

	// Test with version without prefix.
	url, err := installer.BuildAssetURL(tool, "1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/tool-1.2.3.tar.gz", url)

	// Test with version that already has v prefix - should preserve it.
	url2, err := installer.BuildAssetURL(tool, "v1.2.3")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/tool-v1.2.3.tar.gz", url2)
}
