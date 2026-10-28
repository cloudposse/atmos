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

func TestTagConditionalImports(t *testing.T) {
	const gate = "import:\n  - path: pci\n    when: !cel \"'pci' in tags\"\n"
	const negativeGate = "import:\n  - path: pci\n    when: !cel \"!('pci' in tags)\"\n"
	const enabled = "metadata: {tags: [pci]}\n"
	const disabled = "metadata: {tags: []}\n"
	for _, tc := range []struct {
		name     string
		strategy string
		files    map[string]string
		want     bool
	}{
		{"negative membership on unknown list", "", map[string]string{"manifest": "metadata: {tags: !env TAGS}\n" + negativeGate}, false},
		{"negative membership on missing list", "", map[string]string{"manifest": negativeGate}, true},
		{"negative membership on null list", "", map[string]string{"manifest": "metadata: {tags: null}\n" + negativeGate}, true},
		{"quoted bang tag is literal", "", map[string]string{"manifest": "metadata: {tags: ['!env PCI']}\nimport:\n  - path: pci\n    when: !cel \"'!env PCI' in tags\"\n"}, true},
		{"unknown append remains unknown", "append", map[string]string{"manifest": "metadata: {tags: [other]}\nimport: [defaults, parent]\n", "defaults": "metadata: {tags: !env TAGS}\n", "parent": negativeGate}, false},
		{"unknown merge remains unknown", "merge", map[string]string{"manifest": "metadata: {tags: [other]}\nimport: [defaults, parent]\n", "defaults": "metadata: {tags: !env TAGS}\n", "parent": negativeGate}, false},
		{"known replacement clears unknown", "", map[string]string{"manifest": "metadata: {tags: [other]}\nimport: [defaults, parent]\n", "defaults": "metadata: {tags: !env TAGS}\n", "parent": negativeGate}, true},
		{"inline", "", map[string]string{"manifest": enabled + gate}, true},
		{"empty", "", map[string]string{"manifest": disabled + gate}, false},
		{"missing", "", map[string]string{"manifest": gate}, false},
		{"case sensitive", "", map[string]string{"manifest": "metadata: {tags: [PCI]}\n" + gate}, false},
		{"earlier import", "", map[string]string{"manifest": "import: [defaults, parent]\n", "defaults": enabled, "parent": gate}, true},
		{"later import", "", map[string]string{"manifest": "import: [parent, defaults]\n", "defaults": enabled, "parent": gate}, false},
		{"local replaces inherited tags", "", map[string]string{"manifest": disabled + "import: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"ancestor replaces child tags", "", map[string]string{"manifest": disabled + "import: [parent]\n", "parent": enabled + gate}, false},
		{"child replaces earlier defaults", "", map[string]string{"manifest": "import: [defaults, parent]\n", "defaults": enabled, "parent": disabled + gate}, false},
		{"component tags excluded", "", map[string]string{"manifest": "components: {terraform: {app: {metadata: {tags: [pci]}}}}\n" + gate}, false},
		{"null masks inherited tags", "", map[string]string{"manifest": "metadata: {tags: null}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"function masks inherited tags", "", map[string]string{"manifest": "metadata: {tags: !env TAGS}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"unknown member masks list", "", map[string]string{"manifest": "metadata: {tags: [!env TAG]}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"template masks inherited tags", "", map[string]string{"manifest": "metadata: {tags: ['{{ .vars.tag }}']}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, false},
		{"append strategy", "append", map[string]string{"manifest": "metadata: {tags: [other]}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, true},
		{"merge strategy preserves scalar prefix", "merge", map[string]string{"manifest": "metadata: {tags: [other]}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, true},
		{"merge strategy extends shorter list", "merge", map[string]string{"manifest": enabled + "import: [defaults, parent]\n", "defaults": disabled, "parent": gate}, true},
		{"explicit append in ancestor", "", map[string]string{"manifest": "metadata: {tags: !append [other]}\nimport: [defaults, parent]\n", "defaults": enabled, "parent": gate}, true},
		{"glob shares scope", "", map[string]string{"manifest": "import: ['group/*']\n", "group/a": enabled, "group/b": gate}, false},
		{"glob visible afterward", "", map[string]string{"manifest": "import: ['group/*', parent]\n", "group/a": enabled, "parent": gate}, true},
		{"combined labels and tags", "", map[string]string{"manifest": "metadata: {labels: {pci: true}, tags: [pci]}\nimport:\n  - path: pci\n    when: !cel labels.pci == true && 'pci' in tags\n"}, true},
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
			config := &schema.AtmosConfiguration{StacksBaseAbsolutePath: dir, IncludeStackAbsolutePaths: []string{path}, Settings: schema.AtmosSettings{ListMergeStrategy: tc.strategy}}
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
