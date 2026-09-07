package format

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"

	listtree "github.com/cloudposse/atmos/pkg/list/tree"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	uitree "github.com/cloudposse/atmos/pkg/ui/tree"
)

const (
	treeNewline = "\n"
)

const (
	spacerMarker          = "<<<SPACER>>>"
	componentSpacerMarker = "<<<COMPONENT_SPACER>>>"
)

// RenderInstancesTree renders stacks with their components and import hierarchies as a tree.
// Structure: Stacks → Components → Imports.
// If showImports is false, only shows stacks and components without import details.
func RenderInstancesTree(stacksWithComponents map[string]map[string][]*listtree.ImportNode, showImports bool) string {
	defer perf.Track(nil, "format.RenderInstancesTree")()

	if len(stacksWithComponents) == 0 {
		return "No stacks found"
	}

	header := renderTreeHeader("Component Instances")
	root := buildInstancesRootTree(stacksWithComponents, showImports)
	treeOutput := root.String()
	cleanedOutput := cleanupSpacerMarkers(treeOutput, []string{spacerMarker, componentSpacerMarker})

	// Keep one trailing newline here; callers use data.Writeln, which adds a
	// second newline and leaves a deliberate blank line before the shell prompt.
	return header + strings.TrimRight(cleanedOutput, treeNewline) + treeNewline
}

// renderTreeHeader creates and renders a styled header for tree output.
func renderTreeHeader(title string) string {
	h1Style := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.ColorWhite)).
		Background(lipgloss.Color(theme.ColorBlue)).
		Bold(true).
		Padding(0, 1)

	// Lipgloss padding is useful in a terminal, but its right-hand space leaks
	// into plain-text output and makes snapshots and pipelines carry trailing
	// whitespace. Keep the visual left padding while trimming the data-channel
	// suffix.
	return strings.TrimRight(h1Style.Render(title), " ") + treeNewline
}

// buildInstancesRootTree builds the root tree structure for instances.
func buildInstancesRootTree(stacksWithComponents map[string]map[string][]*listtree.ImportNode, showImports bool) *tree.Tree {
	root := tree.New().EnumeratorStyle(getBranchStyle())

	// Add spacer at the top.
	topSpacer := tree.New().Root(spacerMarker).EnumeratorStyle(getBranchStyle())
	root.Child(topSpacer)

	// Sort stack names for consistent output.
	stackNames := getSortedStackNames(stacksWithComponents)

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

	return root
}

// getSortedStackNames extracts and sorts stack names from a map.
func getSortedStackNames(stacksWithComponents map[string]map[string][]*listtree.ImportNode) []string {
	stackNames := make([]string, 0, len(stacksWithComponents))
	for stackName := range stacksWithComponents {
		stackNames = append(stackNames, stackName)
	}
	sort.Strings(stackNames)
	return stackNames
}

// cleanupSpacerMarkers replaces the spacer placeholder rows lipgloss rendered with the
// spacer rows that belong there. Spacers are added to the tree as marker-titled child
// nodes so lipgloss positions them; pkg/ui/tree then derives each spacer's gutter from
// the rendered row, keeping every ancestor rail and turning the node's own connector into
// a rail, so nested spacers stay connected to the rows around them (an earlier version
// counted leading spaces and emitted a single bar, which dropped the ancestor rails of any
// nested spacer).
func cleanupSpacerMarkers(treeOutput string, markers []string) string {
	lines := strings.Split(treeOutput, treeNewline)
	cleaned := make([]string, 0, len(lines))
	style := getBranchStyle()

	for _, line := range lines {
		plainLine := stripANSI(line)

		if !containsAnyMarker(plainLine, markers) {
			cleaned = append(cleaned, line)
			continue
		}

		cleaned = append(cleaned, style.Render(uitree.SpacerFromConnectorRow(plainLine)))
	}

	return strings.Join(cleaned, treeNewline)
}

// containsAnyMarker checks if a string contains any of the given markers.
func containsAnyMarker(s string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
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
	// Build display text: component_name (component_folder).
	var displayText string
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
