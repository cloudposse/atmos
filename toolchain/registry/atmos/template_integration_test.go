package atmos

import (
	"testing"

	aquaReg "github.com/cloudposse/atmos/toolchain/registry/aqua"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAtmosRegistry_TemplateRendering tests that Go templates in inline registry definitions
// work correctly when rendered through the Aqua BuildAssetURL function.
// This is an integration test proving template syntax like {{.OS}}, {{.Arch}}, {{.Version}} works.
func TestAtmosRegistry_TemplateRendering(t *testing.T) {
	toolsConfig := map[string]any{
		"stedolan/jq": map[string]any{
			"type": "github_release",
			"url":  "jq-{{.OS}}-{{.Arch}}",
		},
		"mikefarah/yq": map[string]any{
			"type": "github_release",
			"url":  "yq_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz",
		},
		"example/custom-tool": map[string]any{
			"type": "http",
			"url":  "https://releases.example.com/{{trimV .Version}}/tool-{{.OS}}-{{.Arch}}.zip",
		},
	}

	// Create inline registry.
	reg, err := NewAtmosRegistry(toolsConfig)
	require.NoError(t, err)

	// Create Aqua registry for template rendering (BuildAssetURL).
	aqua := aquaReg.NewAquaRegistry()

	t.Run("github_release with OS and Arch templates", func(t *testing.T) {
		// Get tool from inline registry.
		tool, err := reg.GetToolWithVersion("stedolan", "jq", "1.7.1")
		require.NoError(t, err)

		// Render template using Aqua's BuildAssetURL.
		url, err := aqua.BuildAssetURL(tool, "1.7.1")
		require.NoError(t, err)

		// Verify template was rendered correctly.
		assert.Contains(t, url, "jq-")
		assert.Contains(t, url, "https://github.com/stedolan/jq/releases/download/v1.7.1/")
		// URL should contain OS and Arch (darwin/linux/windows, amd64/arm64).
		assert.NotContains(t, url, "{{.OS}}")
		assert.NotContains(t, url, "{{.Arch}}")
	})

	t.Run("github_release with Version template", func(t *testing.T) {
		tool, err := reg.GetToolWithVersion("mikefarah", "yq", "4.44.1")
		require.NoError(t, err)

		url, err := aqua.BuildAssetURL(tool, "4.44.1")
		require.NoError(t, err)

		// Verify version template was rendered.
		assert.Contains(t, url, "yq_4.44.1_")
		assert.NotContains(t, url, "{{.Version}}")
	})

	t.Run("http type with trimV template function", func(t *testing.T) {
		tool, err := reg.GetToolWithVersion("example", "custom-tool", "v2.0.0")
		require.NoError(t, err)

		url, err := aqua.BuildAssetURL(tool, "v2.0.0")
		require.NoError(t, err)

		// Verify trimV function worked (v2.0.0 -> 2.0.0).
		assert.Contains(t, url, "/2.0.0/")
		assert.NotContains(t, url, "{{trimV .Version}}")
	})

	t.Run("binary_name field is respected", func(t *testing.T) {
		toolsConfigWithBinary := map[string]any{
			"hashicorp/terraform": map[string]any{
				"type":        "github_release",
				"url":         "terraform_{{.Version}}_{{.OS}}_{{.Arch}}.zip",
				"binary_name": "terraform",
			},
		}

		regWithBinary, err := NewAtmosRegistry(toolsConfigWithBinary)
		require.NoError(t, err)

		tool, err := regWithBinary.GetTool("hashicorp", "terraform")
		require.NoError(t, err)

		assert.Equal(t, "terraform", tool.Name)
		assert.Equal(t, "terraform", tool.BinaryName)
	})
}

// TestAtmosRegistry_TemplateVariations tests various template patterns commonly used.
func TestAtmosRegistry_TemplateVariations(t *testing.T) {
	aqua := aquaReg.NewAquaRegistry()

	tests := []struct {
		name            string
		toolConfig      map[string]any
		owner           string
		repo            string
		version         string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "simple OS/Arch pattern",
			toolConfig: map[string]any{
				"type": "github_release",
				"url":  "tool-{{.OS}}-{{.Arch}}",
			},
			owner:           "example",
			repo:            "tool",
			version:         "1.0.0",
			wantContains:    []string{"tool-", "github.com/example/tool/releases"},
			wantNotContains: []string{"{{.OS}}", "{{.Arch}}"},
		},
		{
			name: "version first pattern",
			toolConfig: map[string]any{
				"type": "github_release",
				"url":  "{{.Version}}-tool-{{.OS}}.tar.gz",
			},
			owner:           "example",
			repo:            "tool",
			version:         "2.1.0",
			wantContains:    []string{"2.1.0-tool-"},
			wantNotContains: []string{"{{.Version}}"},
		},
		{
			name: "complex http URL",
			toolConfig: map[string]any{
				"type": "http",
				"url":  "https://downloads.example.com/v{{.Version}}/{{.RepoName}}_{{.OS}}_{{.Arch}}.zip",
			},
			owner:           "myorg",
			repo:            "mytool",
			version:         "3.2.1",
			wantContains:    []string{"v3.2.1", "mytool_"},
			wantNotContains: []string{"{{.RepoName}}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolsConfig := map[string]any{
				tt.owner + "/" + tt.repo: tt.toolConfig,
			}

			reg, err := NewAtmosRegistry(toolsConfig)
			require.NoError(t, err)

			tool, err := reg.GetToolWithVersion(tt.owner, tt.repo, tt.version)
			require.NoError(t, err)

			url, err := aqua.BuildAssetURL(tool, tt.version)
			require.NoError(t, err)

			for _, want := range tt.wantContains {
				assert.Contains(t, url, want, "URL should contain %q", want)
			}

			for _, wantNot := range tt.wantNotContains {
				assert.NotContains(t, url, wantNot, "URL should not contain template syntax %q", wantNot)
			}
		})
	}
}
