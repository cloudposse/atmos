package main

import (
	"fmt"
	"log"
	"strings"

	"gopkg.in/yaml.v3"
)

// MyFunc represents the custom YAML function type.
type MyFunc struct {
	Arg1 string
	Arg2 string
}

// Implement the yaml.Unmarshaler interface to parse "!!my-func" nodes.
func (mf *MyFunc) UnmarshalYAML(value *yaml.Node) error {
	// Check that the node is scalar and tagged as !!my-func.
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("expected scalar node, got %v", value.Kind)
	}
	if value.Tag != "!!my-func" {
		return fmt.Errorf("unexpected tag: %s", value.Tag)
	}

	// Split the scalar value into two arguments.
	parts := strings.Fields(value.Value)
	if len(parts) != 2 {
		return fmt.Errorf("expected 2 arguments, got %d", len(parts))
	}

	mf.Arg1 = parts[0]
	mf.Arg2 = parts[1]
	return nil
}

// LazyEval evaluates the function logic based on the stored arguments.
func (mf *MyFunc) LazyEval() string {
	return fmt.Sprintf("Executed my-func with args: %s, %s", mf.Arg1, mf.Arg2)
}

// Recursive function to traverse the YAML tree.
func processYAML(node *yaml.Node) any {
	switch node.Kind {
	case yaml.DocumentNode:
		// Process the first child, which is usually the root of the YAML document.
		if len(node.Content) > 0 {
			return processYAML(node.Content[0])
		}
		return nil

	case yaml.MappingNode:
		// Process mappings (key-value pairs).
		m := make(map[string]interface{})
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := processYAML(node.Content[i+1])
			m[key] = value
		}
		return m

	case yaml.SequenceNode:
		// Process sequences (lists).
		var arr []interface{}
		for _, elem := range node.Content {
			arr = append(arr, processYAML(elem))
		}
		return arr

	case yaml.ScalarNode:
		// Check if the scalar node is tagged as !!my-func.
		if node.Tag == "!!my-func" {
			var mf MyFunc
			if err := node.Decode(&mf); err != nil {
				log.Fatalf("error decoding my-func: %v", err)
			}
			// Lazy-evaluate the function.
			return mf.LazyEval()
		}
		return node.Value

	default:
		// Handle unexpected node kinds.
		return nil
	}
}

func main() {
	// Example YAML with nested structure and custom !!my-func type.
	yamlData := `
vars:
  var1: !!my-func 10 20
  var2: "some value"
list:
  - !!my-func 30 40
  - item2
`

	// Parse the YAML into a generic yaml.Node structure.
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(yamlData), &root); err != nil {
		log.Fatalf("error: %v", err)
	}

	// Process the YAML tree and print the result.
	result := processYAML(&root)
	fmt.Printf("Processed YAML: %+v\n", result)
}
