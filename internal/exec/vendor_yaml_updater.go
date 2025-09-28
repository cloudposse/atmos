package exec

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// YAMLVersionUpdater uses goccy/go-yaml for proper YAML handling with anchor preservation.
type YAMLVersionUpdater struct{}

// NewYAMLVersionUpdater creates a new YAML updater using goccy/go-yaml.
func NewYAMLVersionUpdater() *YAMLVersionUpdater {
	return &YAMLVersionUpdater{}
}

// UpdateVersionsInFile updates versions in a YAML file while preserving structure.
func (u *YAMLVersionUpdater) UpdateVersionsInFile(filePath string, updates map[string]string) error {
	// Read the file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file '%s': %w", filePath, err)
	}

	// Update versions in content
	updatedContent, err := u.UpdateVersionsInContent(content, updates)
	if err != nil {
		return fmt.Errorf("failed to update versions: %w", err)
	}

	// Write the file back
	return os.WriteFile(filePath, updatedContent, vendorDefaultFilePermissions)
}

// UpdateVersionsInContent updates component versions in YAML content while preserving structure.
func (u *YAMLVersionUpdater) UpdateVersionsInContent(content []byte, updates map[string]string) ([]byte, error) {
	if len(updates) == 0 {
		return content, nil
	}

	// Parse YAML to AST to preserve structure, comments, and anchors
	file, err := parser.ParseBytes(content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Check if we have documents to process
	if !u.hasValidDocument(file) {
		return content, nil
	}

	// Apply updates and encode back to YAML
	return u.applyUpdatesAndEncode(file, updates)
}

// hasValidDocument checks if the file has a valid document to process.
func (u *YAMLVersionUpdater) hasValidDocument(file *ast.File) bool {
	return len(file.Docs) > 0 && file.Docs[0] != nil && file.Docs[0].Body != nil
}

// applyUpdatesAndEncode applies updates to the AST and encodes back to YAML.
func (u *YAMLVersionUpdater) applyUpdatesAndEncode(file *ast.File, updates map[string]string) ([]byte, error) {
	// Track components and their anchors for proper updates
	componentNodes := u.findComponentNodes(file.Docs[0].Body)

	// Apply updates to the AST
	u.applyUpdatesToNodes(componentNodes, updates)

	// Try to convert AST back to YAML string
	// Using the AST String() method directly to preserve anchors and aliases
	if file.Docs[0].Body != nil {
		yamlStr := file.String()
		return []byte(yamlStr), nil
	}

	// Fallback to empty YAML if body is nil
	return []byte{}, nil
}

// applyUpdatesToNodes applies version updates to the component nodes.
func (u *YAMLVersionUpdater) applyUpdatesToNodes(componentNodes map[string][]*ast.MappingNode, updates map[string]string) {
	for componentName, newVersion := range updates {
		if nodes, exists := componentNodes[componentName]; exists {
			for _, node := range nodes {
				u.updateVersionInNode(node, newVersion)
			}
		}
	}
}

// findComponentNodes finds all nodes that define components in the YAML AST.
func (u *YAMLVersionUpdater) findComponentNodes(node ast.Node) map[string][]*ast.MappingNode {
	components := make(map[string][]*ast.MappingNode)
	u.walkAST(node, func(n ast.Node) bool {
		if mapping, ok := n.(*ast.MappingNode); ok {
			componentName := u.extractComponentName(mapping)
			if componentName != "" {
				components[componentName] = append(components[componentName], mapping)
			}
		}
		return true
	})
	return components
}

// extractComponentName extracts the component name from a mapping node.
func (u *YAMLVersionUpdater) extractComponentName(mapping *ast.MappingNode) string {
	for _, value := range mapping.Values {
		if value == nil || value.Key == nil {
			continue
		}
		if keyNode, ok := value.Key.(*ast.StringNode); ok && keyNode.Value == "component" {
			if value.Value != nil {
				if valueNode, ok := value.Value.(*ast.StringNode); ok {
					return valueNode.Value
				}
			}
		}
	}
	return ""
}

// updateVersionInNode updates the version field in a mapping node.
func (u *YAMLVersionUpdater) updateVersionInNode(mapping *ast.MappingNode, newVersion string) {
	for _, value := range mapping.Values {
		if value == nil || value.Key == nil {
			continue
		}
		if keyNode, ok := value.Key.(*ast.StringNode); ok && keyNode.Value == "version" {
			// Update the version value
			if valueNode, ok := value.Value.(*ast.StringNode); ok {
				valueNode.Value = newVersion
			} else {
				// Create new string node if version wasn't a string
				value.Value = &ast.StringNode{
					Value: newVersion,
				}
			}
			return
		}
	}

	// If no version field exists, add one
	u.addVersionField(mapping, newVersion)
}

// addVersionField adds a version field to a mapping node.
func (u *YAMLVersionUpdater) addVersionField(mapping *ast.MappingNode, version string) {
	versionKey := &ast.StringNode{Value: "version"}
	versionValue := &ast.StringNode{Value: version}

	mapping.Values = append(mapping.Values, &ast.MappingValueNode{
		Key:   versionKey,
		Value: versionValue,
	})
}

// walkAST walks the AST and calls the visitor function for each node.
func (u *YAMLVersionUpdater) walkAST(node ast.Node, visitor func(ast.Node) bool) {
	if node == nil || !visitor(node) {
		return
	}

	u.walkNodeChildren(node, visitor)
}

// walkNodeChildren walks the children of a node based on its type.
func (u *YAMLVersionUpdater) walkNodeChildren(node ast.Node, visitor func(ast.Node) bool) {
	switch n := node.(type) {
	case *ast.DocumentNode:
		u.walkAST(n.Body, visitor)
	case *ast.MappingNode:
		u.walkMappingNode(n, visitor)
	case *ast.SequenceNode:
		u.walkSequenceNode(n, visitor)
	case *ast.MappingValueNode:
		u.walkAST(n.Key, visitor)
		u.walkAST(n.Value, visitor)
	case *ast.AnchorNode:
		u.walkAST(n.Value, visitor)
	case *ast.AliasNode:
		// Aliases are references, don't walk into them
	}
}

// walkMappingNode walks all values in a mapping node.
func (u *YAMLVersionUpdater) walkMappingNode(n *ast.MappingNode, visitor func(ast.Node) bool) {
	for _, value := range n.Values {
		if value != nil {
			u.walkAST(value.Key, visitor)
			u.walkAST(value.Value, visitor)
		}
	}
}

// walkSequenceNode walks all values in a sequence node.
func (u *YAMLVersionUpdater) walkSequenceNode(n *ast.SequenceNode, visitor func(ast.Node) bool) {
	for _, value := range n.Values {
		u.walkAST(value, visitor)
	}
}
