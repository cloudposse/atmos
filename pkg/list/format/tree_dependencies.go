package format

import (
	"fmt"

	"github.com/charmbracelet/lipgloss/tree"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Branch labels used when both directions are rendered for a component.
const (
	dependsOnLabel  = "depends on ↓"
	requiredByLabel = "required by ↑"
)

// DepTreeNode is a render-friendly node in a dependency subtree. Circular marks
// a node whose expansion was stopped because it was already on the current path.
type DepTreeNode struct {
	Component string
	Stack     string
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
	root := buildDependenciesRootTree(entries)
	treeOutput := root.String()
	cleanedOutput := cleanupSpacerMarkers(treeOutput, []string{spacerMarker})

	return header + cleanedOutput + treeNewline
}

// buildDependenciesRootTree builds the root tree for all top-level entries.
func buildDependenciesRootTree(entries []*DepTreeEntry) *tree.Tree {
	root := tree.New().EnumeratorStyle(getBranchStyle())

	// Spacer at the top for visual separation from the header.
	root.Child(tree.New().Root(spacerMarker).EnumeratorStyle(getBranchStyle()))

	for i, entry := range entries {
		root.Child(buildDependencyEntryNode(entry))

		// Spacer between entries (but not after the last one).
		if i < len(entries)-1 {
			root.Child(tree.New().Root(spacerMarker).EnumeratorStyle(getBranchStyle()))
		}
	}

	return root
}

// buildDependencyEntryNode builds the tree node for one top-level component.
func buildDependencyEntryNode(entry *DepTreeEntry) *tree.Tree {
	node := tree.New().
		Root(styleComponentStack(entry.Component, entry.Stack)).
		EnumeratorStyle(getBranchStyle())

	if entry.ShowBoth {
		node.Child(buildDirectionBranch(dependsOnLabel, entry.DependsOn))
		node.Child(buildDirectionBranch(requiredByLabel, entry.RequiredBy))
		return node
	}

	// Single direction: attach children directly (no group label).
	children := entry.DependsOn
	if children == nil {
		children = entry.RequiredBy
	}
	if len(children) == 0 {
		node.Child(tree.New().Root(getBranchStyle().Render("(none)")).EnumeratorStyle(getBranchStyle()))
		return node
	}
	for _, child := range children {
		node.Child(buildDependencyChildNode(child))
	}
	return node
}

// buildDirectionBranch builds a labeled branch ("depends on" / "required by").
func buildDirectionBranch(label string, children []*DepTreeNode) *tree.Tree {
	branch := tree.New().
		Root(getImportStyle().Render(label)).
		EnumeratorStyle(getBranchStyle())

	if len(children) == 0 {
		branch.Child(tree.New().Root(getBranchStyle().Render("(none)")).EnumeratorStyle(getBranchStyle()))
		return branch
	}
	for _, child := range children {
		branch.Child(buildDependencyChildNode(child))
	}
	return branch
}

// buildDependencyChildNode recursively builds a dependency subtree node.
func buildDependencyChildNode(n *DepTreeNode) *tree.Tree {
	var label string
	if n.Circular {
		label = getCircularStyle().Render(fmt.Sprintf("%s [%s] (circular reference)", n.Component, n.Stack))
	} else {
		label = styleComponentStack(n.Component, n.Stack)
	}

	node := tree.New().Root(label).EnumeratorStyle(getBranchStyle())

	if !n.Circular {
		for _, child := range n.Children {
			node.Child(buildDependencyChildNode(child))
		}
	}
	return node
}

// styleComponentStack renders "component [stack]" with the component name styled
// and the stack name shown in the muted branch style.
func styleComponentStack(component, stack string) string {
	return getComponentStyle().Render(component) + " " + getBranchStyle().Render("["+stack+"]")
}
