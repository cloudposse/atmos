package dependency

// cloneNodeCore creates a basic deep copy of a node without modifying edges.
// This is a reusable function for creating node copies.
func cloneNodeCore(node *Node) *Node {
	if node == nil {
		return nil
	}

	cloned := &Node{
		ID:           node.ID,
		Component:    node.Component,
		Stack:        node.Stack,
		Type:         node.Type,
		Dependencies: make([]string, len(node.Dependencies)),
		Dependents:   make([]string, len(node.Dependents)),
		Processed:    node.Processed,
	}

	// Copy dependency and dependent slices.
	copy(cloned.Dependencies, node.Dependencies)
	copy(cloned.Dependents, node.Dependents)

	// Deep copy metadata if present.
	cloned.Metadata = cloneNodeMetadata(node.Metadata)

	return cloned
}

// cloneNodeMetadata creates a deep copy of the metadata map.
func cloneNodeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}

	cloned := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

// cloneNodeWithFilteredEdges creates a copy of a node with edges filtered to only include allowed nodes.
func cloneNodeWithFilteredEdges(node *Node, allowedNodes map[string]bool) *Node {
	if node == nil {
		return nil
	}

	cloned := &Node{
		ID:           node.ID,
		Component:    node.Component,
		Stack:        node.Stack,
		Type:         node.Type,
		Dependencies: filterStringSlice(node.Dependencies, allowedNodes),
		Dependents:   filterStringSlice(node.Dependents, allowedNodes),
		Processed:    node.Processed,
		Metadata:     cloneNodeMetadata(node.Metadata),
	}

	return cloned
}

// filterStringSlice returns only the strings that exist in the allowed set.
func filterStringSlice(slice []string, allowed map[string]bool) []string {
	if slice == nil {
		return []string{}
	}

	filtered := make([]string, 0)
	for _, item := range slice {
		if allowed[item] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
