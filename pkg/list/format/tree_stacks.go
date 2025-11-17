package format

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"

	listtree "github.com/cloudposse/atmos/pkg/list/tree"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// RenderStacksTree renders stacks with their import hierarchies as a tree.
// If showImports is false, only stack names are shown without import details.
func RenderStacksTree(stacksWithImports map[string][]*listtree.ImportNode, showImports bool) string {
	if len(stacksWithImports) == 0 {
		return "No stacks found"
	}

	var output strings.Builder

	// Create h1 header style with solid background (matching auth list).
	h1Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Background(lipgloss.Color(theme.ColorBlue)).
		Bold(true).
		Padding(0, 1)

	// Title.
	output.WriteString(h1Style.Render("Stacks"))
	output.WriteString("\n")

	// Sort stack names for consistent output.
	var stackNames []string
	for stackName := range stacksWithImports {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	// Create unified tree structure with all stacks.
	root := tree.New().EnumeratorStyle(getBranchStyle())

	const spacerMarker = "<<<SPACER>>>"

	// Add spacer at the top (after header).
	topSpacer := tree.New().Root(spacerMarker).EnumeratorStyle(getBranchStyle())
	root.Child(topSpacer)

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

	// Render the tree.
	treeOutput := root.String()

	// Post-process to clean up spacer lines.
	// Spacer nodes render with the marker text, replace with just "│".
	lines := strings.Split(treeOutput, "\n")
	var cleaned []string
	for _, line := range lines {
		// Check if this line contains the spacer marker.
		// Strip ANSI codes first to get the plain text.
		plainLine := stripANSI(line)
		if strings.Contains(plainLine, spacerMarker) {
			// Replace the entire line with just the continuation character.
			// Find the indentation level (before the tree characters).
			indent := 0
			for _, ch := range plainLine {
				if ch == ' ' {
					indent++
				} else {
					break
				}
			}
			// Render just the vertical bar with proper styling.
			style := getBranchStyle()
			cleaned = append(cleaned, strings.Repeat(" ", indent)+style.Render("│"))
		} else {
			cleaned = append(cleaned, line)
		}
	}

	output.WriteString(strings.Join(cleaned, "\n"))
	output.WriteString("\n")

	return output.String()
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
