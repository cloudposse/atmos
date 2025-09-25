package dependency

import (
	"fmt"
	"sort"
)

// TopologicalSort returns nodes in dependency order using Kahn's algorithm.
// Nodes with no dependencies are processed first, followed by nodes that depend on them.
func (g *Graph) TopologicalSort() (ExecutionOrder, error) {
	// Create a copy of the graph to avoid modifying the original
	workGraph := g.Clone()

	// Calculate in-degrees for all nodes
	inDegree := make(map[string]int)
	for id, node := range workGraph.Nodes {
		inDegree[id] = len(node.Dependencies)
	}

	// Initialize queue with nodes that have no dependencies
	queue := []string{}
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	// Sort the initial queue for deterministic ordering
	sort.Strings(queue)

	// Process nodes in topological order
	result := ExecutionOrder{}
	processedCount := 0

	for len(queue) > 0 {
		// Dequeue the first node
		currentID := queue[0]
		queue = queue[1:]

		// Add to result
		currentNode := workGraph.Nodes[currentID]
		result = append(result, *currentNode)
		processedCount++

		// Process all dependents of the current node
		readyNodes := []string{}
		for _, dependentID := range currentNode.Dependents {
			inDegree[dependentID]--
			if inDegree[dependentID] == 0 {
				readyNodes = append(readyNodes, dependentID)
			}
		}
		// Sort ready nodes before adding to queue for deterministic ordering
		sort.Strings(readyNodes)
		queue = append(queue, readyNodes...)
	}

	// Check if all nodes were processed
	if processedCount != len(workGraph.Nodes) {
		// Find nodes that weren't processed (involved in cycles)
		unprocessed := []string{}
		for id := range workGraph.Nodes {
			if inDegree[id] > 0 {
				unprocessed = append(unprocessed, id)
			}
		}
		return nil, fmt.Errorf("%w involving nodes: %v", ErrCircularDependency, unprocessed)
	}

	return result, nil
}

// ReverseTopologicalSort returns nodes in reverse dependency order.
// Nodes that depend on others are processed first.
func (g *Graph) ReverseTopologicalSort() (ExecutionOrder, error) {
	order, err := g.TopologicalSort()
	if err != nil {
		return nil, err
	}

	// Reverse the order
	reversed := make(ExecutionOrder, len(order))
	for i := range order {
		reversed[len(order)-1-i] = order[i]
	}

	return reversed, nil
}

// GetExecutionLevels returns nodes grouped by execution level.
// Level 0 contains nodes with no dependencies, level 1 contains nodes that only depend on level 0, etc.
func (g *Graph) GetExecutionLevels() ([][]Node, error) {
	// Check for cycles first
	if hasCycle, cyclePath := g.HasCycles(); hasCycle {
		return nil, fmt.Errorf("%w: %v", ErrCircularDependency, cyclePath)
	}

	levels := [][]Node{}
	processed := make(map[string]bool)
	nodeLevel := make(map[string]int)

	// Calculate the level for each node
	var calculateLevel func(nodeID string) int
	calculateLevel = func(nodeID string) int {
		if level, exists := nodeLevel[nodeID]; exists {
			return level
		}

		node := g.Nodes[nodeID]
		maxDepLevel := -1

		for _, depID := range node.Dependencies {
			depLevel := calculateLevel(depID)
			if depLevel > maxDepLevel {
				maxDepLevel = depLevel
			}
		}

		level := maxDepLevel + 1
		nodeLevel[nodeID] = level
		return level
	}

	// Calculate levels for all nodes
	maxLevel := -1
	for id := range g.Nodes {
		level := calculateLevel(id)
		if level > maxLevel {
			maxLevel = level
		}
	}

	// Initialize levels slice
	for i := 0; i <= maxLevel; i++ {
		levels = append(levels, []Node{})
	}

	// Group nodes by level
	for id, node := range g.Nodes {
		level := nodeLevel[id]
		levels[level] = append(levels[level], *node)
		processed[id] = true
	}

	// Sort nodes within each level for deterministic ordering
	for i := range levels {
		sort.Slice(levels[i], func(a, b int) bool {
			return levels[i][a].ID < levels[i][b].ID
		})
	}

	return levels, nil
}

// FindPath finds a path from one node to another if it exists.
func (g *Graph) FindPath(fromID, toID string) ([]string, bool) {
	if fromID == toID {
		return []string{fromID}, true
	}

	visited := make(map[string]bool)
	path := []string{}

	var dfs func(currentID string) bool
	dfs = func(currentID string) bool {
		if visited[currentID] {
			return false
		}
		visited[currentID] = true
		path = append(path, currentID)

		if currentID == toID {
			return true
		}

		node := g.Nodes[currentID]
		for _, depID := range node.Dependencies {
			if dfs(depID) {
				return true
			}
		}

		// Backtrack
		path = path[:len(path)-1]
		return false
	}

	if dfs(fromID) {
		return path, true
	}

	return nil, false
}

// IsReachable checks if one node is reachable from another.
func (g *Graph) IsReachable(fromID, toID string) bool {
	_, found := g.FindPath(fromID, toID)
	return found
}
