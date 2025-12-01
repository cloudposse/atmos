package format

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/lipgloss/tree"

	listtree "github.com/cloudposse/atmos/pkg/list/tree"
)

// RenderStacksTree renders stacks with their import hierarchies as a tree.
// If showImports is false, only stack names are shown without import details.
func RenderStacksTree(stacksWithImports map[string][]*listtree.ImportNode, showImports bool) string {
	if len(stacksWithImports) == 0 {
		return "No stacks found"
	}

	header := renderTreeHeader("Stacks")
	root := buildStacksRootTree(stacksWithImports, showImports)
	treeOutput := root.String()
	cleanedOutput := cleanupSpacerMarkers(treeOutput, []string{spacerMarker})

	return header + cleanedOutput + treeNewline
}

// buildStacksRootTree builds the root tree structure for stacks.
func buildStacksRootTree(stacksWithImports map[string][]*listtree.ImportNode, showImports bool) *tree.Tree {
	root := tree.New().EnumeratorStyle(getBranchStyle())

	// Add spacer at the top.
	topSpacer := tree.New().Root(spacerMarker).EnumeratorStyle(getBranchStyle())
	root.Child(topSpacer)

	// Sort stack names for consistent output.
	stackNames := getSortedKeysFromImportsMap(stacksWithImports)

	// Build tree for each stack.
	for i, stackName := range stackNames {
		imports := stacksWithImports[stackName]
		var stackNode *tree.Tree
		if showImports {
			stackNode = buildStackNode(stackName, imports)
		} else {
			stackNode = buildStackNodeSimple(stackName)
		}
		root.Child(stackNode)

		// Add spacer between stacks (but not after the last one).
		if i < len(stackNames)-1 {
			spacer := tree.New().Root(spacerMarker).EnumeratorStyle(getBranchStyle())
			root.Child(spacer)
		}
	}

	return root
}

// getSortedKeysFromImportsMap extracts and sorts stack names from an imports map.
func getSortedKeysFromImportsMap(stacksWithImports map[string][]*listtree.ImportNode) []string {
	stackNames := make([]string, 0, len(stacksWithImports))
	for stackName := range stacksWithImports {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)
	return stackNames
}

// buildStackNodeSimple creates a tree node for a stack without imports.
func buildStackNodeSimple(stackName string) *tree.Tree {
	// Style the stack name.
	styledStackName := getStackStyle().Render(stackName)

	// Create the stack node without children.
	stackNode := tree.New().
		Root(styledStackName).
		EnumeratorStyle(getBranchStyle())

	return stackNode
}

// buildStackNode creates a tree node for a stack with its imports.
func buildStackNode(stackName string, imports []*listtree.ImportNode) *tree.Tree {
	// Style the stack name.
	styledStackName := getStackStyle().Render(stackName)

	// Create the stack node.
	stackNode := tree.New().
		Root(styledStackName).
		EnumeratorStyle(getBranchStyle())

	// Add import children.
	for _, imp := range imports {
		importNode := buildImportNode(imp)
		stackNode.Child(importNode)
	}

	return stackNode
}

// buildImportNode recursively creates a tree node for an import.
func buildImportNode(imp *listtree.ImportNode) *tree.Tree {
	var nodeText string

	// Check if this is a circular reference.
	if imp.Circular {
		nodeText = getCircularStyle().Render(fmt.Sprintf("%s (circular reference)", imp.Path))
	} else {
		nodeText = getImportStyle().Render(imp.Path)
	}

	// Create the import node.
	importNode := tree.New().
		Root(nodeText).
		EnumeratorStyle(getBranchStyle())

	// Recursively add children (unless circular).
	if !imp.Circular {
		for _, child := range imp.Children {
			childNode := buildImportNode(child)
			importNode.Child(childNode)
		}
	}

	return importNode
}
