package toolchain

import (
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/toolchain/registry"
)

// TestAssetTemplateWithFormat ensures the Format field is available in asset templates.
func TestAssetTemplateWithFormat(t *testing.T) {
	tests := []struct {
		name           string
		assetTemplate  string
		tool           *registry.Tool
		version        string
		expectedOutput string
	}{
		{
			name:          "format in template",
			assetTemplate: "tool_{{.Version}}_{{.OS}}_{{.Arch}}.{{.Format}}",
			tool: &registry.Tool{
				RepoOwner: "owner",
				RepoName:  "tool",
				Format:    "zip",
			},
			version:        "1.2.3",
			expectedOutput: "tool_1.2.3_darwin_arm64.zip",
		},
		{
			name:          "format tar.gz",
			assetTemplate: "{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.{{.Format}}",
			tool: &registry.Tool{
				RepoOwner: "opentofu",
				RepoName:  "opentofu",
				Format:    "tar.gz",
			},
			version:        "1.10.7",
			expectedOutput: "opentofu_1.10.7_darwin_arm64.tar.gz",
		},
		{
			name:          "format raw (no extension)",
			assetTemplate: "{{.RepoName}}-{{.OS}}-{{.Arch}}",
			tool: &registry.Tool{
				RepoOwner: "plumber-cd",
				RepoName:  "terraform-backend-git",
				Format:    "raw",
			},
			version:        "0.1.8",
			expectedOutput: "terraform-backend-git-darwin-arm64",
		},
		{
			name:          "format with trimV function",
			assetTemplate: "tofu_{{trimV .Version}}_{{.OS}}_{{.Arch}}.{{.Format}}",
			tool: &registry.Tool{
				RepoOwner: "opentofu",
				RepoName:  "opentofu",
				Format:    "tar.gz",
			},
			version:        "v1.10.7",
			expectedOutput: "tofu_1.10.7_darwin_arm64.tar.gz",
		},
		{
			name:          "no format in template still works",
			assetTemplate: "{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.zip",
			tool: &registry.Tool{
				RepoOwner: "hashicorp",
				RepoName:  "terraform",
				Format:    "zip",
			},
			version:        "1.5.0",
			expectedOutput: "terraform_1.5.0_darwin_arm64.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create template data structure matching installer.go.
			data := struct {
				Version   string
				OS        string
				Arch      string
				RepoOwner string
				RepoName  string
				Format    string
			}{
				Version:   strings.TrimPrefix(tt.version, "v"),
				OS:        "darwin",
				Arch:      "arm64",
				RepoOwner: tt.tool.RepoOwner,
				RepoName:  tt.tool.RepoName,
				Format:    tt.tool.Format,
			}

			// Register custom template functions (same as installer.go).
			funcMap := template.FuncMap{
				"trimV": func(s string) string {
					return strings.TrimPrefix(s, "v")
				},
				"trimPrefix": func(prefix, s string) string {
					return strings.TrimPrefix(s, prefix)
				},
				"trimSuffix": func(suffix, s string) string {
					return strings.TrimSuffix(s, suffix)
				},
				"replace": func(old, new, s string) string {
					return strings.ReplaceAll(s, old, new)
				},
			}

			tmpl, err := template.New("asset").Funcs(funcMap).Parse(tt.assetTemplate)
			require.NoError(t, err, "template should parse successfully")

			var result strings.Builder
			err = tmpl.Execute(&result, data)
			require.NoError(t, err, "template should execute successfully")

			assert.Equal(t, tt.expectedOutput, result.String(), "template output should match expected")
		})
	}
}

// TestAssetTemplateWithoutFormat ensures templates work when Format is empty.
func TestAssetTemplateWithoutFormat(t *testing.T) {
	assetTemplate := "{{.RepoName}}_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"

	data := struct {
		Version   string
		OS        string
		Arch      string
		RepoOwner string
		RepoName  string
		Format    string
	}{
		Version:   "1.2.3",
		OS:        "linux",
		Arch:      "amd64",
		RepoOwner: "owner",
		RepoName:  "tool",
		Format:    "", // Empty format
	}

	tmpl, err := template.New("asset").Parse(assetTemplate)
	require.NoError(t, err)

	var result strings.Builder
	err = tmpl.Execute(&result, data)
	require.NoError(t, err)

	assert.Equal(t, "tool_1.2.3_linux_amd64.tar.gz", result.String())
}

// TestAssetTemplateFormatFieldMissing ensures we get a clear error when Format is used but not in data.
func TestAssetTemplateFormatFieldMissing(t *testing.T) {
	assetTemplate := "tool_{{.Version}}.{{.Format}}"

	// Data structure WITHOUT Format field.
	dataWithoutFormat := struct {
		Version string
	}{
		Version: "1.2.3",
	}

	tmpl, err := template.New("asset").Parse(assetTemplate)
	require.NoError(t, err)

	var result strings.Builder
	err = tmpl.Execute(&result, dataWithoutFormat)
	require.Error(t, err, "should error when Format field is missing")
	assert.Contains(t, err.Error(), "Format", "error should mention missing Format field")
}
