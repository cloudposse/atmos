package config

import (
	goyaml "go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// recordStepLiteralFields marks the step mappings of a decoded custom command mapping.
//
// A step is a mapping with a `script` or `command` key, or a container `run` step (`type: container`
// with `action: run` or no action), inside the `steps:` sequence of a command or of a nested group,
// parallel, or matrix step. The node argument is the YAML mapping that
// holds the `steps:` key and decoded is its decoded form. Other mappings that happen to carry a
// `!literal` value (settings, flags, env lists) are never touched, so they gain no extra keys.
func recordStepLiteralFields(node *goyaml.Node, decoded map[string]interface{}) {
	stepsNode := mappingValueNode(node, schema.StepKeySteps)
	if stepsNode == nil || stepsNode.Kind != goyaml.SequenceNode {
		return
	}
	decodedSteps, ok := decoded[schema.StepKeySteps].([]interface{})
	if !ok {
		return
	}
	for i, child := range stepsNode.Content {
		if i >= len(decodedSteps) || child.Kind != goyaml.MappingNode {
			continue
		}
		if step, isMap := decodedSteps[i].(map[string]interface{}); isMap {
			recordStepLiteralFieldsOnStep(child, step)
		}
	}
}

// recordStepLiteralFieldsOnStep sets the internal literal_fields key on one decoded step mapping.
// A user-written literal_fields is always dropped first: only the loader may set it. Only a mapping
// that is a step (see isStepMapping) can receive a list.
func recordStepLiteralFieldsOnStep(node *goyaml.Node, step map[string]interface{}) {
	delete(step, schema.LiteralFieldsKey)
	if !isStepMapping(node) {
		return
	}
	if fields := literalFieldsOfStep(node); len(fields) > 0 {
		list := make([]interface{}, 0, len(fields))
		for _, field := range fields {
			list = append(list, field)
		}
		step[schema.LiteralFieldsKey] = list
	}
}

// isStepMapping reports whether a mapping of a `steps:` sequence is a step that can carry !literal
// fields: it holds a `script` or `command` key, or it is a container `run` step. Container `run`
// keeps its command under `with`, so it has no direct `command` key; the narrow type and action
// check keeps every other mapping that merely has a `type` key from being treated as a step.
func isStepMapping(node *goyaml.Node) bool {
	if mappingValueNode(node, schema.StepKeyScript) != nil || mappingValueNode(node, schema.StepKeyCommand) != nil {
		return true
	}
	return schema.IsContainerRunStep(scalarNodeValue(mappingValueNode(node, "type")), scalarNodeValue(mappingValueNode(node, "action")))
}

// scalarNodeValue returns the value of a scalar node, or "" for any other node.
func scalarNodeValue(node *goyaml.Node) string {
	if node == nil || node.Kind != goyaml.ScalarNode {
		return ""
	}
	return node.Value
}

// literalFieldsOfStep returns the fields of a step mapping whose direct value carries !literal:
// the scalar step fields, and each individual `env` value as `env.NAME`.
func literalFieldsOfStep(node *goyaml.Node) []string {
	var fields []string
	for _, name := range schema.LiteralStepFieldNames() {
		if isLiteralScalar(mappingValueNode(node, name)) {
			fields = append(fields, name)
		}
	}
	envNode := mappingValueNode(node, schema.StepKeyEnv)
	if envNode == nil || envNode.Kind != goyaml.MappingNode {
		return fields
	}
	for i := 0; i+1 < len(envNode.Content); i += 2 {
		if isLiteralScalar(envNode.Content[i+1]) {
			fields = append(fields, schema.LiteralFieldEnvPrefix+envNode.Content[i].Value)
		}
	}
	return fields
}

func isLiteralScalar(node *goyaml.Node) bool {
	return node != nil && node.Kind == goyaml.ScalarNode && node.Tag == u.AtmosYamlFuncLiteral
}
