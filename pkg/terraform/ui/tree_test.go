package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/schema"
	uitree "github.com/cloudposse/atmos/pkg/ui/tree"
)

func TestResolveRenderConfig_NilUsesDefaults(t *testing.T) {
	resolved := resolveRenderConfig(nil)
	assert.NotEqual(t, lipgloss.Style{}, resolved.CreateStyle, "default CreateStyle should be non-zero")
	assert.NotEqual(t, lipgloss.Style{}, resolved.DeleteStyle, "default DeleteStyle should be non-zero")
}

// TestResolveRenderConfig_HonorsCallerStyles is a regression test for resolveRenderConfig, which previously discarded any style the caller set on RenderConfig, contradicting its own documented contract that styles are populated with defaults when not explicitly set.
func TestResolveRenderConfig_HonorsCallerStyles(t *testing.T) {
	customCreate := lipgloss.NewStyle().Bold(true)
	resolved := resolveRenderConfig(&RenderConfig{CreateStyle: customCreate})

	assert.Equal(t, customCreate, resolved.CreateStyle, "caller-provided CreateStyle must be honored")

	// Styles the caller left at zero value still fall back to defaults.
	defaults := resolveRenderConfig(nil)
	assert.Equal(t, defaults.DeleteStyle, resolved.DeleteStyle, "unset DeleteStyle should still use the default")
}

// TestBuildRenderConfig_DefaultsWhenUnset is a regression test: every RenderTree() call site
// previously hardcoded RenderTreeWithConfig(nil), so components.terraform.ui.{compact,
// show_attribute_bar,max_lines} in atmos.yaml were parsed and documented but never actually
// read at render time. BuildRenderConfig is what wires them up; this locks in the documented
// defaults (compact=true, show_attribute_bar=false) when the config fields are left unset.
func TestBuildRenderConfig_DefaultsWhenUnset(t *testing.T) {
	result := BuildRenderConfig(schema.TerraformUI{})

	assert.True(t, result.Compact, "compact must default to true per the documented contract")
	assert.False(t, result.ShowAttributeBar, "show_attribute_bar must default to false")
	assert.Equal(t, 0, result.MaxLines, "max_lines must default to 0 (show all)")
}

func TestBuildRenderConfig_HonorsExplicitValues(t *testing.T) {
	compactFalse := false
	showBarTrue := true

	result := BuildRenderConfig(schema.TerraformUI{
		Compact:          &compactFalse,
		ShowAttributeBar: &showBarTrue,
		MaxLines:         5,
	})

	assert.False(t, result.Compact, "explicit compact=false must be honored")
	assert.True(t, result.ShowAttributeBar, "explicit show_attribute_bar=true must be honored")
	assert.Equal(t, 5, result.MaxLines)
}

func TestColorizedActionSymbol(t *testing.T) {
	tests := []struct {
		action   string
		expected string
	}{
		{"create", "●"},  // Green dot for create.
		{"update", "●"},  // Yellow dot for update/change in place.
		{"delete", "●"},  // Red dot for delete.
		{"replace", "●"}, // Orange dot for replace/recreate.
		{"read", "●"},    // Cyan dot for read.
		{"no-op", " "},   // Space for no-op.
		{"unknown", " "}, // Space for unknown.
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			result := colorizedActionSymbol(tt.action)
			// The result includes ANSI codes, but should contain the expected symbol.
			assert.Contains(t, result, tt.expected)
		})
	}
}

func TestDependencyTree_RenderTree_Simple(t *testing.T) {
	tree := &DependencyTree{
		Root: &TreeNode{
			Address: "root",
			Children: []*TreeNode{
				{Address: "local_file.cache", Action: "create"},
			},
		},
		Stack:     "dev",
		Component: "myapp",
	}

	result := tree.RenderTree()

	// Should contain the stack/component header.
	assert.Contains(t, result, "dev/myapp")
	// Should contain the resource.
	assert.Contains(t, result, "local_file.cache")
	// Should contain the tree connector.
	assert.Contains(t, result, "└─")
}

func TestDependencyTree_RenderTree_MultipleResources(t *testing.T) {
	tree := &DependencyTree{
		Root: &TreeNode{
			Address: "root",
			Children: []*TreeNode{
				{
					Address: "aws_vpc.main",
					Action:  "create",
					Children: []*TreeNode{
						{Address: "aws_subnet.public[0]", Action: "create"},
						{Address: "aws_subnet.public[1]", Action: "create"},
					},
				},
				{Address: "aws_security_group.default", Action: "update"},
			},
		},
		Stack:     "plat-ue2-dev",
		Component: "vpc",
	}

	result := tree.RenderTree()

	// Should contain all resources.
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "aws_subnet.public[0]")
	assert.Contains(t, result, "aws_subnet.public[1]")
	assert.Contains(t, result, "aws_security_group.default")

	// Should contain tree connectors.
	assert.Contains(t, result, "├─")
	assert.Contains(t, result, "└─")
	assert.Contains(t, result, "│")
}

func TestDependencyTree_GetChangeSummary(t *testing.T) {
	tree := &DependencyTree{
		Root: &TreeNode{
			Address: "root",
			Children: []*TreeNode{
				{Address: "aws_vpc.main", Action: "create"},
				{
					Address: "aws_subnet.public",
					Action:  "create",
					Children: []*TreeNode{
						{Address: "aws_instance.web", Action: "update"},
					},
				},
				{Address: "aws_instance.old", Action: "delete"},
			},
		},
	}

	add, change, remove := tree.GetChangeSummary()

	assert.Equal(t, 2, add)    // aws_vpc.main, aws_subnet.public.
	assert.Equal(t, 1, change) // aws_instance.web.
	assert.Equal(t, 1, remove) // aws_instance.old.
}

func TestDependencyTree_GetChangeSummary_WithReplace(t *testing.T) {
	tree := &DependencyTree{
		Root: &TreeNode{
			Address: "root",
			Children: []*TreeNode{
				{Address: "aws_vpc.main", Action: "create"},
				{Address: "aws_instance.web", Action: "replace"}, // Replace counts as +1 add and +1 remove.
				{Address: "aws_instance.old", Action: "delete"},
			},
		},
	}

	add, change, remove := tree.GetChangeSummary()

	assert.Equal(t, 2, add)    // aws_vpc.main + aws_instance.web (replace).
	assert.Equal(t, 0, change) // No updates.
	assert.Equal(t, 2, remove) // aws_instance.old + aws_instance.web (replace).
}

func TestSortChildren(t *testing.T) {
	root := &TreeNode{
		Address: "root",
		Children: []*TreeNode{
			{Address: "z_resource"},
			{Address: "a_resource"},
			{Address: "m_resource"},
		},
	}

	sortChildren(root)

	assert.Equal(t, "a_resource", root.Children[0].Address)
	assert.Equal(t, "m_resource", root.Children[1].Address)
	assert.Equal(t, "z_resource", root.Children[2].Address)
}

func TestRenderChildren_Empty(t *testing.T) {
	var b strings.Builder
	// No styling in test for simplicity.
	renderChildren(&b, nil, nil, nil)

	assert.Empty(t, b.String())
}

func TestRenderChildren_SingleNode(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
	}

	renderChildren(&b, nodes, nil, nil)

	result := b.String()
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "└─") // Last (and only) child uses └─.
}

func TestRenderChildren_MultipleNodes(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
		{Address: "aws_security_group.default", Action: "update"},
	}

	renderChildren(&b, nodes, nil, nil)

	result := b.String()
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "aws_security_group.default")
	assert.Contains(t, result, "├─") // First child uses ├─
	assert.Contains(t, result, "└─") // Last child uses └─
}

// TestRenderChildren_AttributeChangesPreserveGutterForNonLastSibling is a regression test:
// an attribute-diff block rendered under a node that is NOT the last child at its level must
// carry the tree's "│" continuation bar on every line, so the gutter stays visually connected
// down to the next sibling's connector instead of breaking for the height of the diff block.
func TestRenderChildren_AttributeChangesPreserveGutterForNonLastSibling(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{
			Address: "null_resource.vpc",
			Action:  "create",
			Changes: []*AttributeChange{
				{Key: "cidr_block", Before: nil, After: "10.0.0.0/16"},
			},
		},
		{Address: "time_sleep.after_vpc", Action: "create"},
	}

	renderChildren(&b, nodes, nil, nil)

	var attrLine string
	for _, line := range strings.Split(b.String(), "\n") {
		if strings.Contains(line, "cidr_block") {
			attrLine = line
			break
		}
	}

	assert.NotEmpty(t, attrLine, "expected to find the cidr_block attribute line")
	assert.Contains(t, attrLine, "│",
		"attribute row under a non-last sibling must carry the continuation bar so the gutter connects to the next sibling")
}

// TestRenderChildren_AttributeRowsCarryRailToChildren is a regression test for the
// parent-to-child connection: when a node has both attribute changes and children, its
// attribute rows sit between the node's connector and its children's connectors, so those
// rows must carry a "│" at the children's column. Otherwise the first child's "├"/"└"
// floats below the diff block with nothing above it.
func TestRenderChildren_AttributeRowsCarryRailToChildren(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{
			Address: "time_sleep.after_vpc",
			Action:  "create",
			Changes: []*AttributeChange{
				{Key: "create_duration", Before: nil, After: "600ms"},
			},
			Children: []*TreeNode{
				{Address: "null_resource.subnet", Action: "create"},
			},
		},
	}

	renderChildren(&b, nodes, nil, nil)

	lines := strings.Split(ansi.Strip(b.String()), "\n")
	var attrLine, childLine string
	for _, line := range lines {
		switch {
		case strings.Contains(line, "create_duration"):
			attrLine = line
		case strings.Contains(line, "null_resource.subnet"):
			childLine = line
		}
	}
	assert.NotEmpty(t, attrLine)
	assert.NotEmpty(t, childLine)

	// Index by rune, not byte: the "●" symbol ahead of the connector is multi-byte.
	childCol := -1
	for i, r := range []rune(childLine) {
		if r == '├' || r == '└' {
			childCol = i
			break
		}
	}
	assert.Positive(t, childCol, "child row must have a connector")
	attrRunes := []rune(attrLine)
	assert.Less(t, childCol, len(attrRunes), "attribute row must reach the child column")
	assert.Equal(t, '│', attrRunes[childCol],
		"attribute row under a node with children must carry the rail at the child connector column")
}

// TestRenderChildren_AttributeRowsNoRailWithoutChildren is the negative case: a node with
// attribute changes but no children has nothing to connect to below its diff block, so
// its attribute rows must not draw a rail at the (would-be) child column.
func TestRenderChildren_AttributeRowsNoRailWithoutChildren(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{
			Address: "null_resource.leaf",
			Action:  "create",
			Changes: []*AttributeChange{
				{Key: "name", Before: nil, After: "x"},
			},
		},
	}

	renderChildren(&b, nodes, nil, nil)

	for _, line := range strings.Split(ansi.Strip(b.String()), "\n") {
		if strings.Contains(line, "name") {
			assert.NotContains(t, line, "│", "leaf node's attribute rows must not draw a child rail")
		}
	}
}

// TestRenderChildren_NonCompactSpacerCarriesRail verifies the spacer line between
// sibling resource blocks in non-compact mode keeps this level's rail, so the gutter
// doesn't break at the gap before the next sibling.
func TestRenderChildren_NonCompactSpacerCarriesRail(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
		{Address: "aws_security_group.default", Action: "update"},
	}

	renderChildren(&b, nodes, nil, &RenderConfig{Compact: false})

	lines := strings.Split(ansi.Strip(b.String()), "\n")
	// Line 0 is aws_vpc.main, line 1 is the spacer, line 2 is the next sibling.
	if assert.GreaterOrEqual(t, len(lines), 3) {
		assert.Equal(t, "     │", lines[1], "spacer must carry the sibling rail, not be blank")
	}
}

// TestRenderTree_IsConnected asserts the invariant the gutter package guarantees, against
// the real renderer: with attribute changes on every node of a deep, branching tree, every
// connector and rail has something above it in the same column (see uitree.Violations),
// in both compact and non-compact modes.
func TestRenderTree_IsConnected(t *testing.T) {
	change := []*AttributeChange{{Key: "k", Before: nil, After: "v"}, {Key: "k2", Before: "a", After: "b"}}
	tree := &DependencyTree{
		Stack: "dev", Component: "vpc",
		Root: &TreeNode{Address: "root", Children: []*TreeNode{
			{Address: "vpc", Action: "create", Changes: change, Children: []*TreeNode{
				{Address: "after_vpc", Action: "create", Changes: change, Children: []*TreeNode{
					{Address: "subnet_a", Action: "create", Changes: change, Children: []*TreeNode{
						{Address: "after_subnets", Action: "create", Changes: change, Children: []*TreeNode{
							{Address: "rta_a", Action: "create", Changes: change},
							{Address: "rta_b", Action: "create", Changes: change},
						}},
					}},
					{Address: "subnet_b", Action: "create", Changes: change},
				}},
			}},
			{Address: "sibling", Action: "update", Changes: change},
		}},
	}
	for _, compact := range []bool{true, false} {
		out := tree.RenderTreeWithConfig(&RenderConfig{Compact: compact})
		rows := strings.Split(strings.TrimRight(ansi.Strip(out), "\n"), "\n")
		assert.Empty(t, uitree.Violations(rows), "compact=%v:\n%s", compact, ansi.Strip(out))
	}
}

func TestExtractReferences(t *testing.T) {
	tests := []struct {
		name     string
		refs     []string
		prefix   string
		expected []string
	}{
		{
			name:     "simple resource reference",
			refs:     []string{"aws_vpc.main.id"},
			prefix:   "",
			expected: []string{"aws_vpc.main"},
		},
		{
			name:     "module-qualified reference with resource",
			refs:     []string{"module.vpc.aws_subnet.main.id"},
			prefix:   "",
			expected: []string{"module.vpc.aws_subnet.main"},
		},
		{
			name:     "module-qualified reference without attribute",
			refs:     []string{"module.vpc.aws_subnet.main"},
			prefix:   "",
			expected: []string{"module.vpc.aws_subnet.main"},
		},
		{
			name:     "simple module reference",
			refs:     []string{"module.vpc"},
			prefix:   "",
			expected: []string{"module.vpc"},
		},
		{
			name:     "resource with prefix",
			refs:     []string{"aws_instance.web.id"},
			prefix:   "module.app",
			expected: []string{"module.app.aws_instance.web"},
		},
		{
			name:     "filters var references",
			refs:     []string{"var.environment", "aws_vpc.main"},
			prefix:   "",
			expected: []string{"aws_vpc.main"},
		},
		{
			name:     "filters local references",
			refs:     []string{"local.config", "aws_vpc.main"},
			prefix:   "",
			expected: []string{"aws_vpc.main"},
		},
		{
			name:   "nested module reference",
			refs:   []string{"module.network.module.vpc.aws_subnet.main.id"},
			prefix: "",
			// Module path plus the trailing resource type/name, dropping only the attribute (.id).
			expected: []string{"module.network.module.vpc.aws_subnet.main"},
		},
		{
			name:     "nested module reference with no trailing resource",
			refs:     []string{"module.network.module.vpc"},
			prefix:   "",
			expected: []string{"module.network.module.vpc"}, // Module path only, unchanged.
		},
		{
			name:     "nested module reference ending exactly at the module keyword",
			refs:     []string{"module.a.module"}, // Malformed/edge case: no name follows the last "module".
			prefix:   "",
			expected: []string{"module.a.module"}, // Falls back to returning the reference unchanged.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock expression with the references.
			// tfjson.Expression embeds ExpressionData which contains References.
			expr := &tfjson.Expression{
				ExpressionData: &tfjson.ExpressionData{
					References: tt.refs,
				},
			}
			result := extractReferences(expr, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractReferences_NilExpression(t *testing.T) {
	result := extractReferences(nil, "")
	assert.Nil(t, result)
}

// Tests for renderMultilineDiffSimple - verifies line-by-line diff behavior.
func TestRenderMultilineDiffSimple_IdenticalLines(t *testing.T) {
	var b strings.Builder
	before := "line1\nline2\nline3"
	after := "line1\nline2\nline3"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	// Identical content should have no +/- markers.
	assert.NotContains(t, result, "+")
	assert.NotContains(t, result, "-")
	// All lines should be present.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line2")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiffSimple_SingleLineChange(t *testing.T) {
	var b strings.Builder
	before := "line1\nold-line\nline3"
	after := "line1\nnew-line\nline3"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	// Only the changed line should have markers.
	assert.Contains(t, result, "- old-line")
	assert.Contains(t, result, "+ new-line")
	// Unchanged lines should be present without markers.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiffSimple_ConsecutiveChangesGrouped(t *testing.T) {
	var b strings.Builder
	before := "unchanged\nold1\nold2\nold3\nfinal"
	after := "unchanged\nnew1\nnew2\nfinal"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()

	// Verify that - lines come before + lines for the same block of changes.
	// Find positions of the changed lines.
	old1Pos := strings.Index(result, "- old1")
	old2Pos := strings.Index(result, "- old2")
	old3Pos := strings.Index(result, "- old3")
	new1Pos := strings.Index(result, "+ new1")
	new2Pos := strings.Index(result, "+ new2")

	assert.Greater(t, old1Pos, -1, "- old1 should be present")
	assert.Greater(t, old2Pos, -1, "- old2 should be present")
	assert.Greater(t, old3Pos, -1, "- old3 should be present")
	assert.Greater(t, new1Pos, -1, "+ new1 should be present")
	assert.Greater(t, new2Pos, -1, "+ new2 should be present")

	// All - lines should come before all + lines (grouped, not interleaved).
	assert.Less(t, old1Pos, new1Pos, "- old1 should come before + new1")
	assert.Less(t, old2Pos, new1Pos, "- old2 should come before + new1")
	assert.Less(t, old3Pos, new1Pos, "- old3 should come before + new1")
	assert.Less(t, old1Pos, old2Pos, "- old1 should come before - old2")
	assert.Less(t, old2Pos, old3Pos, "- old2 should come before - old3")
	assert.Less(t, new1Pos, new2Pos, "+ new1 should come before + new2")
}

func TestRenderMultilineDiffSimple_MixedUnchangedAndChanged(t *testing.T) {
	var b strings.Builder
	before := "header\nold-section1\nmiddle\nold-section2\nfooter"
	after := "header\nnew-section1\nmiddle\nnew-section2\nfooter"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()

	// Unchanged lines should be present.
	assert.Contains(t, result, "header")
	assert.Contains(t, result, "middle")
	assert.Contains(t, result, "footer")

	// Changed lines should have markers.
	assert.Contains(t, result, "- old-section1")
	assert.Contains(t, result, "+ new-section1")
	assert.Contains(t, result, "- old-section2")
	assert.Contains(t, result, "+ new-section2")
}

func TestRenderMultilineDiffSimple_LinesAdded(t *testing.T) {
	var b strings.Builder
	before := "line1\nline3"
	after := "line1\nline2\nline3"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	// line2 is new, should have + marker.
	assert.Contains(t, result, "+ line2")
	// line1 and line3 are unchanged.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiffSimple_LinesDeleted(t *testing.T) {
	var b strings.Builder
	before := "line1\nline2\nline3"
	after := "line1\nline3"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	// line2 was removed, should have - marker.
	assert.Contains(t, result, "- line2")
	// line1 and line3 are present.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiffSimple_DifferentLengths(t *testing.T) {
	var b strings.Builder
	before := "a\nb"
	after := "a\nb\nc\nd\ne"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	// a and b are unchanged.
	assert.Contains(t, result, "a")
	assert.Contains(t, result, "b")
	// c, d, e are added.
	assert.Contains(t, result, "+ c")
	assert.Contains(t, result, "+ d")
	assert.Contains(t, result, "+ e")
}

func TestRenderMultilineDiffSimple_EmptyBefore(t *testing.T) {
	var b strings.Builder
	before := ""
	after := "new-line"
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	// All content is new.
	assert.Contains(t, result, "+ new-line")
}

func TestRenderMultilineDiffSimple_EmptyAfter(t *testing.T) {
	var b strings.Builder
	before := "old-line"
	after := ""
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	// All content is deleted.
	assert.Contains(t, result, "- old-line")
}

// Tests for attribute change rendering and color coding.
func TestRenderAttributeChanges_NewAttribute(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "new_attr", Before: nil, After: "value", Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "new_attr")
	// Should show the value (raw strings are shown without quotes in newVal).
	assert.Contains(t, result, "value")
}

func TestRenderAttributeChanges_DeletedAttribute(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "deleted_attr", Before: "old_value", After: nil, Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "deleted_attr")
	// Should show (none) for the after value.
	assert.Contains(t, result, "(none)")
}

func TestRenderAttributeChanges_UpdatedAttribute(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "updated_attr", Before: "old", After: "new", Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "updated_attr")
	// Should show old → new format.
	// Old value is formatted via formatSimpleValue (quoted), new value is raw string.
	assert.Contains(t, result, "\"old\"")
	assert.Contains(t, result, "new")
	assert.Contains(t, result, "→")
}

func TestRenderAttributeChanges_ComputedUnknown(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "computed_attr", Before: "old_hash", After: nil, Unknown: true},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "computed_attr")
	// Should show "(known after apply)" for unknown computed values.
	assert.Contains(t, result, "(known after apply)")
}

func TestRenderAttributeChanges_SensitiveValue(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "secret", Before: nil, After: "super-secret", Unknown: false, Sensitive: true},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "secret")
	// Should show "(sensitive)" instead of actual value.
	assert.Contains(t, result, "(sensitive)")
	// Should NOT show the actual secret.
	assert.NotContains(t, result, "super-secret")
}

func TestRenderAttributeChanges_MultipleAttributesAligned(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "short", Before: nil, After: "a", Unknown: false},
		{Key: "medium_key", Before: nil, After: "b", Unknown: false},
		{Key: "very_long_attribute_name", Before: nil, After: "c", Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// All attribute names should be present.
	assert.Contains(t, result, "short")
	assert.Contains(t, result, "medium_key")
	assert.Contains(t, result, "very_long_attribute_name")
}

func TestRenderAttributeChanges_BooleanValues(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "enabled", Before: false, After: true, Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	assert.Contains(t, result, "enabled")
	assert.Contains(t, result, "false")
	assert.Contains(t, result, "true")
}

func TestRenderAttributeChanges_NumericValues(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "count", Before: float64(5), After: float64(10), Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	assert.Contains(t, result, "count")
	assert.Contains(t, result, "5")
	assert.Contains(t, result, "10")
}

func TestRenderAttributeChanges_ForcesReplacement(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "content", Before: "old", After: "new", Unknown: false, ForcesReplacement: true},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "content")
	// Should show "# forces replacement" annotation.
	assert.Contains(t, result, "# forces replacement")
}

func TestRenderAttributeChanges_ForcesReplacementMultiline(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "content", Before: "line1\nline2", After: "line1\nline3", Unknown: false, ForcesReplacement: true},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "content")
	// Should show "# forces replacement" annotation on the key line.
	assert.Contains(t, result, "# forces replacement")
}

func TestRenderAttributeChanges_NoForcesReplacement(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "tags", Before: "old", After: "new", Unknown: false, ForcesReplacement: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	// Should contain the attribute name.
	assert.Contains(t, result, "tags")
	// Should NOT show "# forces replacement" annotation.
	assert.NotContains(t, result, "# forces replacement")
}

func TestExtractAttributeChanges_WithReplacePaths(t *testing.T) {
	// Create a mock ResourceChange with ReplacePaths.
	rc := &tfjson.ResourceChange{
		Address: "local_file.example",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate},
			Before: map[string]interface{}{
				"content":  "old content",
				"filename": "/tmp/test.txt",
			},
			After: map[string]interface{}{
				"content":  "new content",
				"filename": "/tmp/test.txt",
			},
			// ReplacePaths indicates "content" attribute forces replacement.
			ReplacePaths: []interface{}{
				[]interface{}{"content"},
			},
		},
	}

	changes := extractAttributeChanges(rc)

	// Should have one change (content changed, filename stayed the same).
	assert.Len(t, changes, 1)

	// The content attribute should be marked as forcing replacement.
	contentChange := changes[0]
	assert.Equal(t, "content", contentChange.Key)
	assert.True(t, contentChange.ForcesReplacement, "content should force replacement")
}

func TestExtractAttributeChanges_WithNestedReplacePaths(t *testing.T) {
	// Create a mock ResourceChange with nested ReplacePaths (e.g., list element).
	rc := &tfjson.ResourceChange{
		Address: "aws_instance.example",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionDelete, tfjson.ActionCreate},
			Before: map[string]interface{}{
				"ami":           "ami-old",
				"instance_type": "t2.micro",
			},
			After: map[string]interface{}{
				"ami":           "ami-new",
				"instance_type": "t2.micro",
			},
			// ReplacePaths with nested path: ["ami"] forces replacement.
			ReplacePaths: []interface{}{
				[]interface{}{"ami"},
			},
		},
	}

	changes := extractAttributeChanges(rc)

	// Should have one change (ami changed).
	assert.Len(t, changes, 1)

	// The ami attribute should be marked as forcing replacement.
	amiChange := changes[0]
	assert.Equal(t, "ami", amiChange.Key)
	assert.True(t, amiChange.ForcesReplacement, "ami should force replacement")
}

func TestExtractAttributeChanges_NoReplacePaths(t *testing.T) {
	// Create a mock ResourceChange without ReplacePaths (normal update).
	rc := &tfjson.ResourceChange{
		Address: "aws_instance.example",
		Change: &tfjson.Change{
			Actions: []tfjson.Action{tfjson.ActionUpdate},
			Before: map[string]interface{}{
				"tags": map[string]interface{}{"Name": "old"},
			},
			After: map[string]interface{}{
				"tags": map[string]interface{}{"Name": "new"},
			},
			// No ReplacePaths - this is an in-place update.
		},
	}

	changes := extractAttributeChanges(rc)

	// Should have one change (tags changed).
	assert.Len(t, changes, 1)

	// The tags attribute should NOT be marked as forcing replacement.
	tagsChange := changes[0]
	assert.Equal(t, "tags", tagsChange.Key)
	assert.False(t, tagsChange.ForcesReplacement, "tags should not force replacement")
}

// Tests for valuesEqual helper function.
func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        interface{}
		b        interface{}
		expected bool
	}{
		{"both nil", nil, nil, true},
		{"a nil b not nil", nil, "value", false},
		{"a not nil b nil", "value", nil, false},
		{"equal strings", "hello", "hello", true},
		{"different strings", "hello", "world", false},
		{"equal numbers", float64(42), float64(42), true},
		{"different numbers", float64(42), float64(43), false},
		{"equal bools", true, true, true},
		{"different bools", true, false, false},
		{"equal maps", map[string]interface{}{"a": "b"}, map[string]interface{}{"a": "b"}, true},
		{"different maps", map[string]interface{}{"a": "b"}, map[string]interface{}{"a": "c"}, false},
		{"equal slices", []interface{}{"a", "b"}, []interface{}{"a", "b"}, true},
		{"different slices", []interface{}{"a", "b"}, []interface{}{"a", "c"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := valuesEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Tests for formatSimpleValue helper function.
func TestFormatSimpleValue(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		sensitive bool
		expected  string
	}{
		{"nil value", nil, false, "(none)"},
		{"sensitive value", "secret", true, "(sensitive)"},
		{"string value", "hello", false, "\"hello\""},
		{"bool true", true, false, "true"},
		{"bool false", false, false, "false"},
		{"integer float", float64(42), false, "42"},
		{"decimal float", 3.14, false, "3.14"},
		{"simple map", map[string]interface{}{"key": "val"}, false, "{\"key\":\"val\"}"},
		{"simple slice", []interface{}{"a", "b"}, false, "[\"a\",\"b\"]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSimpleValue(tt.value, tt.sensitive)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSimpleValue_LongStringTruncation(t *testing.T) {
	longString := strings.Repeat("a", 100)
	result := formatSimpleValue(longString, false)

	// Should be truncated with "..." at the end.
	assert.Contains(t, result, "...")
	// Should be shorter than original.
	assert.Less(t, len(result), len(longString))
}

// Tests for getRawStringValue helper function.
func TestGetRawStringValue(t *testing.T) {
	tests := []struct {
		name          string
		value         interface{}
		sensitive     bool
		expectedStr   string
		expectedMulti bool
	}{
		{"nil value", nil, false, "(none)", false},
		{"sensitive value", "secret", true, "(sensitive)", false},
		{"single line string", "hello world", false, "hello world", false},
		{"multi line string", "line1\nline2\nline3", false, "line1\nline2\nline3", true},
		{"non-string value", 42, false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str, isMulti := getRawStringValue(tt.value, tt.sensitive)
			assert.Equal(t, tt.expectedStr, str)
			assert.Equal(t, tt.expectedMulti, isMulti)
		})
	}
}

// Tests for getContrastTextColor helper function.
func TestGetContrastTextColor(t *testing.T) {
	tests := []struct {
		name     string
		bgColor  string
		expected string
	}{
		{"dark background (black)", "#000000", "#FFFFFF"},
		{"light background (white)", "#FFFFFF", "#000000"},
		{"dark blue", "#0000FF", "#FFFFFF"},
		{"yellow (light)", "#FFFF00", "#000000"},
		{"green", "#00FF00", "#000000"},
		{"red", "#FF0000", "#FFFFFF"},
		{"gray mid", "#808080", "#FFFFFF"},
		{"without hash", "FF0000", "#FFFFFF"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getContrastTextColor(tt.bgColor)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetContrastTextColor_InvalidInput(t *testing.T) {
	// Invalid inputs should default to white.
	assert.Equal(t, "#FFFFFF", getContrastTextColor("invalid"))
	assert.Equal(t, "#FFFFFF", getContrastTextColor("#ZZZ"))
	assert.Equal(t, "#FFFFFF", getContrastTextColor(""))
}

func TestRenderChangeSummaryBadges_NoChanges(t *testing.T) {
	result := RenderChangeSummaryBadges(0, 0, 0)
	assert.Contains(t, result, "NO CHANGES")
	// Use patterns with numbers to avoid matching within "NO CHANGES".
	assert.NotRegexp(t, `\d+ ADD`, result)
	assert.NotRegexp(t, `\d+ CHANGE`, result)
	assert.NotRegexp(t, `\d+ DELETE`, result)
}

func TestRenderChangeSummaryBadges_OnlyAdd(t *testing.T) {
	result := RenderChangeSummaryBadges(3, 0, 0)
	assert.Contains(t, result, "3 ADD")
	assert.NotContains(t, result, "CHANGE")
	assert.NotContains(t, result, "DELETE")
	assert.NotContains(t, result, "NO CHANGES")
}

func TestRenderChangeSummaryBadges_OnlyChange(t *testing.T) {
	result := RenderChangeSummaryBadges(0, 2, 0)
	assert.Contains(t, result, "2 CHANGE")
	assert.NotContains(t, result, "ADD")
	assert.NotContains(t, result, "DELETE")
	assert.NotContains(t, result, "NO CHANGES")
}

func TestRenderChangeSummaryBadges_OnlyRemove(t *testing.T) {
	result := RenderChangeSummaryBadges(0, 0, 5)
	assert.Contains(t, result, "5 DELETE")
	assert.NotContains(t, result, "ADD")
	assert.NotContains(t, result, "CHANGE")
	assert.NotContains(t, result, "NO CHANGES")
}

func TestRenderChangeSummaryBadges_AllTypes(t *testing.T) {
	result := RenderChangeSummaryBadges(1, 2, 3)
	assert.Contains(t, result, "1 ADD")
	assert.Contains(t, result, "2 CHANGE")
	assert.Contains(t, result, "3 DELETE")
	assert.NotContains(t, result, "NO CHANGES")
}

func TestCountActions_Nil(t *testing.T) {
	var add, change, remove int
	countActions(nil, &add, &change, &remove)
	assert.Equal(t, 0, add)
	assert.Equal(t, 0, change)
	assert.Equal(t, 0, remove)
}

func TestCountActions_SingleNode(t *testing.T) {
	tests := []struct {
		name         string
		action       string
		expectAdd    int
		expectChange int
		expectRemove int
	}{
		{"create action", "create", 1, 0, 0},
		{"update action", "update", 0, 1, 0},
		{"delete action", "delete", 0, 0, 1},
		{"replace action", "replace", 1, 0, 1}, // Replace counts as add + remove.
		{"unknown action", "unknown", 0, 0, 0},
		{"empty action", "", 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var add, change, remove int
			node := &TreeNode{Address: "test", Action: tt.action}
			countActions(node, &add, &change, &remove)
			assert.Equal(t, tt.expectAdd, add)
			assert.Equal(t, tt.expectChange, change)
			assert.Equal(t, tt.expectRemove, remove)
		})
	}
}

// TestRenderChildren_CompactMode_NoBlankLines verifies Compact: true suppresses the blank
// line that would otherwise separate non-last resource blocks.
func TestRenderChildren_CompactMode_NoBlankLines(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
		{Address: "aws_security_group.default", Action: "update"},
	}

	renderChildren(&b, nodes, nil, &RenderConfig{Compact: true})

	result := b.String()
	assert.NotContains(t, result, "\n\n", "compact mode must not add blank lines between resources")
}

// TestRenderAttributeChanges_WithAttributeBar verifies ShowAttributeBar renders the "┃" bar
// alongside each attribute row.
func TestRenderAttributeChanges_WithAttributeBar(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "name", Before: "old", After: "new"},
	}

	renderAttributeChanges(&b, changes, "", &RenderConfig{ShowAttributeBar: true})

	result := b.String()
	assert.Contains(t, result, "┃")
	assert.Contains(t, result, "name")
}

// TestRenderAttributeChanges_MultilineOnlyAddition verifies a purely-added multi-line value
// (Before nil, After multi-line) renders every line with a "+" marker via
// renderMultilineValueSimple, including truncation of lines exceeding the max width.
func TestRenderAttributeChanges_MultilineOnlyAddition(t *testing.T) {
	var b strings.Builder
	longLine := strings.Repeat("a", 150)
	changes := []*AttributeChange{
		{Key: "script", Before: nil, After: "line1\n" + longLine, Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", &RenderConfig{ShowAttributeBar: true})

	// resolveRenderConfig falls back to styled (colored) Create/Delete symbols when
	// RenderConfig leaves them unset, which inserts an ANSI reset between the "+"/"-"
	// symbol and the line text; strip it so the marker+text substring checks below are
	// independent of whether the terminal the test runs under supports color.
	result := ansi.Strip(b.String())
	assert.Contains(t, result, "script")
	assert.Contains(t, result, "+ line1")
	assert.Contains(t, result, "...", "a line longer than the max width must be truncated")
	assert.NotContains(t, result, longLine, "the untruncated long line must not appear verbatim")
}

// TestRenderAttributeChanges_MultilineOnlyDeletion verifies a purely-removed multi-line value
// (Before multi-line, After nil) renders every line with a "-" marker.
func TestRenderAttributeChanges_MultilineOnlyDeletion(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "script", Before: "line1\nline2", After: nil, Unknown: false},
	}

	renderAttributeChanges(&b, changes, "", nil)

	// See TestRenderAttributeChanges_MultilineOnlyAddition: strip ANSI styling so the
	// marker+text substring checks don't depend on terminal color support.
	result := ansi.Strip(b.String())
	assert.Contains(t, result, "script")
	assert.Contains(t, result, "- line1")
	assert.Contains(t, result, "- line2")
}

// TestRenderAttributeChanges_ComplexValue_Create verifies a map/array attribute that's newly
// added (Before nil) is rendered as pretty-printed JSON lines, each prefixed with "+".
func TestRenderAttributeChanges_ComplexValue_Create(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "tags", Before: nil, After: map[string]interface{}{"Name": "main", "Env": "dev"}},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	assert.Contains(t, result, "tags")
	assert.Contains(t, result, "+")
	assert.Contains(t, result, "Name")
}

// TestRenderAttributeChanges_ComplexValue_Delete verifies a map/array attribute that's
// removed (After nil) is rendered as pretty-printed JSON lines, each prefixed with "-".
func TestRenderAttributeChanges_ComplexValue_Delete(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "tags", Before: map[string]interface{}{"Name": "main"}, After: nil},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	assert.Contains(t, result, "tags")
	assert.Contains(t, result, "-")
	assert.Contains(t, result, "Name")
}

// TestRenderAttributeChanges_ComplexValue_Update verifies a map/array attribute present on
// both sides is rendered as a line-by-line JSON diff (renderJSONDiff), also exercising the
// attribute-bar content-indent branch (ShowAttributeBar: true).
func TestRenderAttributeChanges_ComplexValue_Update(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{
			Key:    "tags",
			Before: map[string]interface{}{"Name": "old", "Shared": "same"},
			After:  map[string]interface{}{"Name": "new", "Shared": "same"},
		},
	}

	renderAttributeChanges(&b, changes, "", &RenderConfig{ShowAttributeBar: true})

	result := b.String()
	assert.Contains(t, result, "tags")
	assert.Contains(t, result, "old")
	assert.Contains(t, result, "new")
	// The unchanged "Shared" key/value should appear exactly once (matched line, no diff markers).
	assert.Equal(t, 1, strings.Count(result, "Shared"))
}

// TestRenderMultilineDiffSimple_LongLineTruncated verifies makeTruncator's truncation branch:
// a line exceeding the max content width is cut short and suffixed with "...".
func TestRenderMultilineDiffSimple_LongLineTruncated(t *testing.T) {
	var b strings.Builder
	longBefore := strings.Repeat("x", 150)
	longAfter := strings.Repeat("y", 150)
	before := "short\n" + longBefore
	after := "short\n" + longAfter
	createStyle := lipgloss.NewStyle()
	deleteStyle := lipgloss.NewStyle()

	renderMultilineDiffSimple(&b, before, after, "", &diffStyles{Create: createStyle, Delete: deleteStyle})

	result := b.String()
	assert.Contains(t, result, "...")
	assert.NotContains(t, result, longBefore, "the full untruncated deleted line should not appear")
	assert.NotContains(t, result, longAfter, "the full untruncated added line should not appear")
}

// TestMakeTruncator_RuneSafe verifies makeTruncator cuts on rune boundaries, not byte indices,
// so a multi-byte UTF-8 character (e.g. in a tag/description/template attribute value) is never
// split into an invalid partial sequence.
func TestMakeTruncator_RuneSafe(t *testing.T) {
	truncate := makeTruncator(10)

	// Each "é" is 2 bytes in UTF-8; a byte-index slice at width-3=7 would land mid-rune.
	line := strings.Repeat("é", 20)
	result := truncate(line)

	assert.True(t, strings.HasSuffix(result, "..."), "truncated line must end with the ellipsis")
	assert.True(t, utf8.ValidString(result), "truncated line must remain valid UTF-8, never split mid-rune")
	// 10-3=7 runes kept, plus "...".
	assert.Equal(t, strings.Repeat("é", 7)+"...", result)
}

// TestMakeTruncator_ShortLineUnchanged verifies lines at or under maxWidth pass through untouched.
func TestMakeTruncator_ShortLineUnchanged(t *testing.T) {
	truncate := makeTruncator(10)
	assert.Equal(t, "short", truncate("short"))
}

// TestTransformLines_NilTransform verifies transformLines returns the input slice unchanged
// when no transform function is supplied (the identity path used defensively but never
// actually reached by the current call site, which always passes a non-nil truncator).
func TestTransformLines_NilTransform(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := transformLines(lines, nil)
	assert.Equal(t, lines, result)
}

// TestTransformLines_WithTransform verifies transformLines applies the transform to every line.
func TestTransformLines_WithTransform(t *testing.T) {
	lines := []string{"a", "bb"}
	result := transformLines(lines, strings.ToUpper)
	assert.Equal(t, []string{"A", "BB"}, result)
}

// TestRenderChildren_WithAttributeChanges verifies renderChildren delegates to
// renderAttributeChanges when a node carries attribute-level changes (rather than only
// exercising renderAttributeChanges directly, bypassing the tree-rendering call site).
func TestRenderChildren_WithAttributeChanges(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{
			Address: "aws_instance.web",
			Action:  "update",
			Changes: []*AttributeChange{
				{Key: "instance_type", Before: "t2.micro", After: "t2.small"},
			},
		},
	}

	renderChildren(&b, nodes, nil, nil)

	result := b.String()
	assert.Contains(t, result, "aws_instance.web")
	assert.Contains(t, result, "instance_type")
}

// TestRenderAttributeChanges_MultilineUnknown verifies a multi-line Before value paired with
// Unknown: true (After computed at apply time) renders "(known after apply)" as the after side
// of the diff rather than leaving it blank.
func TestRenderAttributeChanges_MultilineUnknown(t *testing.T) {
	var b strings.Builder
	changes := []*AttributeChange{
		{Key: "cert", Before: "line1\nline2", After: nil, Unknown: true},
	}

	renderAttributeChanges(&b, changes, "", nil)

	result := b.String()
	assert.Contains(t, result, "cert")
	assert.Contains(t, result, "(known after apply)")
}

func TestCountActions_Recursive(t *testing.T) {
	root := &TreeNode{
		Address: "root",
		Action:  "", // Root typically has no action.
		Children: []*TreeNode{
			{Address: "a", Action: "create"},
			{
				Address: "b",
				Action:  "update",
				Children: []*TreeNode{
					{Address: "c", Action: "delete"},
					{Address: "d", Action: "replace"}, // +1 add, +1 remove.
				},
			},
			{Address: "e", Action: "create"},
		},
	}

	var add, change, remove int
	countActions(root, &add, &change, &remove)

	// create(a) + replace(d) + create(e) = 3 add.
	assert.Equal(t, 3, add)
	// update(b) = 1 change.
	assert.Equal(t, 1, change)
	// delete(c) + replace(d) = 2 remove.
	assert.Equal(t, 2, remove)
}
