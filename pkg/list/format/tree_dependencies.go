package format

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Branch labels used when both directions are rendered for a component.
const (
	dependsOnLabel  = "depends on ↓"
	requiredByLabel = "required by ↑"

	dependsOnGlyph  = "▶"
	requiredByGlyph = "◀"

	dependencyColumnMarker = "<<<DEPENDENCY_COLUMNS>>>"
	dependencyTypeMarker   = "<<<DEPENDENCY_TYPE>>>"
	dependencyColumnGap    = 2
)

// DepTreeNode is a render-friendly node in a dependency subtree. Circular marks
// a node whose expansion was stopped because it was already on the current path.
type DepTreeNode struct {
	Component string
	Stack     string
	Type      string
	Circular  bool
	Children  []*DepTreeNode
}

// DepTreeEntry is a top-level component in the dependency tree along with the
// direction subtrees to display. When only one of DependsOn/RequiredBy is
// populated, its children are rendered directly under the component (no group
// label). When both are populated, each is rendered under a labeled branch.
type DepTreeEntry struct {
	Component  string
	Stack      string
	Type       string
	DependsOn  []*DepTreeNode
	RequiredBy []*DepTreeNode
	ShowBoth   bool
}

// RenderDependenciesTree renders component dependencies as a tree.
func RenderDependenciesTree(entries []*DepTreeEntry) string {
	defer perf.Track(nil, "format.RenderDependenciesTree")()

	if len(entries) == 0 {
		return "No dependencies found"
	}

	header := renderTreeHeader("Dependencies")
	treeOutput := renderDependencyEntryTrees(entries)
	columnOutput := alignDependencyComponentColumn(treeOutput)

	return header + columnOutput + treeNewline
}

func renderDependencyEntryTrees(entries []*DepTreeEntry) string {
	rendered := make([]string, 0, len(entries))
	for _, entry := range entries {
		rendered = append(rendered, buildDependencyEntryNode(entry).String())
	}
	return strings.Join(rendered, treeNewline+treeNewline)
}

// buildDependencyEntryNode builds the tree node for one top-level component.
func buildDependencyEntryNode(entry *DepTreeEntry) *tree.Tree {
	node := tree.New().
		Root(styleDependencyRef(entry.Component, entry.Stack, entry.Type, "")).
		EnumeratorStyle(getBranchStyle())

	if entry.ShowBoth {
		node.Child(buildDirectionBranch(dependsOnLabel, entry.DependsOn, dependsOnGlyph, entry.Stack, entry.Type))
		node.Child(buildDirectionBranch(requiredByLabel, entry.RequiredBy, requiredByGlyph, entry.Stack, entry.Type))
		return node
	}

	// Single direction: attach children directly (no group label).
	// Use len == 0 so that an empty-but-initialized DependsOn slice also
	// falls back to RequiredBy, matching the behaviour of nil.
	children := entry.DependsOn
	glyph := dependsOnGlyph
	if len(children) == 0 {
		children = entry.RequiredBy
		glyph = requiredByGlyph
	}
	if len(children) == 0 {
		node.Child(tree.New().Root(getBranchStyle().Render("(none)")).EnumeratorStyle(getBranchStyle()))
		return node
	}
	for _, child := range children {
		node.Child(buildDependencyChildNode(child, glyph, entry.Stack, entry.Type))
	}
	return node
}

// buildDirectionBranch builds a labeled branch ("depends on" / "required by").
func buildDirectionBranch(label string, children []*DepTreeNode, glyph string, rootStack string, rootType string) *tree.Tree {
	branch := tree.New().
		Root(getImportStyle().Render(label)).
		EnumeratorStyle(getBranchStyle())

	if len(children) == 0 {
		branch.Child(tree.New().Root(getBranchStyle().Render("(none)")).EnumeratorStyle(getBranchStyle()))
		return branch
	}
	for _, child := range children {
		branch.Child(buildDependencyChildNode(child, glyph, rootStack, rootType))
	}
	return branch
}

// buildDependencyChildNode recursively builds a dependency subtree node.
func buildDependencyChildNode(n *DepTreeNode, glyph string, rootStack string, rootType string) *tree.Tree {
	var label string
	if n.Circular {
		label = styleCircularDependencyRef(n.Component, displayStack(n.Stack, rootStack), displayType(n.Type, rootType), glyph)
	} else {
		label = styleDependencyRef(n.Component, displayStack(n.Stack, rootStack), displayType(n.Type, rootType), glyph)
	}

	node := tree.New().Root(label).EnumeratorStyle(getBranchStyle())

	if !n.Circular {
		for _, child := range n.Children {
			node.Child(buildDependencyChildNode(child, glyph, rootStack, rootType))
		}
	}
	return node
}

func displayStack(stack, rootStack string) string {
	if stack == rootStack {
		return ""
	}
	return stack
}

func displayType(componentType, rootType string) string {
	if componentType == rootType {
		return ""
	}
	return componentType
}

// styleDependencyRef renders the component as the tree label and stores the
// stack and type after temporary markers. After tree rendering, the markers let
// us move metadata into separate context columns while keeping branch glyphs
// attached to the component names they describe. The optional glyph marks the
// direction of the edge that led to this node.
func styleDependencyRef(component, stack, componentType, glyph string) string {
	return styleDependencyComponent(component, glyph, getComponentStyle().Render) + dependencyColumnMarker + styleDependencyMetadata(stack, componentType)
}

func styleCircularDependencyRef(component, stack, componentType, glyph string) string {
	componentLabel := fmt.Sprintf("%s (circular reference)", component)
	return styleDependencyComponent(componentLabel, glyph, getCircularStyle().Render) + dependencyColumnMarker + styleDependencyMetadata(stack, componentType)
}

func styleDependencyMetadata(stack, componentType string) string {
	return getStackStyle().Render(stack) + dependencyTypeMarker + getBranchStyle().Render(componentType)
}

func styleDependencyComponent(component, glyph string, renderComponent func(...string) string) string {
	if glyph == "" {
		return renderComponent(component)
	}
	return getBranchStyle().Render(glyph) + " " + renderComponent(component)
}

func alignDependencyComponentColumn(output string) string {
	lines := strings.Split(output, treeNewline)
	columnWidths := dependencyColumnWidthsFor(lines)
	if columnWidths.stack == 0 {
		return output
	}

	aligned := make([]string, 0, len(lines)+1)
	aligned = append(aligned, dependencyColumnHeader(columnWidths))
	for _, line := range lines {
		columns := splitDependencyColumns(line)
		if columns.found {
			aligned = append(aligned, dependencyColumnLine(columns.stack, columns.componentType, columns.componentTree, columnWidths))
			continue
		}
		if line == "" {
			aligned = append(aligned, line)
			continue
		}
		aligned = append(aligned, dependencyColumnLine("", "", line, columnWidths))
	}

	return strings.Join(aligned, treeNewline)
}

type dependencyColumnWidths struct {
	stack         int
	componentTree int
	componentType int
}

type dependencyColumns struct {
	componentTree string
	stack         string
	componentType string
	found         bool
}

func dependencyColumnWidthsFor(lines []string) dependencyColumnWidths {
	widths := dependencyColumnWidths{
		stack:         lipgloss.Width("Stack"),
		componentTree: lipgloss.Width("Component"),
		componentType: lipgloss.Width("Type"),
	}
	for _, line := range lines {
		columns := splitDependencyColumns(line)
		if !columns.found {
			continue
		}
		if width := lipgloss.Width(columns.stack); width > widths.stack {
			widths.stack = width
		}
		if width := lipgloss.Width(columns.componentTree); width > widths.componentTree {
			widths.componentTree = width
		}
		if width := lipgloss.Width(columns.componentType); width > widths.componentType {
			widths.componentType = width
		}
	}
	widths.stack += dependencyColumnGap
	widths.componentTree += dependencyColumnGap
	widths.componentType += dependencyColumnGap
	return widths
}

func splitDependencyColumns(line string) dependencyColumns {
	componentTree, metadata, found := strings.Cut(line, dependencyColumnMarker)
	if !found {
		return dependencyColumns{}
	}
	stack, componentType, _ := strings.Cut(metadata, dependencyTypeMarker)
	return dependencyColumns{
		componentTree: componentTree,
		stack:         stack,
		componentType: componentType,
		found:         true,
	}
}

func dependencyColumnHeader(widths dependencyColumnWidths) string {
	return dependencyColumnLine(getStackStyle().Render("Stack"), getBranchStyle().Render("Type"), getComponentStyle().Render("Component"), widths)
}

func dependencyColumnLine(stack, componentType, componentTree string, widths dependencyColumnWidths) string {
	stackPadding := widths.stack - lipgloss.Width(stack)
	if stackPadding < dependencyColumnGap {
		stackPadding = dependencyColumnGap
	}
	line := stack + strings.Repeat(" ", stackPadding) + componentTree
	if componentType == "" {
		return line
	}
	typePadding := widths.componentTree - lipgloss.Width(componentTree)
	if typePadding < dependencyColumnGap {
		typePadding = dependencyColumnGap
	}
	return line + strings.Repeat(" ", typePadding) + componentType
}
