package utils

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	AtmosYamlTags = []string{
		"!terraform.output",
	}
)

// PrintAsYAML prints the provided value as YAML document to the console
func PrintAsYAML(data any) error {
	y, err := ConvertToYAML(data)
	if err != nil {
		return err
	}
	PrintMessage(y)
	return nil
}

// PrintAsYAMLToFileDescriptor prints the provided value as YAML document to a file descriptor
func PrintAsYAMLToFileDescriptor(cliConfig schema.CliConfiguration, data any) error {
	y, err := ConvertToYAML(data)
	if err != nil {
		return err
	}
	LogInfo(cliConfig, y)
	return nil
}

// WriteToFileAsYAML converts the provided value to YAML and writes it to the specified file
func WriteToFileAsYAML(filePath string, data any, fileMode os.FileMode) error {
	y, err := ConvertToYAML(data)
	if err != nil {
		return err
	}
	err = os.WriteFile(filePath, []byte(y), fileMode)
	if err != nil {
		return err
	}
	return nil
}

// ConvertToYAML converts the provided data to a YAML string
func ConvertToYAML(data any) (string, error) {
	y, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

// TaggedValue holds both the value and the YAML tag for each node.
type TaggedValue struct {
	Tag   string `yaml:"tag"`
	Value any    `yaml:"value"`
}

// parseNodeWithTags recursively extracts values and tags from yaml.Node and stores them in a map.
func parseNodeWithTags(node *yaml.Node, out map[string]TaggedValue) error {
	// Handle DocumentNode by unwrapping its content
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return parseNodeWithTags(node.Content[0], out)
	}

	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping node at the root")
	}

	// Process nodes in key-value pairs
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		// Create TaggedValue to store the tag and value
		taggedValue := TaggedValue{Tag: valueNode.Tag}

		switch valueNode.Kind {
		case yaml.ScalarNode:
			// Directly store scalar values
			taggedValue.Value = valueNode.Tag + " " + valueNode.Value
		case yaml.MappingNode:
			// Recursively parse maps
			nestedMap := make(map[string]TaggedValue)
			if err := parseNodeWithTags(valueNode, nestedMap); err != nil {
				return err
			}
			taggedValue.Value = nestedMap
		case yaml.SequenceNode:
			// Parse sequences
			var sequence []TaggedValue
			for _, seqNode := range valueNode.Content {
				seqItem := TaggedValue{Tag: seqNode.Tag, Value: seqNode.Value}
				sequence = append(sequence, seqItem)
			}
			taggedValue.Value = sequence
		case yaml.DocumentNode:
			// Handle nested DocumentNode
			nestedMap := make(map[string]TaggedValue)
			if err := parseNodeWithTags(valueNode, nestedMap); err != nil {
				return err
			}
			taggedValue.Value = nestedMap
		default:
			return fmt.Errorf("invalid YAML node type")
		}

		// Assign parsed TaggedValue to output map with key
		out[keyNode.Value] = taggedValue
	}
	return nil
}

func processCustomTags(node *yaml.Node) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return processCustomTags(node.Content[0])
	}

	for i := 0; i < len(node.Content); i += 2 {
		valueNode := node.Content[i+1]

		if SliceContainsString(AtmosYamlTags, valueNode.Tag) {
			valueNode.Value = valueNode.Tag + " " + valueNode.Value
		}

		if valueNode.Kind == yaml.SequenceNode {
			for _, seqNode := range valueNode.Content {
				if err := processCustomTags(seqNode); err != nil {
					return err
				}
			}
		} else {
			if err := processCustomTags(valueNode); err != nil {
				return err
			}
		}
	}
	return nil
}

func UnmarshalYAML[T any](input string) (T, error) {
	var zeroValue T
	var node yaml.Node
	b := []byte(input)

	// Unmarshal into yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return zeroValue, err
	}

	if err := processCustomTags(&node); err != nil {
		return zeroValue, err
	}

	// Decode the yaml.Node into the desired type T
	var data T
	if err := node.Decode(&data); err != nil {
		return zeroValue, err
	}

	return data, nil
}

//resultWithTags := make(map[string]TaggedValue)
//if err := parseNodeWithTags(&node, resultWithTags); err != nil {
//	return zeroValue, err
//}
//
//_, err := ConvertToYAML(resultWithTags)
//if err != nil {
//	return zeroValue, err
//}
// gopkg.in/yaml.v3.TaggedStyle
