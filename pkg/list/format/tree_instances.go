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

// RenderInstancesTree renders stacks with their components and import hierarchies as a tree.
// Structure: Stacks → Components → Imports.
// If showImports is false, only shows stacks and components without import details.
func RenderInstancesTree(stacksWithComponents map[string]map[string][]*listtree.ImportNode, showImports bool) string {
	if len(stacksWithComponents) == 0 {
		return "No stacks found"
	}

	var output strings.Builder

	// Create h1 header style with solid background (matching list stacks).
	h1Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Background(lipgloss.Color(theme.ColorBlue)).
		Bold(true).
		Padding(0, 1)

	// Title.
	output.WriteString(h1Style.Render("Component Instances"))
	output.WriteString("\n")

	// Create root tree.
	root := tree.New().EnumeratorStyle(getBranchStyle())

	// Sort stack names for consistent output.
	var stackNames []string
	for stackName := range stacksWithComponents {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)

	const spacerMarker = "<<<SPACER>>>"

	// Add spacer at the top (after header).
	topSpacer := tree.New().Root(spacerMarker).EnumeratorStyle(getBranchStyle())
	root.Child(topSpacer)

	// Build tree for each stack.
	for i, stackName := range stackNames {
		components := stacksWithComponents[stackName]
		stackNode := buildStackNodeWithComponents(stackName, components, showImports)
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
	const componentSpacerMarker = "<<<COMPONENT_SPACER>>>"

	lines := strings.Split(treeOutput, "\n")
	var cleaned []string
	for _, line := range lines {
		// Check if this line contains any spacer marker.
		// Strip ANSI codes first to get the plain text.
		plainLine := stripANSI(line)
		if strings.Contains(plainLine, spacerMarker) || strings.Contains(plainLine, componentSpacerMarker) {
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

// buildStackNodeWithComponents creates a tree node for a stack with its components.
func buildStackNodeWithComponents(stackName string, components map[string][]*listtree.ImportNode, showImports bool) *tree.Tree {
	// Style the stack name.
	styledStackName := getStackStyle().Render(stackName)

	// Create the stack node.
	stackNode := tree.New().
		Root(styledStackName).
		EnumeratorStyle(getBranchStyle())

	// Sort component names for consistent output.
	var componentNames []string
	for componentName := range components {
		componentNames = append(componentNames, componentName)
	}
	sort.Strings(componentNames)

	const componentSpacerMarker = "<<<COMPONENT_SPACER>>>"

	// Add component children with spacers between them.
	for i, componentName := range componentNames {
		imports := components[componentName]
		var componentNode *tree.Tree

		// Extract component folder from imports if available.
		var componentFolder string
		if len(imports) > 0 && imports[0].ComponentFolder != "" {
			componentFolder = imports[0].ComponentFolder
		}

		if showImports {
			componentNode = buildComponentNode(componentName, imports)
		} else {
			componentNode = buildComponentNodeSimple(componentName, componentFolder)
		}
		stackNode.Child(componentNode)

		// Add spacer between components only when showing imports (but not after the last one).
		if showImports && i < len(componentNames)-1 {
			spacer := tree.New().Root(componentSpacerMarker).EnumeratorStyle(getBranchStyle())
			stackNode.Child(spacer)
		}
	}

	return stackNode
}

// buildComponentNodeSimple creates a tree node for a component without imports.
func buildComponentNodeSimple(componentName string, componentFolder string) *tree.Tree {
	// Build display text: component_name (component_folder)
	displayText := componentName
	if componentFolder != "" {
		// Style component name and folder path separately.
		styledName := getComponentStyle().Render(componentName)
		// Use muted style for the folder path in parentheses.
		mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorDarkGray))
		styledFolder := mutedStyle.Render(fmt.Sprintf(" (%s)", componentFolder))
		displayText = styledName + styledFolder
	} else {
		displayText = getComponentStyle().Render(componentName)
	}

	// Create the component node without children.
	componentNode := tree.New().
		Root(displayText).
		EnumeratorStyle(getBranchStyle())

	return componentNode
}

// buildComponentNode creates a tree node for a component with its imports.
func buildComponentNode(componentName string, imports []*listtree.ImportNode) *tree.Tree {
	// Style the component name.
	styledComponentName := getComponentStyle().Render(componentName)

	// Create the component node.
	componentNode := tree.New().
		Root(styledComponentName).
		EnumeratorStyle(getBranchStyle())

	// Add import children.
	for _, imp := range imports {
		importNode := buildImportNode(imp)
		componentNode.Child(importNode)
	}

	return componentNode
}

// Note: getComponentStyle and buildImportNode are defined in tree_utils.go and tree_stacks.go respectively.
