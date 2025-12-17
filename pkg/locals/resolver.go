// Package locals provides resolution for file-scoped local variables in Atmos stack configurations.
// Locals enable users to define temporary variables that are available within a single file,
// similar to Terraform and Terragrunt locals.
//
// Key features:
// - File-scoped: locals do not inherit across file boundaries
// - Dependency resolution: locals can reference other locals with topological sorting
// - Cycle detection: circular dependencies are detected and reported clearly
// - Multi-scope: locals can be defined at global, component-type, and component levels
package locals

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	atmostmpl "github.com/cloudposse/atmos/pkg/template"
)

// Resolver handles dependency resolution and cycle detection for locals.
// It uses topological sorting to determine the order in which locals should be resolved.
type Resolver struct {
	locals       map[string]any      // Raw local definitions.
	resolved     map[string]any      // Resolved local values.
	dependencies map[string][]string // Dependency graph: local -> locals it depends on.
	filePath     string              // Source file path for error messages.
}

// NewResolver creates a resolver for a set of locals.
// The filePath is used for error message context.
func NewResolver(locals map[string]any, filePath string) *Resolver {
	defer perf.Track(nil, "locals.NewResolver")()

	return &Resolver{
		locals:       locals,
		resolved:     make(map[string]any),
		dependencies: make(map[string][]string),
		filePath:     filePath,
	}
}

// Resolve processes all locals in dependency order, returning resolved values.
// Parent locals (from outer scopes) are available during resolution.
// Returns error if circular dependency detected or undefined local referenced.
func (r *Resolver) Resolve(parentLocals map[string]any) (map[string]any, error) {
	defer perf.Track(nil, "locals.Resolver.Resolve")()

	// Handle nil/empty cases.
	if len(r.locals) == 0 {
		// Return copy of parent locals or empty map.
		if parentLocals == nil {
			return make(map[string]any), nil
		}
		result := make(map[string]any, len(parentLocals))
		for k, v := range parentLocals {
			result[k] = v
		}
		return result, nil
	}

	// Step 1: Build dependency graph.
	if err := r.buildDependencyGraph(); err != nil {
		return nil, err
	}

	// Step 2: Topological sort with cycle detection.
	order, err := r.topologicalSort()
	if err != nil {
		return nil, err
	}

	// Step 3: Start with parent locals (from outer scope).
	for k, v := range parentLocals {
		r.resolved[k] = v
	}

	// Step 4: Resolve in sorted order.
	for _, name := range order {
		value, err := r.resolveLocal(name)
		if err != nil {
			return nil, err
		}
		r.resolved[name] = value
	}

	return r.resolved, nil
}

// buildDependencyGraph extracts .locals.X references using the pkg/template AST utilities.
// This handles complex expressions like conditionals, pipes, range, and with blocks.
func (r *Resolver) buildDependencyGraph() error {
	defer perf.Track(nil, "locals.Resolver.buildDependencyGraph")()

	for name, value := range r.locals {
		deps, err := r.extractDependencies(value)
		if err != nil {
			return fmt.Errorf("%w %q in %s: %w", errUtils.ErrLocalsDependencyExtract, name, r.filePath, err)
		}
		r.dependencies[name] = deps
	}
	return nil
}

// extractDependencies extracts .locals.X references from a value.
// Handles string values with Go templates and recursively processes maps and slices.
func (r *Resolver) extractDependencies(value any) ([]string, error) {
	switch v := value.(type) {
	case string:
		return r.extractDepsFromString(v)
	case map[string]any:
		return r.extractDepsFromMap(v)
	case []any:
		return r.extractDepsFromSlice(v)
	default:
		// Non-string/map/slice values have no dependencies.
		return nil, nil
	}
}

// extractDepsFromString extracts .locals.X references from a string template.
func (r *Resolver) extractDepsFromString(str string) ([]string, error) {
	// Use pkg/template AST utilities to extract .locals.X references.
	// If it's not a valid template, we return no dependencies.
	// The actual template error will be caught during resolution.
	deps, _ := atmostmpl.ExtractFieldRefsByPrefix(str, "locals")
	return deps, nil
}

// extractDepsFromMap extracts dependencies from map values.
func (r *Resolver) extractDepsFromMap(m map[string]any) ([]string, error) {
	var allDeps []string
	seen := make(map[string]bool)
	for _, mapVal := range m {
		deps, err := r.extractDependencies(mapVal)
		if err != nil {
			return nil, err
		}
		allDeps = r.collectUniqueDeps(allDeps, deps, seen)
	}
	return allDeps, nil
}

// extractDepsFromSlice extracts dependencies from slice elements.
func (r *Resolver) extractDepsFromSlice(slice []any) ([]string, error) {
	var allDeps []string
	seen := make(map[string]bool)
	for _, elem := range slice {
		deps, err := r.extractDependencies(elem)
		if err != nil {
			return nil, err
		}
		allDeps = r.collectUniqueDeps(allDeps, deps, seen)
	}
	return allDeps, nil
}

// collectUniqueDeps adds new dependencies to the list, avoiding duplicates.
func (r *Resolver) collectUniqueDeps(allDeps, newDeps []string, seen map[string]bool) []string {
	for _, dep := range newDeps {
		if !seen[dep] {
			allDeps = append(allDeps, dep)
			seen[dep] = true
		}
	}
	return allDeps
}

// topologicalSort returns locals in resolution order, detecting cycles.
// Uses Kahn's algorithm for topological sorting.
func (r *Resolver) topologicalSort() ([]string, error) {
	// Calculate in-degree for each local (number of dependencies within this scope).
	inDegree := r.calculateInDegree()

	// Start with nodes that have no dependencies within this scope.
	queue := r.getZeroDegreeNodes(inDegree)

	// Process nodes in dependency order.
	result := r.processTopologicalOrder(queue, inDegree)

	// If not all nodes processed, there's a cycle.
	if len(result) != len(r.locals) {
		cycle := r.findCycle()
		return nil, fmt.Errorf("%w at %s\n\nDependency cycle detected:\n  %s",
			errUtils.ErrLocalsCircularDep, r.filePath, cycle)
	}

	return result, nil
}

// calculateInDegree computes the in-degree (number of dependencies) for each local.
func (r *Resolver) calculateInDegree() map[string]int {
	inDegree := make(map[string]int)
	for name, deps := range r.dependencies {
		count := 0
		for _, dep := range deps {
			// Only count dependencies within this scope.
			if _, exists := r.locals[dep]; exists {
				count++
			}
		}
		inDegree[name] = count
	}
	return inDegree
}

// getZeroDegreeNodes returns nodes with no dependencies (in-degree zero).
func (r *Resolver) getZeroDegreeNodes(inDegree map[string]int) []string {
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}
	sort.Strings(queue) // Deterministic order.
	return queue
}

// processTopologicalOrder processes nodes in topological order using Kahn's algorithm.
func (r *Resolver) processTopologicalOrder(queue []string, inDegree map[string]int) []string {
	var result []string
	for len(queue) > 0 {
		// Pop from queue.
		name := queue[0]
		queue = queue[1:]
		result = append(result, name)

		// Reduce in-degree of dependents.
		queue = r.updateDependents(name, queue, inDegree)
	}
	return result
}

// updateDependents reduces in-degree of nodes that depend on the given node.
func (r *Resolver) updateDependents(name string, queue []string, inDegree map[string]int) []string {
	// Collect all newly available nodes first, then sort once.
	var newlyAvailable []string
	for dependent, deps := range r.dependencies {
		for _, dep := range deps {
			if dep == name {
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					newlyAvailable = append(newlyAvailable, dependent)
				}
			}
		}
	}
	if len(newlyAvailable) > 0 {
		queue = append(queue, newlyAvailable...)
		sort.Strings(queue) // Sort once after all additions.
	}
	return queue
}

// findCycle uses DFS to find and return a cycle for error reporting.
func (r *Resolver) findCycle() string {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var cyclePath []string

	// Start DFS from each unvisited node.
	names := r.getSortedLocalNames()
	for _, name := range names {
		if !visited[name] {
			if r.dfsFindCycle(name, visited, recStack, &cyclePath) {
				break
			}
		}
	}

	return r.formatCyclePath(cyclePath)
}

// getSortedLocalNames returns sorted list of local names for deterministic processing.
func (r *Resolver) getSortedLocalNames() []string {
	names := make([]string, 0, len(r.locals))
	for name := range r.locals {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// dfsFindCycle performs depth-first search to find a cycle.
func (r *Resolver) dfsFindCycle(name string, visited, recStack map[string]bool, cyclePath *[]string) bool {
	visited[name] = true
	recStack[name] = true
	*cyclePath = append(*cyclePath, name)

	for _, dep := range r.dependencies[name] {
		if r.shouldSkipDependency(dep) {
			continue
		}
		if r.checkDependencyForCycle(dep, visited, recStack, cyclePath) {
			return true
		}
	}

	*cyclePath = (*cyclePath)[:len(*cyclePath)-1]
	recStack[name] = false
	return false
}

// shouldSkipDependency checks if a dependency should be skipped (not in current scope).
func (r *Resolver) shouldSkipDependency(dep string) bool {
	_, exists := r.locals[dep]
	return !exists
}

// checkDependencyForCycle checks if a dependency creates a cycle.
func (r *Resolver) checkDependencyForCycle(dep string, visited, recStack map[string]bool, cyclePath *[]string) bool {
	if !visited[dep] {
		return r.dfsFindCycle(dep, visited, recStack, cyclePath)
	}
	if recStack[dep] {
		// Found cycle - trim cyclePath to start at dep.
		r.trimCyclePathToStart(dep, cyclePath)
		return true
	}
	return false
}

// trimCyclePathToStart trims the cycle path to start at the given dependency.
func (r *Resolver) trimCyclePathToStart(dep string, cyclePath *[]string) {
	for i, n := range *cyclePath {
		if n == dep {
			*cyclePath = append((*cyclePath)[i:], dep)
			return
		}
	}
}

// formatCyclePath formats the cycle path as "a → b → c → a".
func (r *Resolver) formatCyclePath(cyclePath []string) string {
	var result strings.Builder
	for i, name := range cyclePath {
		if i > 0 {
			result.WriteString(" → ")
		}
		result.WriteString(name)
	}
	return result.String()
}

// resolveLocal resolves a single local's value using the template engine.
func (r *Resolver) resolveLocal(name string) (any, error) {
	defer perf.Track(nil, "locals.Resolver.resolveLocal")()

	value := r.locals[name]
	return r.resolveValue(value, name)
}

// resolveValue recursively resolves template expressions in a value.
func (r *Resolver) resolveValue(value any, localName string) (any, error) {
	switch v := value.(type) {
	case string:
		return r.resolveString(v, localName)

	case map[string]any:
		result := make(map[string]any, len(v))
		for key, val := range v {
			resolved, err := r.resolveValue(val, localName)
			if err != nil {
				return nil, err
			}
			result[key] = resolved
		}
		return result, nil

	case []any:
		result := make([]any, len(v))
		for i, elem := range v {
			resolved, err := r.resolveValue(elem, localName)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil

	default:
		// Non-string/map/slice values pass through unchanged.
		return value, nil
	}
}

// resolveString resolves template expressions in a string value.
func (r *Resolver) resolveString(strVal, localName string) (any, error) {
	// Quick check - if no template delimiters, return as-is.
	if !strings.Contains(strVal, "{{") {
		return strVal, nil
	}

	// Build template context with resolved locals.
	context := map[string]any{
		"locals": r.resolved,
	}

	// Parse and execute the template.
	tmpl, err := template.New(localName).Funcs(sprig.FuncMap()).Parse(strVal)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template for local %q in %s: %w", localName, r.filePath, err)
	}

	// Error on missing keys to catch undefined local references.
	tmpl.Option("missingkey=error")

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, context); err != nil {
		// Provide helpful error message with available locals.
		availableLocals := r.getAvailableLocals()
		return nil, fmt.Errorf("failed to resolve local %q in %s: %w\n\nAvailable locals at this scope:\n%s",
			localName, r.filePath, err, availableLocals)
	}

	return buf.String(), nil
}

// getAvailableLocals returns a formatted string of available locals for error messages.
func (r *Resolver) getAvailableLocals() string {
	if len(r.resolved) == 0 {
		return "  (none)"
	}

	names := make([]string, 0, len(r.resolved))
	for name := range r.resolved {
		names = append(names, name)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		sb.WriteString("  - ")
		sb.WriteString(name)
		sb.WriteString("\n")
	}
	return sb.String()
}

// GetDependencies returns the dependency graph for testing/debugging.
func (r *Resolver) GetDependencies() map[string][]string {
	defer perf.Track(nil, "locals.Resolver.GetDependencies")()

	result := make(map[string][]string, len(r.dependencies))
	for k, v := range r.dependencies {
		result[k] = append([]string{}, v...)
	}
	return result
}
