package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStackLoaderConditionalImportMetadataScope(t *testing.T) {
	for _, staticOnly := range []bool{false, true} {
		for _, strategy := range []string{"replace", "append"} {
			name := strategy + "/runtime"
			if staticOnly {
				name = strategy + "/static"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				files := map[string]string{
					"dev.yaml": `metadata: {tags: [root]}
import:
  - defaults
  - path: branch
    when: !cel "'default' in tags && 'root' in tags"
components:
  terraform:
    app:
      vars: {base: true}
`,
					"defaults.yaml": "metadata: {tags: [default], labels: {enabled: true}}\n",
					"branch.yaml": `import:
  - path: selected
    when: !cel "labels.enabled == true && 'default' in tags && 'root' in tags"
`,
					"selected.yaml": "vars: {selected: true}\n",
				}
				for name, content := range files {
					require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o600))
				}
				config := &schema.AtmosConfiguration{}
				config.Settings.ListMergeStrategy = strategy
				var options *StackLoadOptions
				if staticOnly {
					options = &StackLoadOptions{StaticOnly: true, SkipTemplates: true}
				}
				_, stacks, _, _, err := processYAMLConfigFilesWithOptions(config, root, "", "", "", "",
					[]string{filepath.Join(root, "dev.yaml")}, false, true, false, options)
				require.NoError(t, err)
				stack := stacks["dev"].(map[string]any)
				component := stack["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
				vars := component["vars"].(map[string]any)
				require.Equal(t, true, vars["base"])
				if strategy == "append" {
					require.Equal(t, true, vars["selected"], "nested import must see inherited metadata and the configured list strategy")
				} else {
					require.NotContains(t, vars, "selected", "replacement removes the earlier tag and must skip the import")
				}
			})
		}
	}
}
