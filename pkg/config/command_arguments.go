package config

import (
	goyaml "go.yaml.in/yaml/v3"
)

const (
	commandKeyArguments = "arguments"
	commandKeyRequired  = "required"
	commandKeyDefault   = "default"
)

// recordCommandFields applies the loader-side rules of a decoded custom command mapping: the
// !literal markers of its steps and env, and the requirement of its arguments.
func recordCommandFields(node *goyaml.Node, decoded map[string]interface{}) error {
	if err := recordLiteralFields(node, decoded); err != nil {
		return err
	}
	recordArgumentRequirements(node, decoded)
	return nil
}

// recordArgumentRequirements makes an argument that declares neither `required:` nor a `default:`
// required. YAML cannot tell an omitted `required` from `required: false` once decoded into a
// boolean, so the loader writes the implied value where the key is absent. An explicit
// `required: false` still makes the argument optional.
func recordArgumentRequirements(node *goyaml.Node, decoded map[string]interface{}) {
	if !isCommandMapping(node) {
		return
	}
	argumentsNode := mappingValueNode(node, commandKeyArguments)
	if argumentsNode == nil || argumentsNode.Kind != goyaml.SequenceNode {
		return
	}
	decodedArguments, ok := decoded[commandKeyArguments].([]interface{})
	if !ok {
		return
	}
	for i, child := range argumentsNode.Content {
		argument, ok := entryAt(decodedArguments, i)
		if !ok || child.Kind != goyaml.MappingNode || mappingValueNode(child, commandKeyRequired) != nil {
			continue
		}
		if defaultValue, _ := argument[commandKeyDefault].(string); defaultValue == "" {
			argument[commandKeyRequired] = true
		}
	}
}
