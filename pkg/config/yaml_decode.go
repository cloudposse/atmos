package config

import (
	"context"
	"fmt"
	"path/filepath"

	"go.yaml.in/yaml/v3"
	legacyyaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// decodeNodeWithYamlFunctions decodes a YAML node into plain Go values while
// evaluating Atmos YAML functions on scalar nodes. It is used by config paths
// that must read raw YAML directly instead of Viper's normalized settings.
func decodeNodeWithYamlFunctions(node *yaml.Node) (any, error) {
	return decodeNodeWithYamlFunctionsForFile(node, "")
}

// decodeNodeWithYamlFunctionsForFile evaluates config YAML with source-relative includes
// and a recursion guard shared by every include in this decoding operation.
func decodeNodeWithYamlFunctionsForFile(node *yaml.Node, sourceFile string) (any, error) {
	decoder := configYamlDecoder{includes: make(map[string]bool)}
	return decoder.decode(node, sourceFile)
}

// decode dispatches each YAML node to its config evaluator while retaining the
// source file and active include chain across nested mappings and sequences.
func (d *configYamlDecoder) decode(node *yaml.Node, sourceFile string) (any, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind == yaml.AliasNode {
		return d.decodeAlias(node, sourceFile)
	}
	if node.Anchor != "" {
		return d.decodeAnchored(node, sourceFile)
	}
	return d.decodeNode(node, sourceFile)
}

// decodeNode evaluates one node by kind, without consulting the anchor cache.
func (d *configYamlDecoder) decodeNode(node *yaml.Node, sourceFile string) (any, error) {
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return d.decode(node.Content[0], sourceFile)
	case yaml.MappingNode:
		return d.decodeMappingNodeWithYamlFunctions(node, sourceFile)
	case yaml.SequenceNode:
		return d.decodeSequenceNodeWithYamlFunctions(node, sourceFile)
	case yaml.ScalarNode:
		return d.decodeScalarNodeWithYamlFunctions(node, sourceFile)
	default:
		return decodePlainYamlNode(node)
	}
}

// decodeMappingNodeWithYamlFunctions evaluates mapping values while preserving key spelling.
func (d *configYamlDecoder) decodeMappingNodeWithYamlFunctions(node *yaml.Node, sourceFile string) (any, error) {
	result := make(map[string]interface{}, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		if isMergeKey(keyNode) {
			continue
		}
		d.path = append(d.path, keyNode.Value)
		d.nodeOrigin = nil
		value, err := d.decode(valueNode, sourceFile)
		if err == nil && d.origin != nil {
			d.recordValueOrigin(valueNode, sourceFile)
		}
		d.nodeOrigin = nil
		d.path = d.path[:len(d.path)-1]
		if err != nil {
			return nil, err
		}
		result[keyNode.Value] = value
	}
	if err := d.expandMergeKeys(node, result, sourceFile); err != nil {
		return nil, err
	}
	recordScriptSource(node, result, sourceFile)
	if err := recordCommandFields(node, result); err != nil {
		return nil, err
	}
	return result, nil
}

// recordValueOrigin reports the file that declared the value just decoded at the current path. A
// value produced by an include, or by an alias of an anchored include, belongs to the included file
// (and is remote when that file is), not to the file that merely references it.
func (d *configYamlDecoder) recordValueOrigin(valueNode *yaml.Node, sourceFile string) {
	file, remote := sourceFile, false
	if d.remoteSource != "" {
		file, remote = d.remoteSource, true
	}
	if included := d.nodeOrigin; included != nil && (valueNode.Kind == yaml.AliasNode || isIncludeTag(valueNode.Tag)) {
		file, remote = included.file, included.remote
	}
	d.origin(d.path, file, remote)
}

// decodeSequenceNodeWithYamlFunctions evaluates sequence elements in their original order.
func (d *configYamlDecoder) decodeSequenceNodeWithYamlFunctions(node *yaml.Node, sourceFile string) (any, error) {
	result := make([]interface{}, 0, len(node.Content))
	for _, child := range node.Content {
		value, err := d.decode(child, sourceFile)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// decodeScalarNodeWithYamlFunctions evaluates custom tags and decodes ordinary YAML scalars.
func (d *configYamlDecoder) decodeScalarNodeWithYamlFunctions(node *yaml.Node, sourceFile string) (any, error) {
	if hasCustomTag(node.Tag) {
		return d.processScalarNodeValueForFile(node, sourceFile)
	}
	return decodePlainYamlNode(node)
}

func decodePlainYamlNode(node *yaml.Node) (any, error) {
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// processScalarNodeValueForFile resolves includes against the source file when
// available and delegates other scalar tags to the common config evaluator.
func (d *configYamlDecoder) processScalarNodeValueForFile(node *yaml.Node, sourceFile string) (any, error) {
	if node.Tag == u.AtmosYamlFuncCEL {
		return (yamlsource.Source{Tag: node.Tag, Argument: node.Value, File: sourceFile, Line: node.Line, Column: node.Column}).Encode(), nil
	}
	guard.RememberValue(node.Tag+" "+node.Value, guard.FileOrigin(sourceFile))
	if sourceFile == "" || !isIncludeTag(node.Tag) {
		return processScalarNodeValue(node)
	}

	return d.processIncludeNodeValueForFile(node, sourceFile)
}

// redactIncludeValue removes credentials from an include argument before it is echoed in an error.
// Only URL-shaped values are touched: a local path or an SCP-style git address keeps its text.
func redactIncludeValue(value string) string {
	return u.RedactIncludeArgument(value)
}

func isIncludeTag(tag string) bool {
	return tag == u.AtmosYamlFuncInclude || tag == u.AtmosYamlFuncIncludeRaw
}

// processIncludeNodeValueForFile loads an include and evaluates opted-in YAML
// before decoding, rejecting repeated or excessively deep active include chains.
func (d *configYamlDecoder) processIncludeNodeValueForFile(node *yaml.Node, sourceFile string) (any, error) {
	if err := admission.FunctionValue(context.Background(), node.Tag, node.Tag+" "+node.Value); err != nil {
		return nil, err
	}
	// Resolve the path against the config file and the project base path, never the working
	// directory, so the result is the same wherever Atmos runs from.
	scope := commandIncludeScope(sourceFile)
	include := scope.Resolve(node.Value)
	value := include.Value
	resolved := legacyyaml.Node{
		Kind:  legacyyaml.ScalarNode,
		Tag:   node.Tag,
		Value: value,
	}
	key := sourceFile + "\x00" + node.Value
	if d.includes[key] || len(d.includes) >= maxConfigIncludeDepth {
		return nil, fmt.Errorf("%w: recursive include: %s", ErrExecuteYamlFunctions, redactIncludeValue(node.Value))
	}
	d.includes[key] = true
	defer delete(d.includes, key)
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:         scope.BasePath,
		BasePathAbsolute: scope.BasePath,
	}
	outerTaint := d.remoteTaint
	d.remoteTaint = nil
	walked, err := d.runInclude(atmosConfig, &resolved, node.Tag, value, sourceFile)
	walkedRemote := d.remoteTaint
	d.remoteTaint = outerTaint
	if err != nil {
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, redactIncludeValue(node.Value), err)
	}
	var decoded any
	if err := resolved.Decode(&decoded); err != nil {
		return nil, err
	}
	if d.origin != nil {
		d.recordIncludeOrigin(includeResult{localPath: include.LocalPath, target: node.Value, decoded: decoded, walked: walked, walkedRemote: walkedRemote})
	}
	return decoded, nil
}

// recordIncludedOrigins records plain, non-evaluated mappings without running their tags.
func (d *configYamlDecoder) recordIncludedOrigins(value any, file string, remote bool) {
	mapping, ok := value.(map[string]any)
	if !ok {
		return
	}
	for key, child := range mapping {
		d.path = append(d.path, key)
		d.recordIncludedOrigins(child, file, remote)
		d.origin(d.path, file, remote)
		d.path = d.path[:len(d.path)-1]
	}
}

// includeBasePathForSourceFile returns the directory a config file's bare include paths anchor
// to when it declares no base path: the config file's own directory, or the parent of the
// nearest enclosing `atmos.d` or `.atmos.d` directory for files loaded from one (at any depth).
func includeBasePathForSourceFile(sourceFile string) string {
	configDir := filepath.Dir(sourceFile)
	for dir := configDir; ; dir = filepath.Dir(dir) {
		if base := filepath.Base(dir); base == ".atmos.d" || base == "atmos.d" {
			return filepath.Dir(dir)
		}
		if parent := filepath.Dir(dir); parent == dir {
			return configDir
		}
	}
}
