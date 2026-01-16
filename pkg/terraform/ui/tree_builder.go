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

// BuildDependencyTree parses a planfile and builds the dependency tree.
func BuildDependencyTree(ctx context.Context, planfilePath, terraformPath, workingDir, stack, component string) (*DependencyTree, error) {
	defer perf.Track(nil, "terraform.ui.BuildDependencyTree")()

	// Run terraform show -json planfile.
	cmd := exec.CommandContext(ctx, terraformPath, "show", "-json", planfilePath)
	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: terraform show: %w", errUtils.ErrCommandStart, err)
	}

	var plan tfjson.Plan
	if err := json.Unmarshal(output, &plan); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
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

		// Determine if this is a module node vs a resource within a module.
		// A module node has address like "module.vpc", while a resource within a module
		// has address like "module.vpc.aws_subnet.main" (contains a resource type/name after module path).
		isModule := false
		if strings.HasPrefix(rc.Address, "module.") {
			// Check if this is just a module reference (module.name) vs a resource (module.name.type.name).
			// Count the parts: module.name = 2 parts (is module), module.name.type.name = 4+ parts (is resource).
			parts := strings.Split(rc.Address, ".")
			// A pure module reference has exactly 2 parts: ["module", "name"].
			// Anything with more parts is a resource within a module.
			isModule = len(parts) == 2
		}

		node := &TreeNode{
			Address:  rc.Address,
			Action:   action,
			IsModule: isModule,
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

			// Count how many "module" keywords we have.
			// For nested modules (e.g., module.network.module.vpc.aws_subnet.main),
			// we want to extract only the module path (module.network.module.vpc),
			// not the resource within it.
			// But for single modules (module.vpc.aws_subnet.main), we want the full resource address.
			moduleCount := 0
			lastModuleIdx := -1
			for i := 0; i < len(parts); i++ {
				if parts[i] == "module" {
					moduleCount++
					lastModuleIdx = i
				}
			}

			if moduleCount > 1 {
				// Nested module - extract only up to the last module.name.
				if lastModuleIdx >= 0 && lastModuleIdx+1 < len(parts) {
					ref = strings.Join(parts[:lastModuleIdx+2], ".")
				}
			} else {
				// Single module - extract module.name.resource_type.resource_name.
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
