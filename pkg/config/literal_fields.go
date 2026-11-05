package config

import (
	"fmt"
	"slices"
	"strings"

	goyaml "go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	commandKeyName  = "name"
	commandKeyType  = "type"
	commandKeyValue = "value"
)

// commandKeys are the keys that identify a mapping without a `type` as a custom command.
var commandKeys = []string{schema.StepKeySteps, "commands", "arguments", "flags", "description", "component"}

// recordLiteralFields records every !literal marker of a decoded mapping: the fields of the steps
// it holds, and, when the mapping is a custom command, the values of its own env entries.
func recordLiteralFields(node *goyaml.Node, decoded map[string]interface{}) error {
	if err := recordStepLiteralFields(node, decoded); err != nil {
		return err
	}
	recordCommandEnvLiterals(node, decoded)
	return nil
}

// recordStepLiteralFields marks the step mappings of a decoded custom command mapping.
//
// A step is a mapping with a `script` or `command` key, a group step that holds `steps`, or a
// container `run` step (`type: container` with `action: run` or no action), inside the `steps:`
// sequence of a command or of a nested group, parallel, or matrix step. A step written as a plain
// string with the !literal tag becomes a command step that is not rendered. The node argument is
// the YAML mapping that holds the `steps:` key and decoded is its decoded form. Other mappings
// that happen to carry a `!literal` value (settings, flags) are never touched, so they gain no
// extra keys. A !literal tag on a step field that cannot honor it is an error.
func recordStepLiteralFields(node *goyaml.Node, decoded map[string]interface{}) error {
	stepsNode := mappingValueNode(node, schema.StepKeySteps)
	if stepsNode == nil || stepsNode.Kind != goyaml.SequenceNode {
		return nil
	}
	decodedSteps, ok := decoded[schema.StepKeySteps].([]interface{})
	if !ok {
		return nil
	}
	for i, child := range stepsNode.Content {
		if i >= len(decodedSteps) {
			continue
		}
		if err := recordStepLiteralFieldsAt(child, decodedSteps, i); err != nil {
			return err
		}
	}
	return nil
}

// recordStepLiteralFieldsAt records the markers of one entry of a `steps:` sequence.
func recordStepLiteralFieldsAt(child *goyaml.Node, decodedSteps []interface{}, i int) error {
	switch {
	case isLiteralScalar(child):
		if command, isString := decodedSteps[i].(string); isString {
			decodedSteps[i] = map[string]interface{}{
				schema.StepKeyCommand:   command,
				schema.LiteralFieldsKey: []interface{}{schema.StepKeyCommand},
			}
		}
	case child.Kind == goyaml.MappingNode:
		if step, isMap := decodedSteps[i].(map[string]interface{}); isMap {
			return recordStepLiteralFieldsOnStep(child, step, i)
		}
	}
	return nil
}

// recordStepLiteralFieldsOnStep sets the internal literal_fields key on one decoded step mapping.
// A user-written literal_fields is always dropped first: only the loader may set it. Only a mapping
// that is a step (see isStepMapping) can receive a list.
func recordStepLiteralFieldsOnStep(node *goyaml.Node, step map[string]interface{}, index int) error {
	delete(step, schema.LiteralFieldsKey)
	if !isStepMapping(node) {
		return nil
	}
	if err := rejectUnsupportedLiteral(node, index); err != nil {
		return err
	}
	if fields := literalFieldsOfStep(node); len(fields) > 0 {
		list := make([]interface{}, 0, len(fields))
		for _, field := range fields {
			list = append(list, field)
		}
		step[schema.LiteralFieldsKey] = list
	}
	return nil
}

// rejectUnsupportedLiteral fails when !literal is written on a step field that is rendered by
// something other than the step runner's template pass, where the tag would otherwise be dropped
// without effect. Supported fields, env values, and nested steps are skipped.
func rejectUnsupportedLiteral(node *goyaml.Node, index int) error {
	supported := schema.LiteralStepFieldNames()
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if slices.Contains(supported, key) || key == schema.StepKeyEnv || key == schema.StepKeySteps || key == schema.LiteralFieldsKey {
			continue
		}
		if containsLiteralTag(node.Content[i+1]) {
			name := scalarNodeValue(mappingValueNode(node, commandKeyName))
			if name == "" {
				name = fmt.Sprintf("#%d", index+1)
			}
			return fmt.Errorf("%w on field %q of step %q; it is supported on %s, and on individual env values",
				errUtils.ErrLiteralFieldUnsupported, key, name, strings.Join(supported, ", "))
		}
	}
	return nil
}

// containsLiteralTag reports whether a node or any node below it carries the !literal tag.
func containsLiteralTag(node *goyaml.Node) bool {
	if node.Tag == u.AtmosYamlFuncLiteral {
		return true
	}
	return slices.ContainsFunc(node.Content, containsLiteralTag)
}

// recordCommandEnvLiterals marks the env entries of a custom command whose value is written with
// !literal. Both forms are handled: the list of `{key, value}` entries, where the entry gets a
// literal_fields key, and the map form, where the entry becomes `{value, literal_fields}`. Other
// mappings with an env section, such as the global env of atmos.yaml, are never touched.
func recordCommandEnvLiterals(node *goyaml.Node, decoded map[string]interface{}) {
	if !isCommandMapping(node) {
		return
	}
	envNode := mappingValueNode(node, schema.StepKeyEnv)
	if envNode == nil {
		return
	}
	switch decodedEnv := decoded[schema.StepKeyEnv].(type) {
	case []interface{}:
		markEnvListLiterals(envNode, decodedEnv)
	case map[string]interface{}:
		markEnvMapLiterals(envNode, decodedEnv)
	}
}

// markEnvListLiterals marks the `{key, value}` entries whose value is written with !literal.
func markEnvListLiterals(envNode *goyaml.Node, decodedEnv []interface{}) {
	for i, child := range envNode.Content {
		item, ok := entryAt(decodedEnv, i)
		if !ok || child.Kind != goyaml.MappingNode {
			continue
		}
		delete(item, schema.LiteralFieldsKey)
		if isLiteralScalar(mappingValueNode(child, commandKeyValue)) {
			item[schema.LiteralFieldsKey] = []interface{}{commandKeyValue}
		}
	}
}

// markEnvMapLiterals rewrites the entries of the `NAME: value` form whose value is written with
// !literal into `{value, literal_fields}` entries.
func markEnvMapLiterals(envNode *goyaml.Node, decodedEnv map[string]interface{}) {
	for i := 0; i+1 < len(envNode.Content); i += 2 {
		key := envNode.Content[i].Value
		if isLiteralScalar(envNode.Content[i+1]) {
			decodedEnv[key] = map[string]interface{}{
				commandKeyValue:         decodedEnv[key],
				schema.LiteralFieldsKey: []interface{}{commandKeyValue},
			}
		}
	}
}

func entryAt(items []interface{}, index int) (map[string]interface{}, bool) {
	if index >= len(items) {
		return nil, false
	}
	item, ok := items[index].(map[string]interface{})
	return item, ok
}

// isCommandMapping reports whether a mapping is a custom command: it has a name, no step type, and
// at least one of the keys only commands carry.
func isCommandMapping(node *goyaml.Node) bool {
	if mappingValueNode(node, commandKeyName) == nil || mappingValueNode(node, commandKeyType) != nil || isStepMapping(node) {
		return false
	}
	return slices.ContainsFunc(commandKeys, func(key string) bool { return mappingValueNode(node, key) != nil })
}

// isStepMapping reports whether a mapping of a `steps:` sequence is a step that can carry !literal
// fields: it holds a `script` or `command` key, or it is a container `run` step. Container `run`
// keeps its command under `with`, so it has no direct `command` key; the narrow type and action
// check keeps every other mapping that merely has a `type` key from being treated as a step.
func isStepMapping(node *goyaml.Node) bool {
	if mappingValueNode(node, schema.StepKeyScript) != nil || mappingValueNode(node, schema.StepKeyCommand) != nil {
		return true
	}
	// A parallel or matrix step has no command of its own, but its env values are literal-aware.
	if stepsNode := mappingValueNode(node, schema.StepKeySteps); stepsNode != nil && stepsNode.Kind == goyaml.SequenceNode &&
		mappingValueNode(node, commandKeyType) != nil {
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
		// An !include.raw file is the text to use as written, so it is never rendered either.
		if value := mappingValueNode(node, name); isLiteralScalar(value) || isIncludeRawScalar(value) {
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

func isIncludeRawScalar(node *goyaml.Node) bool {
	return node != nil && node.Kind == goyaml.ScalarNode && node.Tag == u.AtmosYamlFuncIncludeRaw
}
