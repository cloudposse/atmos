package dependency

import (
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Filter creates a new graph containing only the specified nodes and their relationships.
func (g *Graph) Filter(filter Filter) *Graph {
	defer perf.Track(nil, "dependency.Graph.Filter")()

	filtered := NewGraph()
	toInclude := g.collectNodesToInclude(filter)
	g.copyNodesToFilteredGraph(filtered, toInclude)
	filtered.IdentifyRoots()
	return filtered
}

// collectNodesToInclude determines which nodes should be included based on the filter.
func (g *Graph) collectNodesToInclude(filter Filter) map[string]bool {
	toInclude := make(map[string]bool)

	seeds := make([]string, 0, len(filter.NodeIDs))
	for _, id := range filter.NodeIDs {
		if _, exists := g.Nodes[id]; !exists {
			continue
		}
		toInclude[id] = true
		seeds = append(seeds, id)
	}

	if filter.IncludeDependencies {
		g.markReachable(seeds, toInclude, filter.DependencyDepth, dependencyEdges)
	}
	if filter.IncludeDependents {
		g.markReachable(seeds, toInclude, filter.DependentDepth, dependentEdges)
	}

	return toInclude
}

// copyNodesToFilteredGraph copies the included nodes to the filtered graph.
func (g *Graph) copyNodesToFilteredGraph(filtered *Graph, toInclude map[string]bool) {
	for id := range toInclude {
		node, exists := g.Nodes[id]
		if !exists {
			continue
		}

		newNode := g.cloneNodeForFilter(node, toInclude)
		filtered.Nodes[id] = newNode
	}
}

// cloneNodeForFilter creates a copy of a node with filtered relationships.
func (g *Graph) cloneNodeForFilter(node *Node, toInclude map[string]bool) *Node {
	return cloneNodeWithFilteredEdges(node, toInclude)
}

// FilterSelection creates a new graph from a selector-aware selection:
//
//	result = (Seeds + dependencies(Seeds)) + {n in dependents(DependentSeeds) : KeepDependent(n)}
//
// Dependents that fail KeepDependent (and DependentSeeds that are not in Seeds)
// are dropped, but edges through them are contracted so that execution order
// between the surviving nodes is preserved. With a nil KeepDependent and nil
// DependentSeeds the result equals Filter.
func (g *Graph) FilterSelection(f *SelectionFilter) *Graph {
	defer perf.Track(nil, "dependency.Graph.FilterSelection")()

	include, dropped := g.collectSelection(f)

	filtered := NewGraph()
	g.copyNodesToFilteredGraph(filtered, include)
	g.contractDroppedEdges(filtered, include, dropped)
	filtered.IdentifyRoots()
	return filtered
}

// collectSelection returns the included node set and the dropped intermediate set.
func (g *Graph) collectSelection(f *SelectionFilter) (map[string]bool, map[string]bool) {
	include := make(map[string]bool)
	dropped := make(map[string]bool)

	seeds := g.existingNodeIDs(f.Seeds)
	for _, id := range seeds {
		include[id] = true
	}
	if f.IncludeDependencies {
		g.markReachable(seeds, include, f.DependencyDepth, dependencyEdges)
	}
	if f.IncludeDependents {
		g.collectSelectionDependents(f, seeds, include, dropped)
	}
	return include, dropped
}

// collectSelectionDependents walks dependents from the dependent seeds, adding the
// ones that pass KeepDependent to include and recording the rest in dropped.
func (g *Graph) collectSelectionDependents(f *SelectionFilter, seeds []string, include, dropped map[string]bool) {
	dependentSeeds := seeds
	if f.DependentSeeds != nil {
		dependentSeeds = g.existingNodeIDs(f.DependentSeeds)
	}
	reached := make(map[string]bool)
	g.markReachable(dependentSeeds, reached, f.DependentDepth, dependentEdges)

	for _, id := range dependentSeeds {
		if !include[id] {
			dropped[id] = true
		}
	}
	g.partitionReachedDependents(f, reached, include, dropped)

	// A dependent kept through one path must not stay marked as dropped.
	for id := range dropped {
		if include[id] {
			delete(dropped, id)
		}
	}
}

// partitionReachedDependents sorts reached dependents into include (kept) and dropped.
func (g *Graph) partitionReachedDependents(f *SelectionFilter, reached, include, dropped map[string]bool) {
	for id := range reached {
		if include[id] {
			continue
		}
		if f.KeepDependent == nil || f.KeepDependent(g.Nodes[id]) {
			include[id] = true
			continue
		}
		dropped[id] = true
	}
}

// existingNodeIDs returns the IDs that exist in the graph, in input order.
func (g *Graph) existingNodeIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, exists := g.Nodes[id]; exists {
			result = append(result, id)
		}
	}
	return result
}

// contractedEdge is a dependency edge that bypasses one or more dropped nodes.
type contractedEdge struct {
	target   string
	optional bool
}

// contractDroppedEdges adds edges between included nodes that were connected
// only through dropped nodes in the source graph.
func (g *Graph) contractDroppedEdges(filtered *Graph, include, dropped map[string]bool) {
	if len(dropped) == 0 {
		return
	}

	ids := make([]string, 0, len(include))
	for id := range include {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		source := g.Nodes[id]
		for _, edge := range g.contractedEdges(source, include, dropped) {
			addContractedEdge(filtered, id, edge)
		}
	}
}

// addContractedEdge records a contracted edge on the filtered graph. When a
// direct edge already exists the edge stays optional only if every path is optional.
func addContractedEdge(filtered *Graph, fromID string, edge contractedEdge) {
	from := filtered.Nodes[fromID]
	to := filtered.Nodes[edge.target]
	if from == nil || to == nil {
		return
	}
	if from.OptionalDependencies == nil {
		from.OptionalDependencies = make(map[string]bool)
	}
	for _, dep := range from.Dependencies {
		if dep == edge.target {
			from.OptionalDependencies[edge.target] = from.OptionalDependencies[edge.target] && edge.optional
			return
		}
	}
	from.Dependencies = append(from.Dependencies, edge.target)
	from.OptionalDependencies[edge.target] = edge.optional
	to.Dependents = append(to.Dependents, fromID)
}

// walkState is one DFS item through dropped nodes.
type walkState struct {
	id       string
	optional bool
}

// contractionWalk accumulates the included nodes reached from one source
// through dropped nodes only, in first-seen order.
type contractionWalk struct {
	source   *Node
	include  map[string]bool
	dropped  map[string]bool
	order    []string
	optional map[string]bool
}

// initialStack seeds the walk with each dropped direct dependency of the source.
func (w *contractionWalk) initialStack() []walkState {
	var stack []walkState
	for _, dep := range w.source.Dependencies {
		if w.dropped[dep] {
			stack = append(stack, walkState{id: dep, optional: w.source.OptionalDependencies[dep]})
		}
	}
	return stack
}

// expand records the included dependencies of node and returns the dropped ones
// that the walk still has to visit.
func (w *contractionWalk) expand(node *Node, current walkState) []walkState {
	if node == nil {
		return nil
	}
	var next []walkState
	for _, dep := range node.Dependencies {
		pathOptional := current.optional && node.OptionalDependencies[dep]
		switch {
		case w.include[dep]:
			w.record(dep, pathOptional)
		case w.dropped[dep]:
			next = append(next, walkState{id: dep, optional: pathOptional})
		}
	}
	return next
}

// record notes an included target. The edge stays optional only if every path is optional.
func (w *contractionWalk) record(dep string, pathOptional bool) {
	if dep == w.source.ID {
		return
	}
	if previous, seen := w.optional[dep]; seen {
		w.optional[dep] = previous && pathOptional
		return
	}
	w.optional[dep] = pathOptional
	w.order = append(w.order, dep)
}

// edges returns the recorded edges in first-seen order.
func (w *contractionWalk) edges() []contractedEdge {
	edges := make([]contractedEdge, 0, len(w.order))
	for _, target := range w.order {
		edges = append(edges, contractedEdge{target: target, optional: w.optional[target]})
	}
	return edges
}

// contractedEdges finds, for each direct dependency of source that is dropped,
// the first included nodes reachable through dropped nodes only. The walk keeps
// a per-source visited set, so cycles inside the dropped region terminate.
func (g *Graph) contractedEdges(source *Node, include, dropped map[string]bool) []contractedEdge {
	walk := &contractionWalk{
		source:   source,
		include:  include,
		dropped:  dropped,
		optional: make(map[string]bool),
	}
	visited := make(map[walkState]bool)

	stack := walk.initialStack()
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[current] {
			continue
		}
		visited[current] = true
		stack = append(stack, walk.expand(g.Nodes[current.id], current)...)
	}
	return walk.edges()
}

// FilterByType creates a new graph containing only nodes of the specified type.
func (g *Graph) FilterByType(nodeType string) *Graph {
	defer perf.Track(nil, "dependency.Graph.FilterByType")()

	nodeIDs := []string{}
	for id, node := range g.Nodes {
		if node.Type == nodeType {
			nodeIDs = append(nodeIDs, id)
		}
	}

	return g.Filter(Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   false,
	})
}

// FilterByStack creates a new graph containing only nodes from the specified stack.
func (g *Graph) FilterByStack(stack string) *Graph {
	defer perf.Track(nil, "dependency.Graph.FilterByStack")()

	nodeIDs := []string{}
	for id, node := range g.Nodes {
		if node.Stack == stack {
			nodeIDs = append(nodeIDs, id)
		}
	}

	return g.Filter(Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   false,
	})
}

// FilterByComponent creates a new graph containing only nodes with the specified component name.
func (g *Graph) FilterByComponent(component string) *Graph {
	defer perf.Track(nil, "dependency.Graph.FilterByComponent")()

	nodeIDs := []string{}
	for id, node := range g.Nodes {
		if node.Component == component {
			nodeIDs = append(nodeIDs, id)
		}
	}

	return g.Filter(Filter{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   true,
	})
}

// dependencyEdges selects a node's dependency IDs for markReachable.
func dependencyEdges(node *Node) []string {
	return node.Dependencies
}

// dependentEdges selects a node's dependent IDs for markReachable.
func dependentEdges(node *Node) []string {
	return node.Dependents
}

// reachEntry is one BFS frontier item in markReachable.
type reachEntry struct {
	id    string
	depth int
}

// markReachable marks every node reachable from the seed set within maxDepth
// hops (0 = unlimited) following the given edge selector. All seeds enter the
// BFS at depth 0, so the first visit to a node is at its minimum depth from
// the nearest seed; the visited map doubles as cycle protection.
func (g *Graph) markReachable(seeds []string, toInclude map[string]bool, maxDepth int, edges func(*Node) []string) {
	visited := make(map[string]struct{}, len(seeds))
	queue := make([]reachEntry, 0, len(seeds))
	for _, id := range seeds {
		visited[id] = struct{}{}
		queue = append(queue, reachEntry{id: id, depth: 0})
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if maxDepth > 0 && current.depth >= maxDepth {
			continue
		}
		node, exists := g.Nodes[current.id]
		if !exists {
			continue
		}
		for _, nextID := range edges(node) {
			if _, seen := visited[nextID]; seen {
				continue
			}
			visited[nextID] = struct{}{}
			toInclude[nextID] = true
			queue = append(queue, reachEntry{id: nextID, depth: current.depth + 1})
		}
	}
}

// GetConnectedComponents returns all connected components in the graph.
// Each connected component is a subgraph where all nodes are reachable from each other.
func (g *Graph) GetConnectedComponents() []*Graph {
	defer perf.Track(nil, "dependency.Graph.GetConnectedComponents")()

	visited := make(map[string]bool)
	components := []*Graph{}

	// Find all connected components.
	for id := range g.Nodes {
		if !visited[id] {
			// Find all nodes in this component.
			componentNodeIDs := g.findConnectedComponent(id, visited)

			// Build the component graph.
			component := g.buildComponentGraph(componentNodeIDs)
			components = append(components, component)
		}
	}

	return components
}

// findConnectedComponent performs DFS to find all nodes connected to the start node.
// It marks all found nodes as visited and returns their IDs.
func (g *Graph) findConnectedComponent(startID string, visited map[string]bool) map[string]bool {
	componentNodeIDs := make(map[string]bool)
	g.traverseConnectedNodes(startID, visited, componentNodeIDs)
	return componentNodeIDs
}

// traverseConnectedNodes recursively visits all connected nodes using DFS.
func (g *Graph) traverseConnectedNodes(nodeID string, visited map[string]bool, componentNodeIDs map[string]bool) {
	if visited[nodeID] {
		return
	}

	visited[nodeID] = true
	componentNodeIDs[nodeID] = true

	node, exists := g.Nodes[nodeID]
	if !exists {
		return
	}

	// Visit all dependencies.
	for _, depID := range node.Dependencies {
		g.traverseConnectedNodes(depID, visited, componentNodeIDs)
	}

	// Visit all dependents.
	for _, depID := range node.Dependents {
		g.traverseConnectedNodes(depID, visited, componentNodeIDs)
	}
}

// buildComponentGraph creates a new graph containing only the specified nodes.
// It clones nodes and filters their edges to only include nodes within the component.
func (g *Graph) buildComponentGraph(componentNodeIDs map[string]bool) *Graph {
	component := NewGraph()

	// Clone each node with filtered edges.
	for nodeID := range componentNodeIDs {
		node, exists := g.Nodes[nodeID]
		if !exists {
			continue
		}

		// Clone the node with edges filtered to component nodes only.
		clonedNode := cloneNodeWithFilteredEdges(node, componentNodeIDs)
		component.Nodes[nodeID] = clonedNode
	}

	component.IdentifyRoots()
	return component
}

// RemoveNode removes a node and all its relationships from the graph.
func (g *Graph) RemoveNode(nodeID string) error {
	defer perf.Track(nil, "dependency.Graph.RemoveNode")()

	node, exists := g.Nodes[nodeID]
	if !exists {
		return nil // Node doesn't exist, nothing to remove.
	}

	g.removeNodeFromDependencies(nodeID, node)
	g.removeNodeFromDependents(nodeID, node)

	delete(g.Nodes, nodeID)
	g.IdentifyRoots()

	return nil
}

// removeNodeFromDependencies removes the node from its dependencies' dependents lists.
func (g *Graph) removeNodeFromDependencies(nodeID string, node *Node) {
	for _, depID := range node.Dependencies {
		depNode, exists := g.Nodes[depID]
		if !exists {
			continue
		}
		depNode.Dependents = removeStringFromSlice(depNode.Dependents, nodeID)
	}
}

// removeNodeFromDependents removes the node from its dependents' dependencies lists.
func (g *Graph) removeNodeFromDependents(nodeID string, node *Node) {
	for _, depID := range node.Dependents {
		depNode, exists := g.Nodes[depID]
		if !exists {
			continue
		}
		depNode.Dependencies = removeStringFromSlice(depNode.Dependencies, nodeID)
	}
}

// removeStringFromSlice removes a specific string from a slice.
func removeStringFromSlice(slice []string, toRemove string) []string {
	result := []string{}
	for _, item := range slice {
		if item != toRemove {
			result = append(result, item)
		}
	}
	return result
}
