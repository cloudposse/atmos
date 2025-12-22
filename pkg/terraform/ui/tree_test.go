package ui

import (
	"strings"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/stretchr/testify/assert"
)

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
	renderChildren(&b, nil, "", defaultTreeStyle())

	assert.Empty(t, b.String())
}

func TestRenderChildren_SingleNode(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
	}

	renderChildren(&b, nodes, "", defaultTreeStyle())

	result := b.String()
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "└─") // Last (and only) child uses └─
}

func TestRenderChildren_MultipleNodes(t *testing.T) {
	var b strings.Builder
	nodes := []*TreeNode{
		{Address: "aws_vpc.main", Action: "create"},
		{Address: "aws_security_group.default", Action: "update"},
	}

	renderChildren(&b, nodes, "", defaultTreeStyle())

	result := b.String()
	assert.Contains(t, result, "aws_vpc.main")
	assert.Contains(t, result, "aws_security_group.default")
	assert.Contains(t, result, "├─") // First child uses ├─
	assert.Contains(t, result, "└─") // Last child uses └─
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
			name:     "nested module reference",
			refs:     []string{"module.network.module.vpc.aws_subnet.main"},
			prefix:   "",
			expected: []string{"module.network.module.vpc"}, // First module path is extracted.
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

// Tests for renderMultilineDiff - verifies line-by-line diff behavior.
func TestRenderMultilineDiff_IdenticalLines(t *testing.T) {
	var b strings.Builder
	before := "line1\nline2\nline3"
	after := "line1\nline2\nline3"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

	result := b.String()
	// Identical content should have no +/- markers.
	assert.NotContains(t, result, "+")
	assert.NotContains(t, result, "-")
	// All lines should be present.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line2")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiff_SingleLineChange(t *testing.T) {
	var b strings.Builder
	before := "line1\nold-line\nline3"
	after := "line1\nnew-line\nline3"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

	result := b.String()
	// Only the changed line should have markers.
	assert.Contains(t, result, "- old-line")
	assert.Contains(t, result, "+ new-line")
	// Unchanged lines should be present without markers.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiff_ConsecutiveChangesGrouped(t *testing.T) {
	var b strings.Builder
	before := "unchanged\nold1\nold2\nold3\nfinal"
	after := "unchanged\nnew1\nnew2\nfinal"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

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

func TestRenderMultilineDiff_MixedUnchangedAndChanged(t *testing.T) {
	var b strings.Builder
	before := "header\nold-section1\nmiddle\nold-section2\nfooter"
	after := "header\nnew-section1\nmiddle\nnew-section2\nfooter"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

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

func TestRenderMultilineDiff_LinesAdded(t *testing.T) {
	var b strings.Builder
	before := "line1\nline3"
	after := "line1\nline2\nline3"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

	result := b.String()
	// line2 is new, should have + marker.
	assert.Contains(t, result, "+ line2")
	// line1 and line3 are unchanged.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiff_LinesDeleted(t *testing.T) {
	var b strings.Builder
	before := "line1\nline2\nline3"
	after := "line1\nline3"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

	result := b.String()
	// line2 was removed, should have - marker.
	assert.Contains(t, result, "- line2")
	// line1 and line3 are present.
	assert.Contains(t, result, "line1")
	assert.Contains(t, result, "line3")
}

func TestRenderMultilineDiff_DifferentLengths(t *testing.T) {
	var b strings.Builder
	before := "a\nb"
	after := "a\nb\nc\nd\ne"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

	result := b.String()
	// a and b are unchanged.
	assert.Contains(t, result, "a")
	assert.Contains(t, result, "b")
	// c, d, e are added.
	assert.Contains(t, result, "+ c")
	assert.Contains(t, result, "+ d")
	assert.Contains(t, result, "+ e")
}

func TestRenderMultilineDiff_EmptyBefore(t *testing.T) {
	var b strings.Builder
	before := ""
	after := "new-line"

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

	result := b.String()
	// All content is new.
	assert.Contains(t, result, "+ new-line")
}

func TestRenderMultilineDiff_EmptyAfter(t *testing.T) {
	var b strings.Builder
	before := "old-line"
	after := ""

	renderMultilineDiff(&b, before, after, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

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

	renderAttributeChanges(&b, changes, "", false, defaultTreeStyle())

	result := b.String()
	assert.Contains(t, result, "count")
	assert.Contains(t, result, "5")
	assert.Contains(t, result, "10")
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
