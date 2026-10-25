package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// The same fixtures exercise normal stack loading and the early auth scan, which must
// choose identical imports without depending on templates, credentials, or YAML functions.
func TestLabelConditionalImports(t *testing.T) {
	const gate = "import:\n  - path: pci\n    when: !cel labels.pci == true\n"
	const enabled = "metadata: {labels: {pci: true}}\n"
	const disabled = "metadata: {labels: {pci: false}}\n"
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"inline true", map[string]string{"manifest": enabled + gate}, true},
		{"inline false", map[string]string{"manifest": disabled + gate}, false},
		{"missing", map[string]string{"manifest": gate}, false},
		{"string is not boolean", map[string]string{"manifest": "metadata: {labels: {pci: 'true'}}\n" + gate}, false},
		{"earlier import", map[string]string{"manifest": "import:\n  - defaults\n  - parent\n", "defaults": enabled, "parent": gate}, true},
		{"later import unavailable", map[string]string{"manifest": "import:\n  - parent\n  - defaults\n", "defaults": enabled, "parent": gate}, false},
		{"local overrides earlier", map[string]string{"manifest": disabled + "import: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"ancestor overrides child", map[string]string{"manifest": disabled + "import: [parent]\n", "parent": enabled + gate}, false},
		{"child overrides earlier defaults", map[string]string{"manifest": "import: [defaults, parent]\n", "defaults": enabled, "parent": disabled + gate}, false},
		{"deep defaults", map[string]string{"manifest": "import: [defaults, parent]\n", "defaults": "import: [nested]\n", "nested": enabled, "parent": gate}, true},
		{"later sibling overrides earlier", map[string]string{"manifest": "import: [defaults, override, parent]\n", "defaults": enabled, "override": disabled, "parent": gate}, false},
		{"null masks inherited flag", map[string]string{"manifest": "metadata: {labels: {pci: null}}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"null map masks inherited flag", map[string]string{"manifest": "metadata: {labels: null}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"function masks inherited flag", map[string]string{"manifest": "metadata: {labels: {pci: !env PCI}}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"template masks inherited flag", map[string]string{"manifest": "metadata: {labels: {pci: '{{ .vars.pci }}'}}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"component labels excluded", map[string]string{"manifest": "components: {terraform: {app: {metadata: {labels: {pci: true}}}}}\n" + gate}, false},
		{"repeated file reevaluated", map[string]string{"manifest": "import: [parent, defaults, parent]\n", "defaults": enabled, "parent": gate}, true},
		{"glob siblings share scope", map[string]string{"manifest": "import: ['group/*']\n", "group/a": enabled, "group/b": gate}, false},
		{"glob labels visible afterward", map[string]string{"manifest": "import: ['group/*', parent]\n", "group/a": enabled, "group/b": "vars: {}\n", "parent": gate}, true},
		{"compound and bracket syntax", map[string]string{"manifest": enabled + "import:\n  - path: pci\n    when: {all: [always, !cel 'labels[\"pci\"] == true']}\n"}, true},
		{"skipped imports provide no labels", map[string]string{"manifest": "import:\n  - path: defaults\n    when: never\n  - parent\n", "defaults": enabled, "parent": gate}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.files["pci"] = "vars: {pci_enabled: true}\nauth: {identities: {pci: {default: true}}}\n"
			for name, content := range tc.files {
				path := filepath.Join(dir, name+".yaml")
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			}
			path := filepath.Join(dir, "manifest.yaml")
			config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: dir, IncludeStackAbsolutePaths: []string{path}}
			result, err := ProcessYAMLConfigFileWithOptions(config, dir, path, StackLoadOptions{SkipTemplates: true})
			require.NoError(t, err)
			vars, _ := result.DeepMergedConfig["vars"].(map[string]any)
			assert.Equal(t, tc.want, vars["pci_enabled"] == true, "normal loader")
			defaults, err := cfg.LoadStackAuthDefaults(config)
			require.NoError(t, err)
			assert.Equal(t, tc.want, defaults["pci"], "early auth loader")
		})
	}
}
