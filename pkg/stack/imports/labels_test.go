package imports

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestReadLabels(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          schema.StackImportLabels
	}{
		{"absent", "vars: {}", schema.StackImportLabels{}},
		{"literal", "metadata: {labels: {pci: true, owner: security}}", schema.StackImportLabels{Values: map[string]any{"pci": true, "owner": "security"}}},
		{"invalid YAML", "metadata: [", schema.StackImportLabels{Reset: true}},
		{"metadata without labels", "metadata: {name: test}", schema.StackImportLabels{}},
		{"unsupported value", "metadata: {labels: {pci: [true]}}", schema.StackImportLabels{Values: map[string]any{"pci": nil}}},
		{"null map", "metadata: {labels: null}", schema.StackImportLabels{Reset: true}},
		{"dynamic map", "metadata: {labels: !env LABELS}", schema.StackImportLabels{Reset: true}},
		{"dynamic metadata", "metadata: !env METADATA", schema.StackImportLabels{Reset: true}},
		{"unknown values mask defaults", `metadata: {labels: {pci: !env PCI, owner: "{{ .vars.owner }}", team: "[[ .vars.team ]]", old: null}}`, schema.StackImportLabels{Values: map[string]any{"pci": nil, "owner": nil, "team": nil, "old": nil}}},
		{"component labels excluded", "components: {terraform: {app: {metadata: {labels: {pci: true}}}}}", schema.StackImportLabels{}},
		{"alias", "defaults: &labels {pci: true}\nmetadata: {labels: *labels}", schema.StackImportLabels{Values: map[string]any{"pci": true}}},
		{"string alias", "owner: &owner security\nmetadata: {labels: {owner: *owner}}", schema.StackImportLabels{Values: map[string]any{"owner": "security"}}},
		{"scalar alias", "flag: &pci true\nmetadata: {labels: {pci: *pci}}", schema.StackImportLabels{Values: map[string]any{"pci": true}}},
		{"string function", "metadata: {labels: {pci: '!env PCI'}}", schema.StackImportLabels{Values: map[string]any{"pci": nil}}},
		{"merge key", "defaults: &labels {pci: true, owner: ops}\nmetadata: {labels: {<<: *labels, pci: false}}", schema.StackImportLabels{Values: map[string]any{"pci": false, "owner": "ops"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ReadLabels(tc.content, []string{"[[", "]]"}))
		})
	}
}

func TestLabelScope(t *testing.T) {
	defaults := schema.StackImportLabels{Values: map[string]any{"pci": true, "owner": "ops"}}
	local := schema.StackImportLabels{Values: map[string]any{"pci": false}}
	scope := LabelScope{}
	child := scope.Child(defaults, local)
	assert.Equal(t, map[string]any{"pci": false, "owner": "ops"}, child.Effective(schema.StackImportLabels{}, defaults))
	assert.Equal(t, map[string]any{"pci": false}, scope.Effective(defaults, schema.StackImportLabels{Reset: true, Values: local.Values}))
	assert.Equal(t, map[string]any{"owner": "ops"}, scope.Effective(defaults, schema.StackImportLabels{Values: map[string]any{"pci": nil}}))
	assert.Equal(t, true, defaults.Values["pci"], "inputs must remain immutable across parallel glob matches")
}
