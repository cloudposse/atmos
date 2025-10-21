//nolint:revive // Error wrapping pattern used throughout.
package exec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// updateYAMLVersion updates a version field in a YAML file while preserving structure.
// This uses yaml.v3's Node API to preserve comments, anchors, and formatting.
func updateYAMLVersion(atmosConfig *schema.AtmosConfiguration, filePath string, componentName string, newVersion string) error {
	defer perf.Track(atmosConfig, "exec.updateYAMLVersion")()

	// Read the YAML file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("%w: %s", errUtils.ErrReadFile, err)
	}

	// Parse into yaml.Node to preserve structure
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("%w: %s", errUtils.ErrParseFile, err)
	}

	// Find and update the version field for the specified component
	updated := false
	if err := updateVersionInNode(&root, componentName, newVersion, &updated); err != nil {
		return err
	}

	if !updated {
		return fmt.Errorf("%w: %s", errUtils.ErrVendorComponentNotFound, componentName)
	}

	// Marshal back to YAML, preserving structure
	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("%w: %s", errUtils.ErrYAMLUpdateFailed, err)
	}

	// Write back to file
	if err := os.WriteFile(filePath, out, 0o644); err != nil { //nolint:gosec,revive // Standard file permissions for YAML config files
		return fmt.Errorf("%w: %s", errUtils.ErrYAMLUpdateFailed, err)
	}

	return nil
}

// updateVersionInNode recursively searches for a component and updates its version.
// This preserves the Node structure including comments, anchors, and formatting.
//
//nolint:gocognit,nestif,revive,cyclop // YAML tree traversal naturally has high complexity.
func updateVersionInNode(node *yaml.Node, componentName string, newVersion string, updated *bool) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		// Document node wraps the content
		for _, child := range node.Content {
			if err := updateVersionInNode(child, componentName, newVersion, updated); err != nil {
				return err
			}
		}

	case yaml.MappingNode:
		// Mapping node is a key-value map
		// Content is [key1, value1, key2, value2, ...]
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Check if this is a "sources" section
			if keyNode.Value == "sources" && valueNode.Kind == yaml.SequenceNode {
				// Search sources for the component
				for _, sourceNode := range valueNode.Content {
					if sourceNode.Kind == yaml.MappingNode {
						// Look for component and version fields
						var compNameFound bool
						var versionNodeIndex int

						for j := 0; j < len(sourceNode.Content); j += 2 {
							k := sourceNode.Content[j]
							v := sourceNode.Content[j+1]

							if k.Value == "component" && v.Value == componentName {
								compNameFound = true
							}
							if k.Value == "version" {
								versionNodeIndex = j + 1
							}
						}

						// If we found the component, update its version
						if compNameFound && versionNodeIndex > 0 {
							sourceNode.Content[versionNodeIndex].Value = newVersion
							*updated = true
							return nil
						}
					}
				}
			}

			// Recursively search in nested structures
			if err := updateVersionInNode(valueNode, componentName, newVersion, updated); err != nil {
				return err
			}
		}

	case yaml.SequenceNode:
		// Sequence node is an array
		for _, child := range node.Content {
			if err := updateVersionInNode(child, componentName, newVersion, updated); err != nil {
				return err
			}
		}
	}

	return nil
}

// findComponentVersion finds the current version of a component in a YAML file.
func findComponentVersion(atmosConfig *schema.AtmosConfiguration, filePath string, componentName string) (string, error) {
	defer perf.Track(atmosConfig, "exec.findComponentVersion")()

	// Read the YAML file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("%w: %s", errUtils.ErrReadFile, err)
	}

	// Parse into yaml.Node
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return "", fmt.Errorf("%w: %s", errUtils.ErrParseFile, err)
	}

	// Find the version
	version := ""
	if err := findVersionInNode(&root, componentName, &version); err != nil {
		return "", err
	}

	if version == "" {
		return "", fmt.Errorf("%w: %s", errUtils.ErrComponentNotFound, componentName)
	}

	return version, nil
}

// findVersionInNode recursively searches for a component and returns its version.
//
//nolint:gocognit,nestif,revive,cyclop // YAML tree traversal naturally has high complexity.
func findVersionInNode(node *yaml.Node, componentName string, version *string) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := findVersionInNode(child, componentName, version); err != nil {
				return err
			}
		}

	case yaml.MappingNode:
		// Content is [key1, value1, key2, value2, ...]
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Check if this is a "sources" section
			if keyNode.Value == "sources" && valueNode.Kind == yaml.SequenceNode {
				for _, sourceNode := range valueNode.Content {
					if sourceNode.Kind == yaml.MappingNode {
						var compNameFound bool
						var compVersion string

						for j := 0; j < len(sourceNode.Content); j += 2 {
							k := sourceNode.Content[j]
							v := sourceNode.Content[j+1]

							if k.Value == "component" && v.Value == componentName {
								compNameFound = true
							}
							if k.Value == "version" {
								compVersion = v.Value
							}
						}

						if compNameFound && compVersion != "" {
							*version = compVersion
							return nil
						}
					}
				}
			}

			// Recursively search
			if err := findVersionInNode(valueNode, componentName, version); err != nil {
				return err
			}
		}

	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := findVersionInNode(child, componentName, version); err != nil {
				return err
			}
		}
	}

	return nil
}
