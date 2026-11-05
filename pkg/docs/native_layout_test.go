package docs

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"

	"github.com/cloudposse/atmos/pkg/schema"
)

// renderLayoutFixture exercises the shared README template with reduced, real module shapes.
func renderLayoutFixture(t *testing.T, name string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "docs-generate", "native-layout", name))
	require.NoError(t, err)
	docs := nativeDocsTestConfig(t, readDocsTestFile(t, filepath.Join(nativeDocsFixture(t), "README.md.gomplate")))
	docs.Input = []any{map[string]any{"name": name, "description": "README layout regression"}}
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, dir, &docs))
	return readDocsTestFile(t, docs.Output)
}

// TestNativeLayoutEmptySections covers groups whose backing collections differ from their display filters.
func TestNativeLayoutEmptySections(t *testing.T) {
	cases := []struct {
		name            string
		present, absent []string
	}{
		{"context-only", []string{"Context Variables", "### `name`"}, []string{"Required Variables", "Optional Variables", "Resources", "Data Sources"}},
		{"resource-only", []string{"Resources", "terraform_data.sample"}, []string{"Data Sources", "Required Variables", "Optional Variables", "Context Variables"}},
		{"data-only", []string{"Data Sources", "aws_caller_identity.current"}, []string{"## Resources", "Required Variables", "Optional Variables", "Context Variables"}},
		{"required-only", []string{"Required Variables", "### `bucket`"}, []string{"Optional Variables", "Context Variables", "#context-variables"}},
		{"optional-only", []string{"Optional Variables", "### `item_count`"}, []string{"Required Variables", "Context Variables"}},
		{"empty", nil, []string{"Required Variables", "Optional Variables", "Context Variables", "Resources", "Data Sources", "Outputs"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := renderLayoutFixture(t, tc.name)
			for _, s := range tc.present {
				assert.Contains(t, actual, s)
			}
			for _, s := range tc.absent {
				assert.NotContains(t, actual, s)
			}
		})
	}
}

// TestNativeLayoutMultilineMarkdown verifies rendered HTML, not just matching Markdown snapshots.
func TestNativeLayoutMultilineMarkdown(t *testing.T) {
	docs := nativeDocsTestConfig(t, readDocsTestFile(t, filepath.Join(nativeDocsFixture(t), "README.md.gomplate")))
	docs.Input = []any{map[string]any{"name": "layout", "description": "types"}}
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, nativeDocsFixture(t), &docs))
	var rendered bytes.Buffer
	require.NoError(t, goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe())).Convert([]byte(readDocsTestFile(t, docs.Output)), &rendered))
	actual := rendered.String()
	assert.Contains(t, actual, "object({\n    enabled = bool\n    ports   = list(number)\n  })\n</code></pre>")
	assert.NotContains(t, actual, "&lt;dt&gt;Default value")
}

// TestNativeLayoutModuleSources preserves local and Git targets without inventing registry URLs.
func TestNativeLayoutModuleSources(t *testing.T) {
	actual := renderLayoutFixture(t, "module-sources")
	assert.Contains(t, actual, "](./child)")
	assert.Contains(t, actual, "](../resource-only)")
	assert.Contains(t, actual, "](https://github.com/cloudposse/terraform-null-label.git)")
	assert.Contains(t, actual, "https://registry.terraform.io/modules/cloudposse/label/null/latest")
	assert.NotContains(t, actual, "modules/git::")
	assert.NotContains(t, actual, "modules/./")
	assert.False(t, strings.Contains(actual, "/submodules/github.com"))
}
