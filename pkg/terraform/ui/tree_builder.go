package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// dotSeparator is the separator used to join address/reference path segments.
const dotSeparator = "."

// noOpAction represents a resource change with no effective action.
const noOpAction = "no-op"

// TreeBuildOptions groups the parameters needed to build a dependency tree from a planfile.
type TreeBuildOptions struct {
	PlanfilePath  string
	TerraformPath string
	WorkingDir    string
	Stack         string
	Component     string
}

// BuildDependencyTree parses a planfile and builds the dependency tree.
func BuildDependencyTree(ctx context.Context, opts *TreeBuildOptions) (*DependencyTree, error) {
	defer perf.Track(nil, "terraform.ui.BuildDependencyTree")()

	// Run terraform show -json planfile.
	terraformPath, planfilePath := opts.TerraformPath, opts.PlanfilePath
	cmd := exec.CommandContext(ctx, terraformPath, "show", "-json", planfilePath)
	cmd.Dir = opts.WorkingDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: terraform show: %w", errUtils.ErrCommandStart, err)
	}

	var plan tfjson.Plan
	if err := json.Unmarshal(output, &plan); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
	}

	return buildTreeFromPlan(&plan, opts.Stack, opts.Component), nil
}

// buildTreeFromPlan builds a dependency tree from an already-parsed plan. It never fails:
// any malformed resource change data is simply skipped or attached to the root.
func buildTreeFromPlan(plan *tfjson.Plan, stack, component string) *DependencyTree {
	tree := &DependencyTree{
		Root:      &TreeNode{Address: "root"},
		nodes:     make(map[string]*TreeNode),
		Stack:     stack,
		Component: component,
	}

	populateTreeNodes(tree, plan)

	// Build parent-child relationships from dependencies.
	if plan.Config != nil && plan.Config.RootModule != nil {
		buildRelationships(tree, plan)
	} else {
		// No config available, attach all nodes to root.
		attachAllToRoot(tree)
	}

	// Sort children at each level for consistent output.
	sortChildren(tree.Root)

	return tree
}

// populateTreeNodes creates tree nodes for all non-data, non-no-op resource changes in the plan.
func populateTreeNodes(tree *DependencyTree, plan *tfjson.Plan) {
	for _, rc := range plan.ResourceChanges {
		// Skip data sources and no-op changes.
		if rc.Mode == "data" {
			continue
		}

		action := resourceChangeAction(rc)
		if action == noOpAction {
			continue
		}

		node := &TreeNode{
			Address:  rc.Address,
			Action:   action,
			IsModule: isModuleAddress(rc.Address),
			Changes:  extractAttributeChanges(rc),
		}
		tree.nodes[rc.Address] = node
	}
}

// resourceChangeAction determines the action for a resource change, handling composite
// actions like replace (delete+create).
func resourceChangeAction(rc *tfjson.ResourceChange) string {
	switch {
	case len(rc.Change.Actions) == 2:
		// Composite action: Terraform can emit ["delete", "create"] or ["create", "delete"]
		// for replace operations. We represent this as "replace".
		return "replace"
	case len(rc.Change.Actions) > 0:
		return string(rc.Change.Actions[0])
	default:
		return noOpAction
	}
}

// isModuleAddress determines if this is a module node vs a resource within a module.
// A module node has address like "module.vpc", while a resource within a module
// has address like "module.vpc.aws_subnet.main" (contains a resource type/name after module path).
func isModuleAddress(addr string) bool {
	if !strings.HasPrefix(addr, "module"+dotSeparator) {
		return false
	}
	// Count the parts: module.name = 2 parts (is module), module.name.type.name = 4+ parts (is resource).
	// A pure module reference has exactly 2 parts: ["module", "name"].
	// Anything with more parts is a resource within a module.
	parts := strings.Split(addr, dotSeparator)
	return len(parts) == 2
}

// attachAllToRoot attaches every node directly to the tree root (used when no config is available).
func attachAllToRoot(tree *DependencyTree) {
	for _, node := range tree.nodes {
		node.Parent = tree.Root
		tree.Root.Children = append(tree.Root.Children, node)
	}
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
			if _, exists := tree.nodes[dep]; !exists {
				continue
			}
			hasParentInChangeSet = true
			// Find the first dependency that's in the change set and use it as parent.
			parentNode := tree.nodes[dep]
			node.Parent = parentNode
			parentNode.Children = append(parentNode.Children, node)
			attached[addr] = true
			break
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
			addr = prefix + dotSeparator + addr
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
		childPrefix := "module" + dotSeparator + name
		if prefix != "" {
			childPrefix = prefix + dotSeparator + childPrefix
		}
		if call.Module != nil {
			extractDependencies(call.Module, childPrefix, dependsOn)
		}
	}
}

// extractReferences extracts resource address references from a Terraform expression,
// filtering out variable/local references and normalizing module-qualified addresses.
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

		if strings.HasPrefix(ref, "module.") {
			ref = normalizeModuleReference(ref)
		} else {
			ref = normalizeResourceReference(ref, prefix)
		}
		refs = append(refs, ref)
	}
	return refs
}

// normalizeModuleReference collapses a module-qualified reference (e.g.
// module.vpc.aws_subnet.main.id) down to the granularity tracked by the tree:
// the module path for nested modules, or module.type.name for a single module.
func normalizeModuleReference(ref string) string {
	parts := strings.Split(ref, dotSeparator)

	// Count how many "module" keywords we have.
	// For nested modules (e.g., module.network.module.vpc.aws_subnet.main),
	// we want to extract only the module path (module.network.module.vpc),
	// not the resource within it.
	// But for single modules (module.vpc.aws_subnet.main), we want the full resource address.
	moduleCount := 0
	lastModuleIdx := -1
	for i, part := range parts {
		if part == "module" {
			moduleCount++
			lastModuleIdx = i
		}
	}

	if moduleCount > 1 {
		// Nested module - extract only up to the last module.name.
		if lastModuleIdx >= 0 && lastModuleIdx+1 < len(parts) {
			return strings.Join(parts[:lastModuleIdx+2], dotSeparator)
		}
		return ref
	}

	// Single module - extract module.name.resource_type.resource_name.
	// Minimum for a module reference: module.name (2 parts).
	// For a resource within a module: module.name.resource_type.resource_name (4+ parts).
	switch {
	case len(parts) >= 4:
		// Extract the module path and resource address.
		// e.g., module.vpc.aws_subnet.main.id -> module path is module.vpc,
		// resource is aws_subnet.main.
		return strings.Join(parts[:4], dotSeparator)
	case len(parts) >= 2:
		// Just a module reference (module.name) - keep as-is.
		return parts[0] + dotSeparator + parts[1]
	default:
		return ref
	}
}

// normalizeResourceReference strips the attribute path from a non-module reference,
// keeping only resource_type.name, and applies the module prefix if present.
func normalizeResourceReference(ref, prefix string) string {
	parts := strings.Split(ref, dotSeparator)
	if len(parts) >= 2 {
		// Keep resource_type.name format.
		ref = parts[0] + dotSeparator + parts[1]
	}
	// Add prefix for module context.
	if prefix != "" {
		ref = prefix + dotSeparator + ref
	}
	return ref
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

// extractAttributeChanges extracts attribute-level changes from a resource change.
func extractAttributeChanges(rc *tfjson.ResourceChange) []*AttributeChange {
	if rc.Change == nil {
		return nil
	}

	// Parse before/after as maps.
	beforeMap, _ := rc.Change.Before.(map[string]interface{})
	afterMap, _ := rc.Change.After.(map[string]interface{})
	unknownMap, _ := rc.Change.AfterUnknown.(map[string]interface{})
	sensitiveMap, _ := rc.Change.AfterSensitive.(map[string]interface{})

	forcesReplacement := extractForcesReplacement(rc.Change.ReplacePaths)
	sortedKeys := sortedAttributeKeys(beforeMap, afterMap)
	maps := attributeMaps{Before: beforeMap, After: afterMap, Unknown: unknownMap, Sensitive: sensitiveMap}

	var changes []*AttributeChange
	for _, key := range sortedKeys {
		if change := buildAttributeChange(key, maps, forcesReplacement); change != nil {
			changes = append(changes, change)
		}
	}

	return changes
}

// extractForcesReplacement builds the set of top-level attribute names that force resource
// replacement. ReplacePaths is a slice of paths, where each path is a slice of indexes
// (strings or ints); for top-level attributes, the path is a single-element slice
// containing the attribute name.
func extractForcesReplacement(replacePaths []interface{}) map[string]bool {
	forcesReplacement := make(map[string]bool)
	for _, path := range replacePaths {
		pathSlice, ok := path.([]interface{})
		if !ok || len(pathSlice) == 0 {
			continue
		}
		// The first element is the top-level attribute name.
		if attrName, ok := pathSlice[0].(string); ok {
			forcesReplacement[attrName] = true
		}
	}
	return forcesReplacement
}

// sortedAttributeKeys returns the union of keys from the before/after maps, sorted for
// consistent output.
func sortedAttributeKeys(beforeMap, afterMap map[string]interface{}) []string {
	allKeys := make(map[string]bool)
	for k := range beforeMap {
		allKeys[k] = true
	}
	for k := range afterMap {
		allKeys[k] = true
	}

	sortedKeys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	return sortedKeys
}

// attributeMaps bundles the before/after/unknown/sensitive maps parsed from a resource
// change, so per-key lookups only need to pass a single value around.
type attributeMaps struct {
	Before, After, Unknown, Sensitive map[string]interface{}
}

// buildAttributeChange compares a single attribute's before/after value and returns the
// AttributeChange if it changed, or nil if it's unchanged. Indexing a nil map returns the
// zero value in Go, so maps.Unknown/maps.Sensitive may be nil without a separate nil check.
func buildAttributeChange(key string, maps attributeMaps, forcesReplacement map[string]bool) *AttributeChange {
	beforeVal := maps.Before[key]
	afterVal := maps.After[key]

	unknown, _ := maps.Unknown[key].(bool)
	sensitive, _ := maps.Sensitive[key].(bool)

	// Only include if the value changed.
	if !valuesEqual(beforeVal, afterVal) || unknown {
		return &AttributeChange{
			Key:               key,
			Before:            beforeVal,
			After:             afterVal,
			Unknown:           unknown,
			Sensitive:         sensitive,
			ForcesReplacement: forcesReplacement[key],
		}
	}
	return nil
}
