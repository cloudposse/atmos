// Package ui provides Terraform streaming UI components.
package ui

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
	Key               string      // Attribute name.
	Before            interface{} // Value before change (nil for create).
	After             interface{} // Value after change (nil for delete).
	Unknown           bool        // True if value is "(known after apply)".
	Sensitive         bool        // True if value is sensitive.
	ForcesReplacement bool        // True if this attribute forces resource replacement.
}
