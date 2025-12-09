package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	log "github.com/cloudposse/atmos/pkg/logger"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	functionKey       = "function"
	tagValueFormat    = "%s %s"
	errorFormat       = "%w: %v %v error %v"
	failedToProcess   = "failed to process"
	emptyValueWarning = "execute returned empty value"
)

var ErrExecuteYamlFunctions = errors.New("failed to execute yaml function")

// deleteViperKey removes a key from Viper's configuration by walking the dotted path
// and deleting the final segment from its parent map. This is necessary because
// v.Set(path, nil) leaves the key present (reported as null), which doesn't truly
// remove it from the configuration.
//
// Note: Viper's internal config from ReadConfig cannot be modified by Set(key, nil).
// We must re-read the modified configuration as YAML to truly remove keys.
func deleteViperKey(v *viper.Viper, path string) {
	if path == "" {
		return
	}

	// Get all settings as a map (this returns a deep copy).
	allSettings := v.AllSettings()
	if len(allSettings) == 0 {
		return
	}

	// Split the path into segments.
	segments := strings.Split(path, ".")
	if len(segments) == 0 {
		return
	}

	// Delete the key from the nested map structure.
	if !deleteNestedKey(allSettings, segments) {
		return // Key didn't exist or couldn't be deleted.
	}

	// Re-read the modified settings as YAML.
	// This is necessary because Viper's Set(key, nil) doesn't truly remove keys
	// when the config was loaded via ReadConfig - it maintains the original values.
	yamlBytes, err := yaml.Marshal(allSettings)
	if err != nil {
		log.Debug("Failed to marshal settings to YAML for key deletion", "error", err)
		return
	}

	// Read the modified config back into Viper.
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(string(yamlBytes))); err != nil {
		log.Debug("Failed to re-read config after key deletion", "error", err)
	}
}

// deleteNestedKey deletes a key from a nested map structure given a path of segments.
// Returns true if the key was found and deleted, false otherwise.
func deleteNestedKey(m map[string]any, segments []string) bool {
	if len(segments) == 0 {
		return false
	}

	// If it's a top-level key, delete it directly.
	if len(segments) == 1 {
		key := strings.ToLower(segments[0])
		if _, exists := m[key]; exists {
			delete(m, key)
			return true
		}
		return false
	}

	// Walk to the parent map.
	current := m
	for i := 0; i < len(segments)-1; i++ {
		key := strings.ToLower(segments[i])
		next, ok := current[key]
		if !ok {
			return false // Path doesn't exist.
		}
		nextMap, ok := next.(map[string]any)
		if !ok {
			return false // Not a map, can't traverse further.
		}
		current = nextMap
	}

	// Delete the final key from the parent map.
	finalKey := strings.ToLower(segments[len(segments)-1])
	if _, exists := current[finalKey]; exists {
		delete(current, finalKey)
		return true
	}
	return false
}

// PreprocessYAML processes the given YAML content, replacing specific directives
// (such as !env,!include,!exec,!repo-root) with their corresponding values.
// It parses the YAML content into a tree structure, processes each node recursively,
// and updates the provided Viper instance with resolved values.
//
// Parameters:
// - yamlContent: The raw YAML content as a byte slice.
// - v: A pointer to a Viper instance where processed values will be stored.
//
// Returns:
// - An error if the YAML content cannot be parsed.
func preprocessAtmosYamlFunc(yamlContent []byte, v *viper.Viper) error {
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlContent, &rootNode); err != nil {
		log.Debug("failed to parse YAML", "error", err)
		return err
	}
	if err := processNode(&rootNode, v, ""); err != nil {
		return err
	}
	return nil
}

// processNode recursively traverses a YAML node tree and processes special directives
// (such as !env). If a directive is found, it replaces the corresponding value in Viper
// using values retrieved from Atmos custom functions.
//
// Parameters:
// - node: A pointer to the current YAML node being processed.
// - v: A pointer to a Viper instance where processed values will be stored.
// ProcessNode recursively traverses a YAML node tree and processes custom Atmos YAML directives, updating the provided Viper instance with resolved values.
// It accepts Document, Mapping, Sequence, and tagged Scalar nodes and uses currentPath as the hierarchical key path for nested values.
// Node is the YAML node to process, v is the Viper instance to populate, and currentPath is the hierarchical key path used for setting values.
// It returns an error if processing any directive or child node fails.
func processNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	if node == nil {
		return nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		// Document nodes are just wrappers, process their content.
		for _, child := range node.Content {
			if err := processNode(child, v, currentPath); err != nil {
				return err
			}
		}

	case yaml.MappingNode:
		if err := processMappingNode(node, v, currentPath); err != nil {
			return err
		}

	case yaml.SequenceNode:
		if err := processSequenceNode(node, v, currentPath); err != nil {
			return err
		}

	case yaml.ScalarNode:
		if node.Tag != "" {
			if err := processScalarNode(node, v, currentPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// processMappingNode walks a YAML mapping node, constructs dotted paths for each key under currentPath, and processes each corresponding value into the provided Viper instance.
// It iterates over key/value pairs, appends the key to currentPath using "." when currentPath is non-empty, and delegates processing of each value to processNode.
// Returns any error produced while processing a child value.
func processMappingNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		newPath := keyNode.Value

		if currentPath != "" {
			newPath = currentPath + "." + newPath
		}

		// Check if the value node has the !unset tag.
		if valueNode.Tag == u.AtmosYamlFuncUnset {
			// Remove this key from Viper. The key may have been loaded by Viper's
			// ReadConfig before preprocessing, so we need to explicitly delete it.
			// Using deleteViperKey ensures the key is truly removed (not just set to nil),
			// so IsSet returns false and AllSettings doesn't include it.
			deleteViperKey(v, newPath)
			log.Debug("Unsetting configuration key", "path", newPath)
			continue
		}

		if err := processNode(valueNode, v, newPath); err != nil {
			return err
		}
	}
	return nil
}

// processSequenceNode processes a YAML sequence node and resolves any custom Atmos
// YAML function tags it contains, populating the provided Viper instance with the
// evaluated element values.
//
// If the sequence contains no custom tags this function is a no-op. For sequences
// that require processing it sets each element individually using an index-based
// path (for example, "parent[0]") and, if the sequence yields any values and
// currentPath is non-empty, sets the entire sequence at currentPath. Processed
// scalar tags are cleared on the node to avoid duplicate processing.
//
// Errors from underlying tag evaluation or node decoding are returned.
// SequenceNeedsProcessing checks if any child in the sequence has custom tags.
func sequenceNeedsProcessing(node *yaml.Node) bool {
	for _, child := range node.Content {
		if child.Kind == yaml.ScalarNode && hasCustomTag(child.Tag) {
			return true
		}
		if child.Kind == yaml.MappingNode && containsCustomTags(child) {
			return true
		}
	}
	return false
}

func processSequenceNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	if !sequenceNeedsProcessing(node) {
		return nil
	}

	var values []any
	for idx, child := range node.Content {
		elementPath := fmt.Sprintf("%s[%d]", currentPath, idx)
		value, err := processSequenceElement(child, v, elementPath)
		if err != nil {
			return err
		}
		values = append(values, value)
	}

	if len(values) > 0 && currentPath != "" {
		v.Set(currentPath, values)
	}

	return nil
}

// processSequenceElement processes a single element in a YAML sequence.
// Returns the processed value or an error.
func processSequenceElement(child *yaml.Node, v *viper.Viper, elementPath string) (any, error) {
	switch {
	case child.Kind == yaml.ScalarNode && child.Tag != "":
		// Scalar with a tag: process the tag and get the value.
		value, err := processScalarNodeValue(child)
		if err != nil {
			return nil, err
		}
		// Also set the individual element for path-based access.
		v.Set(elementPath, value)
		// Clear the tag to avoid re-processing.
		child.Tag = ""
		return value, nil
	case child.Kind == yaml.MappingNode:
		// Nested mapping: process recursively to enable path-based access.
		if err := processMappingNode(child, v, elementPath); err != nil {
			return nil, err
		}
		// Decode the mapping for the slice.
		var val any
		if err := child.Decode(&val); err != nil {
			return nil, err
		}
		return val, nil
	default:
		// Other types: decode normally.
		var val any
		if err := child.Decode(&val); err != nil {
			return nil, err
		}
		// Set the individual element.
		v.Set(elementPath, val)
		return val, nil
	}
}

// hasCustomTag reports whether the YAML tag starts with any Atmos custom function prefix (env, exec, include, repo-root, random).
func hasCustomTag(tag string) bool {
	return strings.HasPrefix(tag, u.AtmosYamlFuncEnv) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncExec) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncInclude) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncGitRoot) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncRandom)
}

// containsCustomTags reports whether the node or any of its descendants contains a custom Atmos YAML function tag.
// A custom tag is an Atmos function tag such as !env, !exec, !include, !repo-root, or !random; the function returns true if any node in the subtree has one of these tags.
func containsCustomTags(node *yaml.Node) bool {
	if node == nil {
		return false
	}

	// Check current node.
	if hasCustomTag(node.Tag) {
		return true
	}

	// Recursively check all children.
	for _, child := range node.Content {
		if containsCustomTags(child) {
			return true
		}
	}

	return false
}

// processEnvTag processes the !env tag.
func processEnvTag(strFunc, nodeValue string) (any, error) {
	envValue, err := u.ProcessTagEnv(strFunc, nil)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncEnv, nodeValue, err)
	}
	return strings.TrimSpace(envValue), nil
}

// processExecTag processes the !exec tag.
func processExecTag(strFunc, nodeValue string) (any, error) {
	execValue, err := u.ProcessTagExec(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncExec, nodeValue, err)
	}
	return execValue, nil
}

// processIncludeTag processes the !include tag.
func processIncludeTag(nodeTag, nodeValue, strFunc string) (any, error) {
	includeValue, err := u.UnmarshalYAML[map[any]any](fmt.Sprintf("%s: %s %s", "include_data", nodeTag, nodeValue))
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, nodeValue, err)
	}
	if includeValue != nil {
		if data, ok := includeValue["include_data"]; ok {
			return data, nil
		}
	}
	return nil, nil
}

// processGitRootTag processes the !repo-root tag.
func processGitRootTag(strFunc, nodeValue string) (any, error) {
	gitRootValue, err := u.ProcessTagGitRoot(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncGitRoot, nodeValue, err)
	}
	return strings.TrimSpace(gitRootValue), nil
}

// processRandomTag processes the !random tag.
func processRandomTag(strFunc, nodeValue string) (any, error) {
	randomValue, err := u.ProcessTagRandom(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncRandom, nodeValue, err)
	}
	return randomValue, nil
}

// processScalarNodeValue evaluates a YAML scalar node's custom Atmos tag and returns the resolved value.
// It supports the !env, !exec, !include, !repo-root, and !random tags; failures during evaluation return an error wrapped with ErrExecuteYamlFunctions, and unknown/unsupported tags are decoded and returned as their YAML value.
func processScalarNodeValue(node *yaml.Node) (any, error) {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)

	switch {
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncEnv):
		return processEnvTag(strFunc, node.Value)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncExec):
		return processExecTag(strFunc, node.Value)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncInclude):
		return processIncludeTag(node.Tag, node.Value, strFunc)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncGitRoot):
		return processGitRootTag(strFunc, node.Value)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncRandom):
		return processRandomTag(strFunc, node.Value)
	default:
		var val any
		if err := node.Decode(&val); err != nil {
			return nil, err
		}
		return val, nil
	}
}

// processScalarNode processes a YAML scalar node tagged with an Atmos custom function and stores the resolved value in v.
// It dispatches handling for !env, !exec, !include, !repo-root and !random tags to their respective handlers.
// If the node has no tag or the tag is not one of the recognized Atmos functions, the function is a no-op.
// It returns any error produced by the invoked handler.
func processScalarNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	if node.Tag == "" {
		return nil
	}

	switch {
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncUnset):
		// The !unset tag is handled in processMappingNode by skipping the key.
		// If we reach here, it means !unset was used in a context where it can't
		// prevent the key from being added (e.g., scalar value context).
		// In this case, we simply don't set any value and clear the tag.
		log.Debug("Unsetting configuration key", "path", currentPath)
		node.Tag = "" // Avoid re-processing.
		return nil
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncEnv):
		return handleEnv(node, v, currentPath)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncExec):
		return handleExec(node, v, currentPath)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncInclude):
		return handleInclude(node, v, currentPath)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncGitRoot):
		return handleGitRoot(node, v, currentPath)
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncRandom):
		return handleRandom(node, v, currentPath)
	}
	return nil
}

// handleEnv processes a YAML node with an !env tag and sets the value in Viper, returns an error if the processing fails, warns if the value is empty.
func handleEnv(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	// In atmos.yaml processing, we don't have stack context, so pass nil.
	// This will make !env fall back to OS environment variables only.
	envValue, err := u.ProcessTagEnv(strFunc, nil)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncEnv, node.Value, err)
	}
	envValue = strings.TrimSpace(envValue)
	if envValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	// Set the value in Viper .
	v.Set(currentPath, envValue)
	node.Tag = "" // Avoid re-processing .
	return nil
}

// handleExec Process the !exec tag and set the value in Viper , returns an error if the processing fails, warns if the value is empty.
func handleExec(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	execValue, err := u.ProcessTagExec(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncExec, node.Value, err)
	}
	if execValue != nil {
		// Set the value in Viper .
		v.Set(currentPath, execValue)
	} else {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	node.Tag = "" // Avoid re-processing
	return nil
}

// handleInclude Process the !include tag and set the value in Viper , returns an error if the processing fails, warns if the value is empty.
func handleInclude(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	includeValue, err := u.UnmarshalYAML[map[any]any](fmt.Sprintf("%s: %s %s", "include_data", node.Tag, node.Value))
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
	}
	if includeValue != nil {
		data, ok := includeValue["include_data"]
		if ok {
			// Set the value in Viper.
			v.Set(currentPath, data)
		} else {
			log.Warn("Invalid value returned from the YAML function",
				functionKey, strFunc,
				"value", includeValue,
			)
		}
	} else {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	node.Tag = "" // Avoid re-processing
	return nil
}

// handleGitRoot evaluates an `!repo-root` YAML tag and stores the resulting repository root string into Viper at the given path.
// If evaluation fails, it returns an error wrapped with ErrExecuteYamlFunctions; if the result is empty it logs a debug warning but still sets the value.
func handleGitRoot(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	gitRootValue, err := u.ProcessTagGitRoot(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncGitRoot, node.Value, err)
	}
	gitRootValue = strings.TrimSpace(gitRootValue)
	if gitRootValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	// Set the value in Viper .
	v.Set(currentPath, gitRootValue)
	node.Tag = "" // Avoid re-processing .
	return nil
}

// handleRandom evaluates a YAML scalar tagged with !random and stores the result in the provided Viper instance.
//
// If evaluation succeeds, the resulting value is stored at the given Viper key path (`currentPath`) and the node's
// tag is cleared to avoid re-processing. If the underlying random tag processor returns an error, that error is
// logged and returned wrapped with ErrExecuteYamlFunctions.
func handleRandom(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	randomValue, err := u.ProcessTagRandom(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncRandom, node.Value, err)
	}
	// Set the value in Viper.
	v.Set(currentPath, randomValue)
	node.Tag = "" // Avoid re-processing.
	return nil
}
