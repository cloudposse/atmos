package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tfjson "github.com/hashicorp/terraform-json"
	"golang.org/x/term"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

const (
	// defaultTerminalWidth is the fallback width when terminal size cannot be determined.
	defaultTerminalWidth = 120
	// treeIndentWidth is the approximate width of tree prefix and symbols.
	treeIndentWidth = 20
)

// DependencyTree represents the resource hierarchy.
type DependencyTree struct {
	Root      *TreeNode
	nodes     map[string]*TreeNode
	Stack     string // Atmos stack name (e.g., "plat-ue2-dev").
	Component string // Atmos component name (e.g., "vpc").
}

// TreeNode represents a resource in the dependency tree.
type TreeNode struct {
	Address  string // Full Terraform address (e.g., "aws_vpc.main").
	Action   string // create, update, delete, read, no-op.
	Children []*TreeNode
	Parent   *TreeNode
	IsModule bool               // True if this is a module node.
	Changes  []*AttributeChange // Attribute-level changes.
}

// AttributeChange represents a single attribute change.
type AttributeChange struct {
	Key              string      // Attribute name.
	Before           interface{} // Value before change (nil for create).
	After            interface{} // Value after change (nil for delete).
	Unknown          bool        // True if value is "(known after apply)".
	Sensitive        bool        // True if value is sensitive.
	ForcesReplacement bool       // True if this attribute forces resource replacement.
}

// BuildDependencyTree parses a planfile and builds the dependency tree.
func BuildDependencyTree(ctx context.Context, planfilePath, terraformPath, workingDir, stack, component string) (*DependencyTree, error) {
	// Run terraform show -json planfile.
	cmd := exec.CommandContext(ctx, terraformPath, "show", "-json", planfilePath)
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run terraform show: %w", err)
	}

	var plan tfjson.Plan
	if err := json.Unmarshal(output, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	return buildTreeFromPlan(&plan, stack, component)
}

func buildTreeFromPlan(plan *tfjson.Plan, stack, component string) (*DependencyTree, error) {
	tree := &DependencyTree{
		Root:      &TreeNode{Address: "root"},
		nodes:     make(map[string]*TreeNode),
		Stack:     stack,
		Component: component,
	}

	// Create nodes for all resource changes.
	for _, rc := range plan.ResourceChanges {
		// Skip data sources and no-op changes.
		if rc.Mode == "data" {
			continue
		}

		// Determine action, handling composite actions like replace (delete+create).
		action := "no-op"
		if len(rc.Change.Actions) == 2 {
			// Composite action: Terraform can emit ["delete", "create"] or ["create", "delete"]
			// for replace operations. We represent this as "replace".
			action = "replace"
		} else if len(rc.Change.Actions) > 0 {
			action = string(rc.Change.Actions[0])
		}

		// Skip no-op actions.
		if action == "no-op" {
			continue
		}

		node := &TreeNode{
			Address:  rc.Address,
			Action:   action,
			IsModule: strings.HasPrefix(rc.Address, "module."),
			Changes:  extractAttributeChanges(rc),
		}
		tree.nodes[rc.Address] = node
	}

	// Build parent-child relationships from dependencies.
	if plan.Config != nil && plan.Config.RootModule != nil {
		buildRelationships(tree, plan)
	} else {
		// No config available, attach all nodes to root.
		for _, node := range tree.nodes {
			node.Parent = tree.Root
			tree.Root.Children = append(tree.Root.Children, node)
		}
	}

	// Sort children at each level for consistent output.
	sortChildren(tree.Root)

	return tree, nil
}

func buildRelationships(tree *DependencyTree, plan *tfjson.Plan) {
	// Build a dependency map: resource -> resources it depends on.
	dependsOn := make(map[string][]string)

	// Extract dependencies from configuration.
	extractDependencies(plan.Config.RootModule, "", dependsOn)

	// Build reverse map: resource -> resources that depend on it.
	dependedBy := make(map[string][]string)
	for resource, deps := range dependsOn {
		for _, dep := range deps {
			dependedBy[dep] = append(dependedBy[dep], resource)
		}
	}

	// Find root resources (resources with no dependencies in our change set).
	attached := make(map[string]bool)
	for addr, node := range tree.nodes {
		deps := dependsOn[addr]
		hasParentInChangeSet := false
		for _, dep := range deps {
			if _, exists := tree.nodes[dep]; exists {
				hasParentInChangeSet = true
				// Find the first dependency that's in the change set and use it as parent.
				parentNode := tree.nodes[dep]
				node.Parent = parentNode
				parentNode.Children = append(parentNode.Children, node)
				attached[addr] = true
				break
			}
		}
		if !hasParentInChangeSet {
			// This is a root-level resource.
			node.Parent = tree.Root
			tree.Root.Children = append(tree.Root.Children, node)
			attached[addr] = true
		}
	}

	// Attach any remaining unattached nodes to root.
	for addr, node := range tree.nodes {
		if !attached[addr] {
			node.Parent = tree.Root
			tree.Root.Children = append(tree.Root.Children, node)
		}
	}
}

func extractDependencies(module *tfjson.ConfigModule, prefix string, dependsOn map[string][]string) {
	if module == nil {
		return
	}

	// Process resources in this module.
	for _, res := range module.Resources {
		addr := res.Address
		if prefix != "" {
			addr = prefix + "." + addr
		}

		var deps []string

		// Explicit depends_on.
		deps = append(deps, res.DependsOn...)

		// Implicit dependencies from expressions.
		for _, expr := range res.Expressions {
			deps = append(deps, extractReferences(expr, prefix)...)
		}

		if len(deps) > 0 {
			dependsOn[addr] = deps
		}
	}

	// Recursively process child modules.
	for name, call := range module.ModuleCalls {
		childPrefix := "module." + name
		if prefix != "" {
			childPrefix = prefix + "." + childPrefix
		}
		if call.Module != nil {
			extractDependencies(call.Module, childPrefix, dependsOn)
		}
	}
}

func extractReferences(expr *tfjson.Expression, prefix string) []string {
	if expr == nil {
		return nil
	}

	var refs []string
	for _, ref := range expr.References {
		// Filter out self-references and local values.
		if strings.HasPrefix(ref, "var.") || strings.HasPrefix(ref, "local.") {
			continue
		}

		// Handle module-qualified references (e.g., module.vpc.aws_subnet.main.id).
		if strings.HasPrefix(ref, "module.") {
			parts := strings.Split(ref, ".")
			// Minimum for a module reference: module.name (2 parts).
			// For a resource within a module: module.name.resource_type.resource_name (4+ parts).
			if len(parts) >= 4 {
				// Extract the module path and resource address.
				// e.g., module.vpc.aws_subnet.main.id -> module path is module.vpc,
				// resource is aws_subnet.main.
				modulePath := parts[0] + "." + parts[1]
				resourceType := parts[2]
				resourceName := parts[3]
				ref = modulePath + "." + resourceType + "." + resourceName
			} else if len(parts) >= 2 {
				// Just a module reference (module.name) - keep as-is.
				ref = parts[0] + "." + parts[1]
			}
		} else {
			// Non-module reference - normalize to resource address (remove attribute path).
			parts := strings.Split(ref, ".")
			if len(parts) >= 2 {
				// Keep resource_type.name format.
				ref = parts[0] + "." + parts[1]
			}
			// Add prefix for module context.
			if prefix != "" {
				ref = prefix + "." + ref
			}
		}
		refs = append(refs, ref)
	}
	return refs
}

func sortChildren(node *TreeNode) {
	if node == nil {
		return
	}

	// Sort children by address.
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Address < node.Children[j].Address
	})

	// Recursively sort grandchildren.
	for _, child := range node.Children {
		sortChildren(child)
	}
}

// RenderTree renders the tree as a string with box-drawing characters.
// Uses a two-column layout: action symbol (fixed width) | tree structure.
func (t *DependencyTree) RenderTree() string {
	var b strings.Builder

	// Styles.
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan)).Bold(true)
	treeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray)) // Dark gray for branches.

	// Render stack/component header (cyan, bold) - aligned with tree.
	b.WriteString(fmt.Sprintf("     %s\n", headerStyle.Render(t.Stack+"/"+t.Component)))

	// Render resource tree.
	renderChildren(&b, t.Root.Children, "", treeStyle)
	return b.String()
}

func renderChildren(b *strings.Builder, nodes []*TreeNode, prefix string, treeStyle lipgloss.Style) {
	for i, node := range nodes {
		isLastChild := i == len(nodes)-1

		// Determine box-drawing characters.
		var connector, childPrefix string
		if isLastChild {
			connector = "└── "
			childPrefix = prefix + "    "
		} else {
			connector = "├── "
			childPrefix = prefix + "│   "
		}

		// Colorized action symbol (fixed 2-char width: symbol + space).
		symbol := colorizedActionSymbol(node.Action)

		// Build tree line: "  +  ├── resource_name"
		// Column 1: 2 spaces + symbol + 2 spaces (5 chars total for alignment)
		// Column 2: tree prefix + connector + resource address
		treeLine := treeStyle.Render(prefix+connector) + node.Address

		b.WriteString(fmt.Sprintf("  %s  %s\n", symbol, treeLine))

		// Render attribute changes below the resource.
		if len(node.Changes) > 0 {
			renderAttributeChanges(b, node.Changes, childPrefix, len(node.Children) > 0 || !isLastChild, treeStyle)
		}

		// Render children.
		if len(node.Children) > 0 {
			renderChildren(b, node.Children, childPrefix, treeStyle)
		}
	}
}

// renderAttributeChanges renders attribute-level changes with two-column layout.
// For multi-line values, shows each line on its own row without arrow separator.
func renderAttributeChanges(b *strings.Builder, changes []*AttributeChange, prefix string, hasMoreContent bool, treeStyle lipgloss.Style) {
	// Styles for keys only (values are not colorized).
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))

	// Calculate max key width for alignment.
	maxKeyWidth := 0
	for _, change := range changes {
		if len(change.Key) > maxKeyWidth {
			maxKeyWidth = len(change.Key)
		}
	}

	// Pre-compute all formatted values for column width calculation.
	type formattedChange struct {
		change   *AttributeChange
		oldVal   string
		newVal   string
		isMulti  bool
		beforeML bool
		afterML  bool
	}
	formatted := make([]formattedChange, len(changes))

	maxOldValWidth := 0
	for i, change := range changes {
		_, beforeIsMultiline := getRawStringValue(change.Before, change.Sensitive)
		afterStr, afterIsMultiline := getRawStringValue(change.After, change.Sensitive)
		if change.Unknown {
			afterStr = "(known after apply)"
			afterIsMultiline = false
		}

		isMulti := beforeIsMultiline || afterIsMultiline

		var oldVal, newVal string
		if !isMulti {
			oldVal = formatSimpleValue(change.Before, change.Sensitive)
			newVal = afterStr
			if newVal == "" {
				newVal = formatSimpleValue(change.After, change.Sensitive)
			}
			if len(oldVal) > maxOldValWidth {
				maxOldValWidth = len(oldVal)
			}
		}

		formatted[i] = formattedChange{
			change:   change,
			oldVal:   oldVal,
			newVal:   newVal,
			isMulti:  isMulti,
			beforeML: beforeIsMultiline,
			afterML:  afterIsMultiline,
		}
	}

	for _, fc := range formatted {
		change := fc.change

		// Tree continuation line.
		// Only show │ if there are more sibling resources below (hasMoreContent).
		// Don't show │ just because there are more attributes - that creates
		// a visual "line to nowhere" under └── resources.
		var treeCont string
		if hasMoreContent {
			treeCont = treeStyle.Render(prefix + "│")
		} else {
			treeCont = treeStyle.Render(prefix + " ")
		}

		// Determine key style based on change type (color indicates change type).
		// No symbol on attribute lines - only color-coded keys.
		// - Green: new attribute (before=nil, after!=nil)
		// - Red: deleted attribute (before!=nil, after=nil, NOT unknown)
		// - Yellow: updated attribute (both have values, or unknown computed value)
		var keyStyle lipgloss.Style
		if change.Before == nil && change.After != nil {
			keyStyle = createStyle
		} else if change.Before != nil && change.After == nil && !change.Unknown {
			// Only show as delete if it's truly being removed (not a computed value).
			keyStyle = deleteStyle
		} else {
			// Updated value (including unknown/computed values).
			keyStyle = updateStyle
		}

		// Pad key for alignment.
		paddedKey := fmt.Sprintf("%-*s", maxKeyWidth, change.Key)

		// Build "# forces replacement" annotation if applicable.
		var forcesReplacementAnnotation string
		if change.ForcesReplacement {
			replaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange))
			forcesReplacementAnnotation = " " + replaceStyle.Render("# forces replacement")
		}

		// Check if we need multi-line rendering.
		if fc.isMulti {
			// Multi-line rendering: show key on first line, then each value line.
			// No arrow separator for multi-line values.
			// No symbol - only color-coded key.
			b.WriteString(fmt.Sprintf("      %s  %s%s\n",
				treeCont,
				keyStyle.Render(paddedKey),
				forcesReplacementAnnotation,
			))

			// Determine if tree line should show for multi-line content:
			// Only show │ if there are more sibling resources below (hasMoreContent).
			beforeStr, _ := getRawStringValue(change.Before, change.Sensitive)
			afterStr, _ := getRawStringValue(change.After, change.Sensitive)
			if change.Unknown {
				afterStr = "(known after apply)"
			}

			hasBeforeContent := beforeStr != "" && beforeStr != "(none)"
			hasAfterContent := afterStr != "" && afterStr != "(none)"

			// Render diff based on what content we have.
			if hasBeforeContent && hasAfterContent {
				// Both have content - do a proper line-by-line diff.
				// This shows only changed lines with -/+ markers.
				renderMultilineDiff(b, beforeStr, afterStr, prefix, hasMoreContent, treeStyle)
			} else if hasBeforeContent {
				// Only old content (deletion) - show all lines with -.
				renderMultilineValue(b, beforeStr, prefix, hasMoreContent, treeStyle, "-")
			} else if hasAfterContent {
				// Only new content (creation) - show all lines with +.
				renderMultilineValue(b, afterStr, prefix, hasMoreContent, treeStyle, "+")
			}
		} else {
			// Single-line rendering: old → new on same line with aligned columns.
			// Values are not colorized, only keys are (color indicates change type).
			// No symbol - only color-coded key.
			// Pad old value for consistent column alignment.
			paddedOldVal := fmt.Sprintf("%-*s", maxOldValWidth, fc.oldVal)

			b.WriteString(fmt.Sprintf("      %s  %s %s  %s  %s%s\n",
				treeCont,
				keyStyle.Render(paddedKey),
				dimStyle.Render(paddedOldVal),
				dimStyle.Render("→"),
				fc.newVal,
				forcesReplacementAnnotation,
			))
		}
	}
}

// getRawStringValue returns the raw string content and whether it's multi-line.
func getRawStringValue(v interface{}, sensitive bool) (string, bool) {
	if sensitive {
		return "(sensitive)", false
	}
	if v == nil {
		return "(none)", false
	}
	if s, ok := v.(string); ok {
		isMultiline := strings.Contains(s, "\n")
		return s, isMultiline
	}
	// Non-string values are not multi-line.
	return "", false
}

// formatSimpleValue formats a non-multi-line value for display.
func formatSimpleValue(v interface{}, sensitive bool) string {
	if sensitive {
		return "(sensitive)"
	}
	if v == nil {
		return "(none)"
	}
	switch val := v.(type) {
	case string:
		// Single-line string - truncate if needed.
		const maxWidth = 40
		if len(val) > maxWidth-2 {
			return fmt.Sprintf("\"%s...\"", val[:maxWidth-5])
		}
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case map[string]interface{}, []interface{}:
		const maxWidth = 40
		jsonBytes, err := json.Marshal(val)
		if err != nil {
			return "(complex)"
		}
		s := string(jsonBytes)
		if len(s) > maxWidth {
			return s[:maxWidth-3] + "..."
		}
		return s
	default:
		return fmt.Sprintf("%v", val)
	}
}

// renderMultilineDiff renders a line-by-line diff of two multi-line strings.
// Only lines that differ get -/+ markers; unchanged lines have no marker.
// Consecutive changed lines are grouped (all - lines, then all + lines) to match
// Terraform's native diff output style.
func renderMultilineDiff(b *strings.Builder, before, after string, prefix string, showTreeLine bool, treeStyle lipgloss.Style) {
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))

	// Get terminal width for smart truncation.
	maxLineWidth := getMaxLineWidth()

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	// Truncate long lines helper.
	truncateLine := func(line string) string {
		if len(line) > maxLineWidth {
			return line[:maxLineWidth-3] + "..."
		}
		return line
	}

	// Render a line with optional symbol.
	renderLine := func(line, symbol string, style lipgloss.Style) {
		var treeCont string
		if showTreeLine {
			treeCont = treeStyle.Render(prefix + "│")
		} else {
			treeCont = treeStyle.Render(prefix + " ")
		}
		if symbol == "" {
			// No marker for unchanged lines.
			b.WriteString(fmt.Sprintf("      %s    %s\n", treeCont, truncateLine(line)))
		} else {
			b.WriteString(fmt.Sprintf("      %s  %s %s\n", treeCont, style.Render(symbol), truncateLine(line)))
		}
	}

	i, j := 0, 0
	for i < len(beforeLines) || j < len(afterLines) {
		// Check if current lines match.
		if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
			// Lines are identical - no marker.
			renderLine(beforeLines[i], "", lipgloss.Style{})
			i++
			j++
		} else {
			// Lines differ - find the extent of consecutive differences.
			// Collect all consecutive differing lines, then output grouped.
			var deletedLines []string
			var addedLines []string

			// Scan ahead to find how many consecutive lines differ.
			// A line "differs" if it doesn't match or we're past one array's end.
			for i < len(beforeLines) || j < len(afterLines) {
				// Check if we're back to matching lines.
				if i < len(beforeLines) && j < len(afterLines) && beforeLines[i] == afterLines[j] {
					break // Found matching lines, stop collecting.
				}

				// Collect differing lines from both sides.
				if i < len(beforeLines) {
					deletedLines = append(deletedLines, beforeLines[i])
					i++
				}
				if j < len(afterLines) {
					addedLines = append(addedLines, afterLines[j])
					j++
				}
			}

			// Output all deleted lines first (grouped).
			for _, line := range deletedLines {
				renderLine(line, "-", deleteStyle)
			}
			// Then output all added lines (grouped).
			for _, line := range addedLines {
				renderLine(line, "+", createStyle)
			}
		}
	}
}

// renderMultilineValue renders each line of a multi-line string value with a symbol.
// Used when there's only before OR after content (not both for diffing).
func renderMultilineValue(b *strings.Builder, content, prefix string, showTreeLine bool, treeStyle lipgloss.Style, symbol string) {
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))

	// Choose symbol style based on +/-.
	var symbolStyle lipgloss.Style
	if symbol == "+" {
		symbolStyle = createStyle
	} else {
		symbolStyle = deleteStyle
	}

	// Get terminal width for smart truncation.
	maxLineWidth := getMaxLineWidth()

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		var treeCont string
		if showTreeLine {
			treeCont = treeStyle.Render(prefix + "│")
		} else {
			treeCont = treeStyle.Render(prefix + " ")
		}
		// Truncate long lines based on terminal width.
		if len(line) > maxLineWidth {
			line = line[:maxLineWidth-3] + "..."
		}
		// Output: tree continuation + symbol + line content.
		b.WriteString(fmt.Sprintf("      %s  %s %s\n",
			treeCont,
			symbolStyle.Render(symbol),
			line,
		))
	}
}

func colorizedActionSymbol(action string) string {
	createStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGreen))
	updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorYellow))
	deleteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorRed))
	readStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorCyan))
	replaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorOrange)) // Orange for replace (delete+create).

	// Use colored dots (●) for all actions with different colors:
	// - Green: create
	// - Yellow: update/change in place
	// - Red: delete
	// - Orange: replace/recreate
	// - Cyan: read/refresh
	switch action {
	case "create":
		return createStyle.Render(theme.IconActive)
	case "update":
		return updateStyle.Render(theme.IconActive)
	case "delete":
		return deleteStyle.Render(theme.IconActive)
	case "replace":
		return replaceStyle.Render(theme.IconActive)
	case "read":
		return readStyle.Render(theme.IconActive)
	case "no-op":
		return " "
	default:
		return " "
	}
}

// GetChangeSummary returns a summary of changes from the tree.
func (t *DependencyTree) GetChangeSummary() (add, change, remove int) {
	countActions(t.Root, &add, &change, &remove)
	return
}

func countActions(node *TreeNode, add, change, remove *int) {
	if node == nil {
		return
	}

	switch node.Action {
	case "create":
		*add++
	case "update":
		*change++
	case "delete":
		*remove++
	case "replace":
		// Replace counts as both add and remove since the resource is destroyed and recreated.
		*add++
		*remove++
	}

	for _, child := range node.Children {
		countActions(child, add, change, remove)
	}
}

// defaultTreeStyle returns the default tree branch style (for testing).
func defaultTreeStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.ColorGray))
}

// extractAttributeChanges extracts attribute-level changes from a resource change.
func extractAttributeChanges(rc *tfjson.ResourceChange) []*AttributeChange {
	if rc.Change == nil {
		return nil
	}

	var changes []*AttributeChange

	// Parse before/after as maps.
	beforeMap, _ := rc.Change.Before.(map[string]interface{})
	afterMap, _ := rc.Change.After.(map[string]interface{})
	unknownMap, _ := rc.Change.AfterUnknown.(map[string]interface{})
	sensitiveMap, _ := rc.Change.AfterSensitive.(map[string]interface{})

	// Build a set of attributes that force replacement.
	// ReplacePaths is a slice of paths, where each path is a slice of indexes (strings or ints).
	// For top-level attributes, the path is a single-element slice containing the attribute name.
	forcesReplacement := make(map[string]bool)
	for _, path := range rc.Change.ReplacePaths {
		// Each path is a slice of indexes.
		if pathSlice, ok := path.([]interface{}); ok && len(pathSlice) > 0 {
			// The first element is the top-level attribute name.
			if attrName, ok := pathSlice[0].(string); ok {
				forcesReplacement[attrName] = true
			}
		}
	}

	// Collect all keys from both maps.
	allKeys := make(map[string]bool)
	for k := range beforeMap {
		allKeys[k] = true
	}
	for k := range afterMap {
		allKeys[k] = true
	}

	// Sort keys for consistent output.
	var sortedKeys []string
	for k := range allKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	// Compare values for each key.
	for _, key := range sortedKeys {
		beforeVal := beforeMap[key]
		afterVal := afterMap[key]

		// Check if this value is unknown (computed).
		unknown := false
		if unknownMap != nil {
			if u, ok := unknownMap[key].(bool); ok {
				unknown = u
			}
		}

		// Check if this value is sensitive.
		sensitive := false
		if sensitiveMap != nil {
			if s, ok := sensitiveMap[key].(bool); ok {
				sensitive = s
			}
		}

		// Only include if the value changed.
		if !valuesEqual(beforeVal, afterVal) || unknown {
			changes = append(changes, &AttributeChange{
				Key:               key,
				Before:            beforeVal,
				After:             afterVal,
				Unknown:           unknown,
				Sensitive:         sensitive,
				ForcesReplacement: forcesReplacement[key],
			})
		}
	}

	return changes
}

// valuesEqual compares two interface values for equality.
func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Use JSON encoding for deep comparison of complex types.
	aJSON, errA := json.Marshal(a)
	bJSON, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(aJSON) == string(bJSON)
}

// getMaxLineWidth returns the maximum width for content lines based on terminal width.
func getMaxLineWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		width = defaultTerminalWidth
	}
	// Subtract space for tree indent, symbols, and some margin.
	maxWidth := width - treeIndentWidth
	if maxWidth < 40 {
		maxWidth = 40 // Minimum reasonable width.
	}
	return maxWidth
}

// RenderChangeSummaryBadges renders a badge-style change summary.
// Shows "NO CHANGES" badge if all counts are zero.
// Format: "  1 ADD 2 CHANGE 1 DELETE" with colored badges (green/yellow/red backgrounds).
func RenderChangeSummaryBadges(add, change, remove int) string {
	var badges []string

	// If no changes, show a "NO CHANGES" badge.
	if add == 0 && change == 0 && remove == 0 {
		noChangesBadge := lipgloss.NewStyle().
			Background(lipgloss.Color(theme.ColorDarkGray)).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1).
			Render("NO CHANGES")
		badges = append(badges, noChangesBadge)
	} else {
		// Badge styles with background colors and contrasting text.
		if add > 0 {
			addBadge := lipgloss.NewStyle().
				Background(lipgloss.Color(theme.ColorGreen)).
				Foreground(lipgloss.Color(getContrastTextColor(theme.ColorGreen))).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("%d ADD", add))
			badges = append(badges, addBadge)
		}

		if change > 0 {
			changeBadge := lipgloss.NewStyle().
				Background(lipgloss.Color(theme.ColorYellow)).
				Foreground(lipgloss.Color(getContrastTextColor(theme.ColorYellow))).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("%d CHANGE", change))
			badges = append(badges, changeBadge)
		}

		if remove > 0 {
			removeBadge := lipgloss.NewStyle().
				Background(lipgloss.Color(theme.ColorRed)).
				Foreground(lipgloss.Color(getContrastTextColor(theme.ColorRed))).
				Bold(true).
				Padding(0, 1).
				Render(fmt.Sprintf("%d DELETE", remove))
			badges = append(badges, removeBadge)
		}
	}

	// Join badges with a space, add blank line above and below, and indent 2 spaces.
	return "\n  " + strings.Join(badges, " ") + "\n\n"
}

// getContrastTextColor returns black or white text color based on background luminance.
// Uses WCAG relative luminance formula for accessibility.
func getContrastTextColor(bgColor string) string {
	// Parse hex color (handles both #RRGGBB and RRGGBB formats).
	hexColor := bgColor
	if len(hexColor) > 0 && hexColor[0] == '#' {
		hexColor = hexColor[1:]
	}

	// Default to white if parsing fails.
	if len(hexColor) != 6 {
		return "#FFFFFF"
	}

	// Parse RGB components.
	r, err1 := parseHexComponent(hexColor[0:2])
	g, err2 := parseHexComponent(hexColor[2:4])
	b, err3 := parseHexComponent(hexColor[4:6])

	if err1 != nil || err2 != nil || err3 != nil {
		return "#FFFFFF" // Default to white on parse error.
	}

	// Convert to 0-1 range and apply gamma correction.
	toLinear := func(c int64) float64 {
		v := float64(c) / 255.0
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}

	rLinear := toLinear(r)
	gLinear := toLinear(g)
	bLinear := toLinear(b)

	// Calculate relative luminance (WCAG formula).
	luminance := 0.2126*rLinear + 0.7152*gLinear + 0.0722*bLinear

	// Use black text for light backgrounds (luminance > 0.5), white for dark backgrounds.
	if luminance > 0.5 {
		return "#000000" // Black text for light backgrounds.
	}
	return "#FFFFFF" // White text for dark backgrounds.
}

// parseHexComponent parses a 2-character hex string to int64.
func parseHexComponent(hex string) (int64, error) {
	var result int64
	for _, c := range hex {
		result *= 16
		switch {
		case c >= '0' && c <= '9':
			result += int64(c - '0')
		case c >= 'a' && c <= 'f':
			result += int64(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			result += int64(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("%w: invalid hex character: %c", errUtils.ErrParseHexColor, c)
		}
	}
	return result, nil
}
