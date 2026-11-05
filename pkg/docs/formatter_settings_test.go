package docs

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestTerraformFormattingSettings exercises decoded configuration through the production renderer.
func TestTerraformFormattingSettings(t *testing.T) {
	for _, format := range []string{"markdown table", "markdown"} {
		for _, section := range []string{"providers", "inputs", "outputs"} {
			t.Run(format+"/"+section, func(t *testing.T) {
				docs := nativeDocsTestConfig(t, `{{ (ds "config").terraform_docs }}|native={{ len (terraform).Inputs }}`)
				require.NoError(t, yaml.Unmarshal([]byte("enabled: true\nformat: "+format+"\nshow_"+section+": false\n"), &docs.Terraform))
				require.NoError(t, Generate(&schema.AtmosConfiguration{}, nativeDocsFixture(t), &docs))
				actual := readDocsTestFile(t, docs.Output)
				assert.NotContains(t, actual, `name="`+strings.TrimSuffix(section, "s")+`_`)
				assert.Contains(t, actual, "native=7")
				for sibling, anchor := range map[string]string{"providers": `name="provider_aws"`, "inputs": `name="input_bucket_name"`, "outputs": `name="output_bucket_id"`} {
					if sibling != section {
						assert.Contains(t, actual, anchor)
					}
				}
			})
		}
	}
}

// TestTerraformFormattingLayout verifies empty sections and heading levels obey configuration.
func TestTerraformFormattingLayout(t *testing.T) {
	docs := nativeDocsTestConfig(t, `{{ (ds "config").terraform_docs }}`)
	docs.Terraform.HideEmpty = true
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, t.TempDir(), &docs))
	assert.Empty(t, strings.TrimSpace(readDocsTestFile(t, docs.Output)))
	docs.Terraform.HideEmpty = false
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, t.TempDir(), &docs))
	assert.Contains(t, readDocsTestFile(t, docs.Output), "No inputs.")
	docs.Terraform.IndentLevel = 4
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, nativeDocsFixture(t), &docs))
	assert.Contains(t, readDocsTestFile(t, docs.Output), "#### Inputs")
	assert.NotContains(t, readDocsTestFile(t, docs.Output), "\n## Inputs")
}

// TestTerraformFormattingEdition preserves historical output for older edition pins.
func TestTerraformFormattingEdition(t *testing.T) {
	for _, pin := range []string{"", "2026-10-16", "2026-10-17", "2026-10-18", "invalid"} {
		t.Run(pin, func(t *testing.T) {
			docs := nativeDocsTestConfig(t, `{{ (ds "config").terraform_docs }}`)
			require.NoError(t, yaml.Unmarshal([]byte("enabled: true\nshow_inputs: false\n"), &docs.Terraform))
			err := Generate(&schema.AtmosConfiguration{Edition: pin}, nativeDocsFixture(t), &docs)
			if pin == "invalid" {
				require.ErrorIs(t, err, edition.ErrInvalidEdition)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, pin == "2026-10-16", strings.Contains(readDocsTestFile(t, docs.Output), "## Inputs"))
		})
	}
}

// TestGenerateAbsoluteSource resolves absolute module paths without joining the base twice.
func TestGenerateAbsoluteSource(t *testing.T) {
	docs := nativeDocsTestConfig(t, `{{ with terraform }}{{ len .Inputs }}{{ else }}missing{{ end }}`)
	docs.Terraform.Source = nativeDocsFixture(t)
	require.True(t, filepath.IsAbs(docs.Terraform.Source))
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, t.TempDir(), &docs))
	assert.Equal(t, "7", readDocsTestFile(t, docs.Output))
}
