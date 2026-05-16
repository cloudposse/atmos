package installer

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
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

func TestBuildTemplateData_GOOSAndGOARCH(t *testing.T) {
	// GOOS/GOARCH should always be raw runtime values, even when replacements are applied.
	tool := &registry.Tool{
		RepoOwner: "test",
		RepoName:  "tool",
		Replacements: map[string]string{
			runtime.GOOS:   "replaced-os",
			runtime.GOARCH: "replaced-arch",
		},
	}

	data := buildTemplateData(tool, "1.0.0")

	// OS/Arch should have replacements applied.
	assert.Equal(t, "replaced-os", data.OS)
	assert.Equal(t, "replaced-arch", data.Arch)
	// GOOS/GOARCH should be raw runtime values.
	assert.Equal(t, runtime.GOOS, data.GOOS)
	assert.Equal(t, runtime.GOARCH, data.GOARCH)
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

	// Aqua-specific overrides: test via direct type assertion.
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

	// Sprig-provided functions: test via template execution (Sprig uses interface{} signatures).
	t.Run("eq compares equality via template", func(t *testing.T) {
		tmpl := template.Must(template.New("test").Funcs(funcs).Parse(`{{if eq .A .B}}true{{else}}false{{end}}`))
		var buf strings.Builder
		require.NoError(t, tmpl.Execute(&buf, map[string]string{"A": "a", "B": "a"}))
		assert.Equal(t, "true", buf.String())
	})

	t.Run("ne compares inequality via template", func(t *testing.T) {
		tmpl := template.Must(template.New("test").Funcs(funcs).Parse(`{{if ne .A .B}}true{{else}}false{{end}}`))
		var buf strings.Builder
		require.NoError(t, tmpl.Execute(&buf, map[string]string{"A": "a", "B": "b"}))
		assert.Equal(t, "true", buf.String())
	})

	t.Run("ternary returns conditional value via template", func(t *testing.T) {
		tmpl := template.Must(template.New("test").Funcs(funcs).Parse(`{{ternary "yes" "no" true}}`))
		var buf strings.Builder
		require.NoError(t, tmpl.Execute(&buf, nil))
		assert.Equal(t, "yes", buf.String())
	})

	// Sprig functions: verify key Sprig functions are available.
	t.Run("sprig title function", func(t *testing.T) {
		tmpl := template.Must(template.New("test").Funcs(funcs).Parse(`{{title .OS}}`))
		var buf strings.Builder
		require.NoError(t, tmpl.Execute(&buf, map[string]string{"OS": "darwin"}))
		assert.Equal(t, "Darwin", buf.String())
	})

	t.Run("sprig upper function", func(t *testing.T) {
		tmpl := template.Must(template.New("test").Funcs(funcs).Parse(`{{upper .OS}}`))
		var buf strings.Builder
		require.NoError(t, tmpl.Execute(&buf, map[string]string{"OS": "linux"}))
		assert.Equal(t, "LINUX", buf.String())
	})

	t.Run("sprig lower function", func(t *testing.T) {
		tmpl := template.Must(template.New("test").Funcs(funcs).Parse(`{{lower .Name}}`))
		var buf strings.Builder
		require.NoError(t, tmpl.Execute(&buf, map[string]string{"Name": "MyTool"}))
		assert.Equal(t, "mytool", buf.String())
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
			template: "{{ternary \"linux.tar.gz\" \"darwin.tar.gz\" (eq .OS \"linux\")}}",
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

// TestHasArchiveExtension tests the hasArchiveExtension helper function.
func TestHasArchiveExtension(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"tar.gz extension", "tool-1.0.0.tar.gz", true},
		{"tgz extension", "tool-1.0.0.tgz", true},
		{"zip extension", "tool-1.0.0.zip", true},
		{"gz extension", "tool-1.0.0.gz", true},
		{"tar extension", "tool-1.0.0.tar", true},
		{"pkg extension", "tool-1.0.0.pkg", true},
		{"tar.xz extension", "tool-1.0.0.tar.xz", true},
		{"txz extension", "tool-1.0.0.txz", true},
		{"tar.bz2 extension", "tool-1.0.0.tar.bz2", true},
		{"tbz extension", "tool-1.0.0.tbz", true},
		{"tbz2 extension", "tool-1.0.0.tbz2", true},
		{"bz2 extension", "tool-1.0.0.bz2", true},
		{"xz extension", "tool-1.0.0.xz", true},
		{"7z extension", "tool-1.0.0.7z", true},
		{"uppercase TAR.GZ", "tool-1.0.0.TAR.GZ", true},
		{"mixed case Zip", "tool-1.0.0.Zip", true},
		{"uppercase TAR.XZ", "tool-1.0.0.TAR.XZ", true},
		{"no extension", "tool-windows-amd64", false},
		{"exe extension", "tool.exe", false},
		{"partial match tar", "mytar-tool", false},
		{"partial match zip", "unzip-tool", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasArchiveExtension(tt.input)
			assert.Equal(t, tt.expected, result, "hasArchiveExtension(%q)", tt.input)
		})
	}
}

// TestBuildAssetURL_WindowsExeExtensionForRawBinary tests that on Windows,
// raw binary assets get .exe appended to the download URL.
func TestBuildAssetURL_WindowsExeExtensionForRawBinary(t *testing.T) {
	installer := &Installer{}

	// Tool with raw binary asset (no archive extension) - like jq.
	tool := &registry.Tool{
		Type:          "github_release",
		RepoOwner:     "jqlang",
		RepoName:      "jq",
		Asset:         "jq-{{.OS}}-{{.Arch}}",
		VersionPrefix: "jq-",
	}

	url, err := installer.BuildAssetURL(tool, "1.7.1")
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		// On Windows, should have .exe extension.
		assert.Contains(t, url, ".exe", "Windows URL should contain .exe")
		assert.True(t, strings.HasSuffix(url, ".exe"), "Windows URL should end with .exe")
	} else {
		// On non-Windows, should NOT have .exe extension.
		assert.NotContains(t, url, ".exe", "Non-Windows URL should not contain .exe")
	}
}

// TestBuildAssetURL_NoExeForArchives tests that archive assets don't get .exe appended.
func TestBuildAssetURL_NoExeForArchives(t *testing.T) {
	installer := &Installer{}

	tests := []struct {
		name  string
		asset string
	}{
		{"tar.gz archive", "tool_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"},
		{"zip archive", "tool_{{.Version}}_{{.OS}}_{{.Arch}}.zip"},
		{"tgz archive", "tool-{{.Version}}.tgz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &registry.Tool{
				Type:      "github_release",
				RepoOwner: "example",
				RepoName:  "tool",
				Asset:     tt.asset,
			}

			url, err := installer.BuildAssetURL(tool, "1.0.0")
			require.NoError(t, err)

			// Archives should never get double .exe extension, even on Windows.
			assert.NotContains(t, url, ".exe", "Archive URL should not contain .exe")
		})
	}
}

// TestBuildAssetURL_HTTPTypeWindowsExeExtension tests that HTTP type tools also get
// .exe appended on Windows for raw binary URLs (like kubectl from dl.k8s.io).
func TestBuildAssetURL_HTTPTypeWindowsExeExtension(t *testing.T) {
	installer := &Installer{}

	// HTTP type tool with raw binary URL (no extension) - like kubectl.
	tool := &registry.Tool{
		Type:  "http",
		Asset: "https://dl.k8s.io/v{{.Version}}/bin/{{.OS}}/{{.Arch}}/kubectl",
	}

	url, err := installer.BuildAssetURL(tool, "1.31.4")
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		// On Windows, should have .exe extension.
		assert.True(t, strings.HasSuffix(url, ".exe"), "Windows HTTP URL should end with .exe: %s", url)
		expectedURL := fmt.Sprintf("https://dl.k8s.io/v1.31.4/bin/windows/%s/kubectl.exe", runtime.GOARCH)
		assert.Equal(t, expectedURL, url)
	} else {
		// On non-Windows, should NOT have .exe extension.
		assert.False(t, strings.HasSuffix(url, ".exe"), "Non-Windows HTTP URL should not end with .exe: %s", url)
	}
}

// TestBuildAssetURL_HTTPTypeNoExeForArchives tests that HTTP type archive URLs
// don't get .exe appended.
func TestBuildAssetURL_HTTPTypeNoExeForArchives(t *testing.T) {
	installer := &Installer{}

	// HTTP type tool with archive URL.
	tool := &registry.Tool{
		Type:  "http",
		Asset: "https://example.com/releases/v{{.Version}}/tool-{{.OS}}-{{.Arch}}.tar.gz",
	}

	url, err := installer.BuildAssetURL(tool, "1.0.0")
	require.NoError(t, err)

	// Archives should never get .exe appended, even on Windows.
	assert.False(t, strings.HasSuffix(url, ".exe"), "Archive HTTP URL should not have .exe: %s", url)
	assert.True(t, strings.HasSuffix(url, ".tar.gz"), "Archive URL should keep .tar.gz extension: %s", url)
}

// TestBuildAssetURL_NoExeForOtherExtensions tests that assets with non-archive extensions
// (like .msi, .dmg, .deb) don't get .exe appended.
func TestBuildAssetURL_NoExeForOtherExtensions(t *testing.T) {
	installer := &Installer{}

	tests := []struct {
		name  string
		asset string
	}{
		{"msi installer", "tool_{{.Version}}_{{.OS}}_{{.Arch}}.msi"},
		{"dmg image", "tool_{{.Version}}_{{.OS}}_{{.Arch}}.dmg"},
		{"deb package", "tool_{{.Version}}_{{.OS}}_{{.Arch}}.deb"},
		{"rpm package", "tool_{{.Version}}_{{.OS}}_{{.Arch}}.rpm"},
		{"appimage", "tool_{{.Version}}_{{.OS}}_{{.Arch}}.AppImage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &registry.Tool{
				Type:      "github_release",
				RepoOwner: "example",
				RepoName:  "tool",
				Asset:     tt.asset,
			}

			url, err := installer.BuildAssetURL(tool, "1.0.0")
			require.NoError(t, err)

			// Non-archive extensions should not get .exe appended (avoids .msi.exe, etc.).
			assert.False(t, strings.HasSuffix(url, ".exe"),
				"URL with %s extension should not have .exe appended: %s", tt.name, url)
		})
	}
}

// TestExecuteAssetTemplate_TwoPassRendering tests the two-pass rendering for Asset/AssetWithoutExt.
func TestExecuteAssetTemplate_TwoPassRendering(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner: "charmbracelet",
		RepoName:  "gum",
	}

	t.Run("template referencing .AssetWithoutExt triggers second pass", func(t *testing.T) {
		data := &assetTemplateData{
			Version:   "v0.15.2",
			SemVer:    "0.15.2",
			OS:        "Linux",
			Arch:      "x86_64",
			RepoOwner: "charmbracelet",
			RepoName:  "gum",
			Format:    "tar.gz",
		}
		// Template that references .AssetWithoutExt (contains ".Asset" substring → triggers two-pass).
		// Pass 1: AssetWithoutExt is empty, renders to "gum_0.15.2_Linux_x86_64.tar.gz/"
		// Then: Asset = "gum_0.15.2_Linux_x86_64.tar.gz/", AssetWithoutExt = "gum_0.15.2_Linux_x86_64/"
		// Pass 2: re-renders with AssetWithoutExt populated.
		tmpl := "{{.RepoName}}_{{.SemVer}}_{{.OS}}_{{.Arch}}.{{.Format}}/{{.AssetWithoutExt}}"
		result, err := executeAssetTemplate(tmpl, tool, data)
		require.NoError(t, err)
		// Verify AssetWithoutExt was populated (two-pass was triggered).
		assert.NotEmpty(t, data.AssetWithoutExt)
		assert.NotEmpty(t, data.Asset)
		assert.Contains(t, result, "gum_0.15.2_Linux_x86_64")
	})

	t.Run("template referencing .Asset directly triggers second pass", func(t *testing.T) {
		data := &assetTemplateData{
			Version:   "v1.0.0",
			SemVer:    "1.0.0",
			OS:        "linux",
			Arch:      "amd64",
			RepoOwner: "test",
			RepoName:  "tool",
			Format:    "tar.gz",
		}
		// Template with {{.Asset}} reference → triggers two-pass.
		// Pass 1: Asset is empty → result = "tool_1.0.0_linux_amd64.tar.gz/"
		// data.Asset = "tool_1.0.0_linux_amd64.tar.gz/", AssetWithoutExt computed.
		// Pass 2: re-renders with Asset populated.
		tmpl := "{{.RepoName}}_{{.SemVer}}_{{.OS}}_{{.Arch}}.{{.Format}}/{{.Asset}}"
		result, err := executeAssetTemplate(tmpl, tool, data)
		require.NoError(t, err)
		assert.NotEmpty(t, data.Asset)
		assert.Contains(t, result, "tool_1.0.0_linux_amd64")
	})

	t.Run("template without .Asset renders in single pass", func(t *testing.T) {
		data := &assetTemplateData{
			Version:   "v1.0.0",
			SemVer:    "1.0.0",
			OS:        "linux",
			Arch:      "amd64",
			RepoOwner: "test",
			RepoName:  "test",
			Format:    "tar.gz",
		}
		// No .Asset reference → single pass only.
		tmpl := "{{.RepoName}}_{{.SemVer}}.{{.Format}}"
		result, err := executeAssetTemplate(tmpl, tool, data)
		require.NoError(t, err)
		assert.Equal(t, "test_1.0.0.tar.gz", result)
		// AssetWithoutExt should NOT have been set.
		assert.Empty(t, data.AssetWithoutExt)
	})
}

func TestStripFileExtension(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tar.gz compound", "tool_linux_amd64.tar.gz", "tool_linux_amd64"},
		{"tar.xz compound", "tool_linux_amd64.tar.xz", "tool_linux_amd64"},
		{"tar.bz2 compound", "tool_linux_amd64.tar.bz2", "tool_linux_amd64"},
		{"zip extension", "tool_windows_amd64.zip", "tool_windows_amd64"},
		{"no extension", "tool_linux_amd64", "tool_linux_amd64"},
		{"exe extension", "tool.exe", "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, stripFileExtension(tt.input))
		})
	}
}

func TestBuildTemplateData_FormatOverrides(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner: "test",
		RepoName:  "tool",
		Format:    "tar.gz",
		FormatOverrides: []registry.FormatOverride{
			{GOOS: runtime.GOOS, Format: "zip"},
		},
	}

	data := buildTemplateData(tool, "1.0.0")
	// The format override for the current OS should be applied.
	assert.Equal(t, "zip", data.Format)
}

// TestBuildTemplateData_Rosetta2 verifies that Rosetta2 fallback is applied in template data.
// On darwin/arm64, Arch should fall back to "amd64" while GOARCH preserves the raw value.
func TestBuildTemplateData_Rosetta2(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner: "test",
		RepoName:  "tool",
		Format:    "tar.gz",
		Rosetta2:  true,
	}

	data := buildTemplateData(tool, "1.0.0")

	// GOARCH must always preserve the raw runtime value regardless of Rosetta2.
	assert.Equal(t, runtime.GOARCH, data.GOARCH, "GOARCH always preserves raw runtime value")

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		assert.Equal(t, "amd64", data.Arch, "Rosetta2 should fall back Arch to amd64 on darwin/arm64")
	} else {
		assert.Equal(t, runtime.GOARCH, data.Arch, "Rosetta2 should not affect Arch on non-darwin-arm64")
	}
}

// TestBuildTemplateData_WindowsArmEmulation verifies that Windows ARM emulation fallback
// is applied in template data. On windows/arm64, Arch should fall back to "amd64".
func TestBuildTemplateData_WindowsArmEmulation(t *testing.T) {
	tool := &registry.Tool{
		RepoOwner:           "test",
		RepoName:            "tool",
		Format:              "tar.gz",
		WindowsArmEmulation: true,
	}

	data := buildTemplateData(tool, "1.0.0")

	// GOARCH must always preserve the raw runtime value regardless of emulation.
	assert.Equal(t, runtime.GOARCH, data.GOARCH, "GOARCH always preserves raw runtime value")

	if runtime.GOOS == "windows" && runtime.GOARCH == "arm64" {
		assert.Equal(t, "amd64", data.Arch, "WindowsArmEmulation should fall back Arch to amd64 on windows/arm64")
	} else {
		assert.Equal(t, runtime.GOARCH, data.Arch, "WindowsArmEmulation should not affect Arch on non-windows-arm64")
	}
}

// =============================================================================
// github_archive package type tests
// =============================================================================
//
// Cross-references upstream aquaproj/aqua test coverage:
//   - Validate fails when repo_owner/repo_name missing
//     (aqua: pkg/config/registry/package_info_test.go)
//   - GetFormat hardcodes "tar.gz" regardless of Format field
//     (aqua: pkg/config/registry/package_info.go GetFormat)
//   - RenderAsset returns "" for github_archive (no separate asset name)
//     (aqua: pkg/config/package_test.go)
//
// Atmos has no separate RenderAsset; the URL is built directly via BuildAssetURL.
// The "asset field ignored" subtest below is the behavioral equivalent.

// TestBuildAssetURL_GitHubArchiveType covers URL building for the github_archive
// type, including version_prefix handling and the fields that must be ignored.
func TestBuildAssetURL_GitHubArchiveType(t *testing.T) {
	installer := &Installer{}

	tests := []struct {
		name    string
		tool    *registry.Tool
		version string
		want    string
	}{
		{
			name: "default tag URL with no version_prefix",
			tool: &registry.Tool{
				Type:      "github_archive",
				RepoOwner: "npryce",
				RepoName:  "adr-tools",
			},
			version: "3.0.0",
			want:    "https://github.com/npryce/adr-tools/archive/refs/tags/3.0.0.tar.gz",
		},
		{
			name: "version_prefix v adds v to URL",
			tool: &registry.Tool{
				Type:          "github_archive",
				RepoOwner:     "tfutils",
				RepoName:      "tfenv",
				VersionPrefix: "v",
			},
			version: "3.0.0",
			want:    "https://github.com/tfutils/tfenv/archive/refs/tags/v3.0.0.tar.gz",
		},
		{
			name: "version already has matching prefix is not doubled",
			tool: &registry.Tool{
				Type:          "github_archive",
				RepoOwner:     "tfutils",
				RepoName:      "tfenv",
				VersionPrefix: "v",
			},
			version: "v3.0.0",
			want:    "https://github.com/tfutils/tfenv/archive/refs/tags/v3.0.0.tar.gz",
		},
		{
			name: "asset field is ignored (no separate asset for archives)",
			tool: &registry.Tool{
				Type:      "github_archive",
				RepoOwner: "npryce",
				RepoName:  "adr-tools",
				Asset:     "ignored-{{.Version}}.tar.gz",
			},
			version: "3.0.0",
			want:    "https://github.com/npryce/adr-tools/archive/refs/tags/3.0.0.tar.gz",
		},
		{
			name: "url field is ignored",
			tool: &registry.Tool{
				Type:      "github_archive",
				RepoOwner: "npryce",
				RepoName:  "adr-tools",
				URL:       "https://example.com/should-not-be-used",
			},
			version: "3.0.0",
			want:    "https://github.com/npryce/adr-tools/archive/refs/tags/3.0.0.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := installer.BuildAssetURL(tt.tool, tt.version)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildAssetURL_GitHubArchive_MissingOwner verifies required-field validation
// mirrors aqua's Validate() behavior for github_archive (repo_owner required).
func TestBuildAssetURL_GitHubArchive_MissingOwner(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:     "github_archive",
		RepoName: "adr-tools",
	}

	_, err := installer.BuildAssetURL(tool, "3.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RepoOwner and RepoName must be set")
	assert.Contains(t, err.Error(), "github_archive")
}

// TestBuildAssetURL_GitHubArchive_MissingName verifies required-field validation
// mirrors aqua's Validate() behavior for github_archive (repo_name required).
func TestBuildAssetURL_GitHubArchive_MissingName(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "github_archive",
		RepoOwner: "npryce",
	}

	_, err := installer.BuildAssetURL(tool, "3.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RepoOwner and RepoName must be set")
	assert.Contains(t, err.Error(), "github_archive")
}

// TestBuildAssetURL_GitHubArchive_FormatDefaultsToTarGz mirrors aqua's GetFormat()
// behavior: github_archive always produces a .tar.gz URL regardless of Format /
// FormatOverrides settings. See aquaproj/aqua pkg/config/registry/package_info.go:GetFormat.
func TestBuildAssetURL_GitHubArchive_FormatDefaultsToTarGz(t *testing.T) {
	installer := &Installer{}

	tests := []struct {
		name string
		tool *registry.Tool
	}{
		{
			name: "Format unset",
			tool: &registry.Tool{
				Type:      "github_archive",
				RepoOwner: "npryce",
				RepoName:  "adr-tools",
			},
		},
		{
			name: "Format=zip is ignored",
			tool: &registry.Tool{
				Type:      "github_archive",
				RepoOwner: "npryce",
				RepoName:  "adr-tools",
				Format:    "zip",
			},
		},
		{
			name: "Format=tar.xz is ignored",
			tool: &registry.Tool{
				Type:      "github_archive",
				RepoOwner: "npryce",
				RepoName:  "adr-tools",
				Format:    "tar.xz",
			},
		},
		{
			name: "FormatOverrides are ignored",
			tool: &registry.Tool{
				Type:      "github_archive",
				RepoOwner: "npryce",
				RepoName:  "adr-tools",
				FormatOverrides: []registry.FormatOverride{
					{GOOS: runtime.GOOS, Format: "zip"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := installer.BuildAssetURL(tt.tool, "3.0.0")
			require.NoError(t, err)
			assert.True(t, strings.HasSuffix(got, ".tar.gz"),
				"github_archive URL must end with .tar.gz regardless of Format, got %q", got)
		})
	}
}

// TestBuildAssetURL_GitHubArchive_URLPattern verifies the exact URL host and path
// pattern used by upstream aqua (archive/refs/tags endpoint, not codeload or API).
func TestBuildAssetURL_GitHubArchive_URLPattern(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "github_archive",
		RepoOwner: "npryce",
		RepoName:  "adr-tools",
	}

	got, err := installer.BuildAssetURL(tool, "3.0.0")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(got, "https://github.com/"),
		"URL must use github.com host, got %q", got)
	assert.Contains(t, got, "/archive/refs/tags/",
		"URL must use /archive/refs/tags/ endpoint, got %q", got)
	assert.True(t, strings.HasSuffix(got, ".tar.gz"),
		"URL must end with .tar.gz, got %q", got)
}

// TestExpandFileSrcTemplate_GitHubArchiveTrimV verifies the documented Aqua idiom
// for github_archive: files[].src uses "{{trimV .Version}}" to match GitHub's
// archive root directory name (e.g., "adr-tools-3.0.0/src/adr" for version v3.0.0).
func TestExpandFileSrcTemplate_GitHubArchiveTrimV(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:          "github_archive",
		RepoOwner:     "npryce",
		RepoName:      "adr-tools",
		VersionPrefix: "v",
		Version:       "v3.0.0",
	}

	got, err := installer.expandFileSrcTemplate("adr-tools-{{trimV .Version}}/src/adr", tool)
	require.NoError(t, err)
	assert.Equal(t, "adr-tools-3.0.0/src/adr", got)
}

// TestExpandFileSrcTemplate_GitHubArchiveVersion verifies that {{.Version}} (without
// trimV) is used literally when version_prefix is unset.
func TestExpandFileSrcTemplate_GitHubArchiveVersion(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "github_archive",
		RepoOwner: "npryce",
		RepoName:  "adr-tools",
		Version:   "3.0.0",
	}

	got, err := installer.expandFileSrcTemplate("adr-tools-{{.Version}}/src/adr", tool)
	require.NoError(t, err)
	assert.Equal(t, "adr-tools-3.0.0/src/adr", got)
}

// =============================================================================
// github_content package type tests
// =============================================================================
//
// Cross-references upstream aquaproj/aqua test coverage:
//   - Validate fails when repo_owner/repo_name/path missing
//     (aqua: pkg/config/registry/package_info.go Validate)
//   - URL is built from raw.githubusercontent.com with tag and path
//     (aqua: pkg/download/github_content.go)
//   - Asset, URL, Format, FormatOverrides fields are ignored
//
// Atmos splits the work into a validator + formatter + dispatcher to keep
// each piece independently testable. Tests below exercise each layer.

// TestValidateGitHubContentFields exercises every branch of the pure validator.
func TestValidateGitHubContentFields(t *testing.T) {
	tests := []struct {
		name      string
		tool      *registry.Tool
		wantErr   bool
		errSubstr []string
	}{
		{
			name: "all fields present is valid",
			tool: &registry.Tool{RepoOwner: "ahmetb", RepoName: "kubectx", Path: "kubens"},
		},
		{
			name:      "missing RepoOwner is error",
			tool:      &registry.Tool{RepoName: "kubectx", Path: "kubens"},
			wantErr:   true,
			errSubstr: []string{"github_content", "RepoOwner", "RepoName=\"kubectx\"", "Path=\"kubens\""},
		},
		{
			name:      "missing RepoName is error",
			tool:      &registry.Tool{RepoOwner: "ahmetb", Path: "kubens"},
			wantErr:   true,
			errSubstr: []string{"github_content", "RepoName", "RepoOwner=\"ahmetb\"", "Path=\"kubens\""},
		},
		{
			name:      "missing Path is error",
			tool:      &registry.Tool{RepoOwner: "ahmetb", RepoName: "kubectx"},
			wantErr:   true,
			errSubstr: []string{"github_content", "Path", "RepoOwner=\"ahmetb\"", "RepoName=\"kubectx\""},
		},
		{
			name:      "all three empty is one combined error",
			tool:      &registry.Tool{},
			wantErr:   true,
			errSubstr: []string{"github_content", "RepoOwner=\"\"", "RepoName=\"\"", "Path=\"\""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitHubContentFields(tt.tool)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidToolSpec)
			for _, s := range tt.errSubstr {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

// TestFormatGitHubContentURL exercises the pure URL formatter.
func TestFormatGitHubContentURL(t *testing.T) {
	tests := []struct {
		name                       string
		owner, repo, version, path string
		want                       string
	}{
		{
			name:    "typical single-file path",
			owner:   "ahmetb",
			repo:    "kubectx",
			version: "v0.9.4",
			path:    "kubens",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/v0.9.4/kubens",
		},
		{
			name:    "nested path preserves separators",
			owner:   "ahmetb",
			repo:    "kubectx",
			version: "v0.9.4",
			path:    "scripts/install.sh",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/v0.9.4/scripts/install.sh",
		},
		{
			name:    "version without v prefix passes through unchanged",
			owner:   "ahmetb",
			repo:    "kubectx",
			version: "0.9.4",
			path:    "kubens",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/0.9.4/kubens",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatGitHubContentURL(tt.owner, tt.repo, tt.version, tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildAssetURL_GitHubContentType exercises end-to-end URL building via
// the method receiver, including version_prefix handling and the fields that
// must be ignored.
func TestBuildAssetURL_GitHubContentType(t *testing.T) {
	installer := &Installer{}

	tests := []struct {
		name    string
		tool    *registry.Tool
		version string
		want    string
	}{
		{
			name: "default URL with no version_prefix",
			tool: &registry.Tool{
				Type:      "github_content",
				RepoOwner: "ahmetb",
				RepoName:  "kubectx",
				Path:      "kubens",
			},
			version: "0.9.4",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/0.9.4/kubens",
		},
		{
			name: "version_prefix v adds v to URL",
			tool: &registry.Tool{
				Type:          "github_content",
				RepoOwner:     "ahmetb",
				RepoName:      "kubectx",
				Path:          "kubens",
				VersionPrefix: "v",
			},
			version: "0.9.4",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/v0.9.4/kubens",
		},
		{
			name: "version already has matching prefix is not doubled",
			tool: &registry.Tool{
				Type:          "github_content",
				RepoOwner:     "ahmetb",
				RepoName:      "kubectx",
				Path:          "kubens",
				VersionPrefix: "v",
			},
			version: "v0.9.4",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/v0.9.4/kubens",
		},
		{
			name: "asset field is ignored",
			tool: &registry.Tool{
				Type:      "github_content",
				RepoOwner: "ahmetb",
				RepoName:  "kubectx",
				Path:      "kubens",
				Asset:     "ignored-{{.Version}}.tar.gz",
			},
			version: "0.9.4",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/0.9.4/kubens",
		},
		{
			name: "url field is ignored",
			tool: &registry.Tool{
				Type:      "github_content",
				RepoOwner: "ahmetb",
				RepoName:  "kubectx",
				Path:      "kubens",
				URL:       "https://example.com/should-not-be-used",
			},
			version: "0.9.4",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/0.9.4/kubens",
		},
		{
			name: "format field is ignored (no archive extension applied)",
			tool: &registry.Tool{
				Type:      "github_content",
				RepoOwner: "ahmetb",
				RepoName:  "kubectx",
				Path:      "kubens",
				Format:    "zip",
			},
			version: "0.9.4",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/0.9.4/kubens",
		},
		{
			name: "nested path is preserved",
			tool: &registry.Tool{
				Type:      "github_content",
				RepoOwner: "ahmetb",
				RepoName:  "kubectx",
				Path:      "scripts/install.sh",
			},
			version: "0.9.4",
			want:    "https://raw.githubusercontent.com/ahmetb/kubectx/0.9.4/scripts/install.sh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := installer.BuildAssetURL(tt.tool, tt.version)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildAssetURL_GitHubContent_MissingFields ensures the method wires the
// validator through and surfaces each missing-field error.
func TestBuildAssetURL_GitHubContent_MissingFields(t *testing.T) {
	installer := &Installer{}

	tests := []struct {
		name string
		tool *registry.Tool
	}{
		{
			name: "missing RepoOwner",
			tool: &registry.Tool{Type: "github_content", RepoName: "kubectx", Path: "kubens"},
		},
		{
			name: "missing RepoName",
			tool: &registry.Tool{Type: "github_content", RepoOwner: "ahmetb", Path: "kubens"},
		},
		{
			name: "missing Path",
			tool: &registry.Tool{Type: "github_content", RepoOwner: "ahmetb", RepoName: "kubectx"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := installer.BuildAssetURL(tt.tool, "0.9.4")
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidToolSpec)
			assert.Contains(t, err.Error(), "github_content")
		})
	}
}

// TestBuildAssetURL_GitHubContent_URLPattern verifies the exact host and
// endpoint used by upstream aqua (raw.githubusercontent.com, not raw.github.com
// and not the API).
func TestBuildAssetURL_GitHubContent_URLPattern(t *testing.T) {
	installer := &Installer{}

	tool := &registry.Tool{
		Type:      "github_content",
		RepoOwner: "ahmetb",
		RepoName:  "kubectx",
		Path:      "kubens",
	}

	got, err := installer.BuildAssetURL(tool, "0.9.4")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(got, "https://raw.githubusercontent.com/"),
		"URL must use raw.githubusercontent.com host, got %q", got)
	assert.Contains(t, got, "/ahmetb/kubectx/0.9.4/kubens",
		"URL must contain owner/repo/version/path, got %q", got)
}
