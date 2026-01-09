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
	}

	data := buildTemplateData(tool, "2.15.0")

	assert.Equal(t, "v2.15.0", data.Version)
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
			name:            "default prefix adds v",
			version:         "2.15.0",
			versionPrefix:   "",
			expectedVersion: "v2.15.0",
			expectedSemVer:  "2.15.0",
		},
		{
			name:            "version already has default prefix",
			version:         "v2.15.0",
			versionPrefix:   "",
			expectedVersion: "v2.15.0",
			expectedSemVer:  "2.15.0",
		},
		{
			name:            "custom prefix",
			version:         "2.15.0",
			versionPrefix:   "release-",
			expectedVersion: "release-2.15.0",
			expectedSemVer:  "2.15.0",
		},
		{
			name:            "version already has custom prefix",
			version:         "release-2.15.0",
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

	tool := &registry.Tool{
		Type:      "github_release",
		RepoOwner: "hashicorp",
		RepoName:  "terraform",
		Asset:     "terraform_{{trimV .Version}}_{{.OS}}_{{.Arch}}.zip",
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
