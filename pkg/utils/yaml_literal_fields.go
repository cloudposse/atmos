package utils

import (
	"slices"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	hooksKey     = "hooks"
	workflowsKey = "workflows"
	withKey      = "with"
	typeKey      = "type"
	actionKey    = "action"
)

// stackDataSections lists the stack sections that hold plain data, never steps. The
// markLiteralStepFields function does not descend into them, so a `steps` list that a component var
// or setting happens to contain never receives the internal literal_fields key.
var stackDataSections = []string{"vars", "settings", "env", "metadata", "mocks", "locals", "secrets", "backend", "providers"}

// markLiteralStepFields records, on every step mapping of root that has a direct child tagged
// !literal, the internal literal_fields key listing those fields. It must run before the !literal
// tags are cleared so the step runner can keep the tagged fields exactly as written instead of
// rendering them as templates.
//
// Step mappings are found by path, not by key name alone: the `steps` of a workflow, the `with`
// payload of a hook (one step, or a list of steps), and the `steps` nested inside any of those
// (group, parallel, and matrix steps). Every mapping on those paths is sanitized: a user-written
// literal_fields is replaced by the loader's list when the step tags fields with !literal, and
// removed otherwise, because only the loader may set it. Only a mapping that holds a `script` or
// `command` key, or is a container `run` step (see schema.IsContainerRunStep), receives a list.
// Mappings outside those paths, such as `vars` and `settings`, keep the plain "clear the tag"
// behavior, gain no extra keys, and keep any literal_fields key they contain.
//
// The function is idempotent. The marker the loader wrote is a synthesized node (it has no source
// position), so processing a tree again, after the !literal tags are already cleared, keeps it,
// while a marker parsed from YAML text is always treated as user-written.
func markLiteralStepFields(root *yaml.Node) {
	if root == nil {
		return
	}
	switch root.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range root.Content {
			markLiteralStepFields(child)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(root.Content); i += 2 {
			markLiteralStepFieldsInSection(root.Content[i].Value, root.Content[i+1])
		}
	}
}

// markLiteralStepFieldsInSection dispatches one `key: value` pair of a stack mapping.
func markLiteralStepFieldsInSection(key string, value *yaml.Node) {
	switch {
	case key == hooksKey && value.Kind == yaml.MappingNode:
		for i := 0; i+1 < len(value.Content); i += 2 {
			markHookSteps(value.Content[i+1])
		}
	case key == workflowsKey && value.Kind == yaml.MappingNode:
		for i := 0; i+1 < len(value.Content); i += 2 {
			markSteps(mappingValue(value.Content[i+1], schema.StepKeySteps))
		}
	case slices.Contains(stackDataSections, key):
		// Plain data: never contains steps.
	default:
		markLiteralStepFields(value)
	}
}

// markHookSteps marks the step payload of one hook: its `with` value is a single step or a list.
// The hook's own `type` names the step type of a single payload, which carries no `type` key.
func markHookSteps(hook *yaml.Node) {
	with := mappingValue(hook, withKey)
	if with == nil {
		return
	}
	switch with.Kind {
	case yaml.MappingNode:
		markStepOfType(with, scalarValue(mappingValue(hook, typeKey)))
	case yaml.SequenceNode:
		markSteps(with)
	}
}

// markSteps marks every mapping of a `steps` sequence.
func markSteps(steps *yaml.Node) {
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return
	}
	for _, step := range steps.Content {
		markStep(step)
	}
}

// markStep marks one step mapping, taking its step type from its own `type` key, and then the
// steps nested inside it.
func markStep(step *yaml.Node) {
	markStepOfType(step, "")
}

// markStepOfType marks one step mapping and then the steps nested inside it. The declaredType
// argument is the step type known from outside the mapping (a hook's `type`); when empty, the
// mapping's own `type` key is used.
func markStepOfType(step *yaml.Node, declaredType string) {
	if step.Kind != yaml.MappingNode {
		return
	}
	var fields []string
	if isStepMapping(step, declaredType) {
		fields = literalFieldsOfStep(step)
	}
	reconcileLiteralFields(step, fields)
	markSteps(mappingValue(step, schema.StepKeySteps))
}

// isStepMapping reports whether a mapping on a step path is a step that can carry !literal fields:
// it holds a `script` or `command` key, or it is a container `run` step. Container `run` keeps its
// command under `with`, so it has no direct `command` key; the narrow type and action check keeps
// every other mapping that merely has a `type` key from being treated as a step.
func isStepMapping(step *yaml.Node, declaredType string) bool {
	if mappingHasKey(step, schema.StepKeyScript) || mappingHasKey(step, schema.StepKeyCommand) {
		return true
	}
	stepType := declaredType
	if stepType == "" {
		stepType = scalarValue(mappingValue(step, typeKey))
	}
	return schema.IsContainerRunStep(stepType, scalarValue(mappingValue(step, actionKey)))
}

// scalarValue returns the value of a scalar node, or "" for any other node.
func scalarValue(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return ""
	}
	return node.Value
}

// reconcileLiteralFields makes the literal_fields key of a step mapping match the fields the loader
// found: it sets the list when there are fields, and otherwise removes a user-written key. A key the
// loader synthesized on an earlier pass of the same tree is kept, since the !literal tags that
// produced it are already cleared.
func reconcileLiteralFields(step *yaml.Node, fields []string) {
	if len(fields) > 0 {
		setMappingStrings(step, schema.LiteralFieldsKey, fields)
		return
	}
	removeUserLiteralFields(step)
}

// removeUserLiteralFields deletes every literal_fields entry of mapping that came from YAML text.
func removeUserLiteralFields(mapping *yaml.Node) {
	kept := mapping.Content[:0]
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Value == schema.LiteralFieldsKey && !isLoaderWritten(key) {
			continue
		}
		kept = append(kept, key, mapping.Content[i+1])
	}
	// Clear the dropped tail so the removed nodes can be collected.
	clear(mapping.Content[len(kept):])
	mapping.Content = kept
}

// isLoaderWritten reports whether a literal_fields key node was synthesized by setMappingStrings
// rather than parsed from YAML text: parsed nodes always carry a source line.
func isLoaderWritten(key *yaml.Node) bool {
	return key.Line == 0 && key.Column == 0
}

// literalFieldsOfStep returns the fields of a step mapping whose direct value carries !literal:
// the scalar step fields, and each individual `env` value as `env.NAME`.
func literalFieldsOfStep(step *yaml.Node) []string {
	var fields []string
	for _, name := range schema.LiteralStepFieldNames() {
		if isLiteralValue(mappingValue(step, name)) {
			fields = append(fields, name)
		}
	}
	env := mappingValue(step, schema.StepKeyEnv)
	if env == nil || env.Kind != yaml.MappingNode {
		return fields
	}
	for i := 0; i+1 < len(env.Content); i += 2 {
		if isLiteralValue(env.Content[i+1]) {
			fields = append(fields, schema.LiteralFieldEnvPrefix+env.Content[i].Value)
		}
	}
	return fields
}

func isLiteralValue(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag == AtmosYamlFuncLiteral
}

// mappingValue returns the value node stored under key in mapping, or nil.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// setMappingStrings sets key to a sequence of string values in mapping, replacing an existing
// entry in place. Both nodes of the entry are synthesized, so they carry no source position.
func setMappingStrings(mapping *yaml.Node, key string, values []string) {
	items := make([]*yaml.Node, 0, len(values))
	for _, value := range values {
		items = append(items, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
	valueNode := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: items}
	// The key node is always synthesized, even when it replaces an existing entry, so that
	// isLoaderWritten can tell the loader's list from one parsed out of YAML text.
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i] = keyNode
			mapping.Content[i+1] = valueNode
			return
		}
	}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
}
