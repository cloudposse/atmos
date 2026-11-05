package docs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFormatterDefaultsAndFormats preserves omitted visibility and native data across formatters.
func TestFormatterDefaultsAndFormats(t *testing.T) {
	for _, format := range []string{"markdown table", "markdown", "tfvars hcl", "tfvars json"} {
		t.Run(format, func(t *testing.T) {
			settings := schema.TerraformDocsReadmeSettings{Format: format}
			result, err := runTerraformDocs(nativeDocsFixture(t), &settings)
			require.NoError(t, err)
			require.Len(t, result.module.Inputs, 7)
			assert.Equal(t, "bucket_name", result.module.Inputs[0].Name)
			assert.Equal(t, "tags", result.module.Inputs[6].Name)
			switch {
			case strings.HasPrefix(format, "markdown"):
				for _, anchor := range []string{`name="provider_aws"`, `name="input_bucket_name"`, `name="output_bucket_id"`} {
					assert.Contains(t, result.content, anchor)
				}
			case format == "tfvars json":
				var values map[string]any
				require.NoError(t, json.Unmarshal([]byte(result.content), &values))
				assert.Equal(t, float64(2), values["replicas"])
				assert.Nil(t, values["optional_note"])
			default:
				assert.Contains(t, result.content, "replicas")
			}
		})
	}
}

// TestFormatterSortOrder verifies configured order is shared by native objects and legacy output.
func TestFormatterSortOrder(t *testing.T) {
	for _, tc := range []struct{ sort, first, second string }{
		{"name", "bucket_name", "configuration"}, {"required", "bucket_name", "namespace"}, {"type", "legacy_mode", "tags"},
	} {
		t.Run(tc.sort, func(t *testing.T) {
			result, err := runTerraformDocs(nativeDocsFixture(t), &schema.TerraformDocsReadmeSettings{SortBy: tc.sort})
			require.NoError(t, err)
			require.Len(t, result.module.Inputs, 7)
			assert.Equal(t, tc.first, result.module.Inputs[0].Name)
			assert.Equal(t, tc.second, result.module.Inputs[1].Name)
			first := strings.Index(result.content, `name="input_`+tc.first+`"`)
			second := strings.Index(result.content, `name="input_`+tc.second+`"`)
			require.NotEqual(t, -1, first)
			require.NotEqual(t, -1, second)
			assert.Less(t, first, second)
		})
	}
}

// TestDocsFailurePreservesOutput protects an existing README when parsing or rendering fails.
func TestDocsFailurePreservesOutput(t *testing.T) {
	for _, invalid := range []string{"template", "terraform"} {
		t.Run(invalid, func(t *testing.T) {
			docs := nativeDocsTestConfig(t, `{{ missing_function }}`)
			dir := t.TempDir()
			want := errUtils.ErrRenderTemplate
			if invalid == "terraform" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`variable "broken" {`), 0o600))
				want = errUtils.ErrGenerateTerraformDocs
			}
			require.NoError(t, os.WriteFile(docs.Output, []byte("existing README\n"), 0o600))
			require.ErrorIs(t, Generate(&schema.AtmosConfiguration{}, dir, &docs), want)
			assert.Equal(t, "existing README\n", readDocsTestFile(t, docs.Output))
		})
	}
}

// TestFormatterExplicitVisibility ensures true values and omitted siblings survive YAML decoding.
func TestFormatterExplicitVisibility(t *testing.T) {
	docs := nativeDocsTestConfig(t, `{{ (ds "config").terraform_docs }}`)
	require.NoError(t, yaml.Unmarshal([]byte("enabled: true\nshow_providers: true\nshow_inputs: true\nshow_outputs: true\n"), &docs.Terraform))
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, nativeDocsFixture(t), &docs))
	for _, section := range []string{"Providers", "Inputs", "Outputs"} {
		assert.Contains(t, readDocsTestFile(t, docs.Output), "## "+section)
	}
}
