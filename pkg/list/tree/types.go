package tree

// ImportNode represents a node in the import tree with recursive children.
type ImportNode struct {
	Path            string
	Children        []*ImportNode
	Circular        bool   // True if this node creates a circular reference
	ComponentFolder string // The resolved component folder path (e.g., "components/terraform/vpc")
}
