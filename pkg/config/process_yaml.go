package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
	legacyyaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	atmosGit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
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
	// Handle !append tag for list concatenation during merging.
	if node.Tag == u.AtmosYamlFuncAppend {
		return handleAppend(node, v, currentPath)
	}

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

// hasCustomTag reports whether the YAML tag is a non-standard explicit YAML tag.
func hasCustomTag(tag string) bool {
	return strings.HasPrefix(tag, "!") && !strings.HasPrefix(tag, "!!")
}

func isStandardYAMLTag(tag string) bool {
	return strings.HasPrefix(tag, "!!")
}

// containsCustomTags reports whether the node or any of its descendants contains
// a non-standard explicit YAML tag.
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
	gitRootValue, err := atmosGit.ProcessTagRoot(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(gitRootValue), nil
}

// processGitShaTag processes the !git.sha and !git.ref tags.
func processGitShaTag(strFunc, nodeValue string) (any, error) {
	gitShaValue, err := atmosGit.ProcessTagSHA(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(gitShaValue), nil
}

// processGitBranchTag processes the !git.branch tag.
func processGitBranchTag(strFunc, nodeValue string) (any, error) {
	gitBranchValue, err := atmosGit.ProcessTagBranch(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(gitBranchValue), nil
}

// processGitRepoInfoTag processes the repository-metadata tags (!git.repository,
// !git.owner, !git.name, !git.host, !git.url) using the supplied processor.
func processGitRepoInfoTag(strFunc, nodeValue string, process func(string) (string, error)) (any, error) {
	value, err := process(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, strFunc, nodeValue, err)
	}
	return strings.TrimSpace(value), nil
}

// processCwdTag processes the !cwd tag.
func processCwdTag(strFunc, nodeValue string) (any, error) {
	cwdValue, err := u.ProcessTagCwd(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncCwd, nodeValue, err)
	}
	return strings.TrimSpace(cwdValue), nil
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
// It supports the atmos.yaml YAML tags registered in pkg/function/tag; failures
// during evaluation return an error wrapped with ErrExecuteYamlFunctions, and
// unknown/unsupported custom tags return ErrUnsupportedYamlTag.
func processScalarNodeValue(node *yaml.Node) (any, error) {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)

	if isStandardYAMLTag(node.Tag) {
		var val any
		if err := node.Decode(&val); err != nil {
			return nil, err
		}
		return val, nil
	}

	switch node.Tag {
	case u.AtmosYamlFuncUnset:
		return nil, nil
	case u.AtmosYamlFuncEnv:
		return processEnvTag(strFunc, node.Value)
	case u.AtmosYamlFuncExec:
		return processExecTag(strFunc, node.Value)
	case u.AtmosYamlFuncInclude, u.AtmosYamlFuncIncludeRaw:
		return processIncludeTag(node.Tag, node.Value, strFunc)
	case u.AtmosYamlFuncGitRoot, u.AtmosYamlFuncGitRootAlias:
		return processGitRootTag(strFunc, node.Value)
	case u.AtmosYamlFuncGitSha, u.AtmosYamlFuncGitRef:
		return processGitShaTag(strFunc, node.Value)
	case u.AtmosYamlFuncGitBranch:
		return processGitBranchTag(strFunc, node.Value)
	case u.AtmosYamlFuncGitRepository:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagRepository)
	case u.AtmosYamlFuncGitOwner:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagOwner)
	case u.AtmosYamlFuncGitName:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagName)
	case u.AtmosYamlFuncGitHost:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagHost)
	case u.AtmosYamlFuncGitUrl:
		return processGitRepoInfoTag(strFunc, node.Value, atmosGit.ProcessTagURL)
	case u.AtmosYamlFuncCwd:
		return processCwdTag(strFunc, node.Value)
	case u.AtmosYamlFuncRandom:
		return processRandomTag(strFunc, node.Value)
	default:
		return nil, unsupportedAtmosYamlTagError(node.Tag, "")
	}
}

// decodeNodeWithYamlFunctions decodes a YAML node into plain Go values while
// evaluating Atmos YAML functions on scalar nodes. It is used by config paths
// that must read raw YAML directly instead of Viper's normalized settings.
func decodeNodeWithYamlFunctions(node *yaml.Node) (any, error) {
	return decodeNodeWithYamlFunctionsForFile(node, "")
}

func decodeNodeWithYamlFunctionsForFile(node *yaml.Node, sourceFile string) (any, error) {
	if node == nil {
		return nil, nil
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return decodeNodeWithYamlFunctionsForFile(node.Content[0], sourceFile)
	case yaml.MappingNode:
		return decodeMappingNodeWithYamlFunctions(node, sourceFile)
	case yaml.SequenceNode:
		return decodeSequenceNodeWithYamlFunctions(node, sourceFile)
	case yaml.ScalarNode:
		return decodeScalarNodeWithYamlFunctions(node, sourceFile)
	default:
		return decodePlainYamlNode(node)
	}
}

func decodeMappingNodeWithYamlFunctions(node *yaml.Node, sourceFile string) (any, error) {
	result := make(map[string]interface{}, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		value, err := decodeNodeWithYamlFunctionsForFile(valueNode, sourceFile)
		if err != nil {
			return nil, err
		}
		result[keyNode.Value] = value
	}
	return result, nil
}

func decodeSequenceNodeWithYamlFunctions(node *yaml.Node, sourceFile string) (any, error) {
	result := make([]interface{}, 0, len(node.Content))
	for _, child := range node.Content {
		value, err := decodeNodeWithYamlFunctionsForFile(child, sourceFile)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func decodeScalarNodeWithYamlFunctions(node *yaml.Node, sourceFile string) (any, error) {
	if hasCustomTag(node.Tag) {
		return processScalarNodeValueForFile(node, sourceFile)
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

func processScalarNodeValueForFile(node *yaml.Node, sourceFile string) (any, error) {
	if sourceFile == "" || !isIncludeTag(node.Tag) {
		return processScalarNodeValue(node)
	}

	return processIncludeNodeValueForFile(node, sourceFile)
}

func isIncludeTag(tag string) bool {
	return tag == u.AtmosYamlFuncInclude || tag == u.AtmosYamlFuncIncludeRaw
}

func processIncludeNodeValueForFile(node *yaml.Node, sourceFile string) (any, error) {
	resolved := legacyyaml.Node{
		Kind:  legacyyaml.ScalarNode,
		Tag:   node.Tag,
		Value: node.Value,
	}
	basePath := includeBasePathForSourceFile(sourceFile)
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:         basePath,
		BasePathAbsolute: basePath,
	}
	var err error
	if node.Tag == u.AtmosYamlFuncIncludeRaw {
		err = u.ProcessIncludeRawTag(atmosConfig, &resolved, node.Value, sourceFile)
	} else {
		err = u.ProcessIncludeTag(atmosConfig, &resolved, node.Value, sourceFile)
	}
	if err != nil {
		return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
	}
	var value any
	if err := resolved.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func includeBasePathForSourceFile(sourceFile string) string {
	dir := filepath.Dir(sourceFile)
	if base := filepath.Base(dir); base == ".atmos.d" || base == "atmos.d" {
		return filepath.Dir(dir)
	}
	return dir
}

// processScalarNode processes a YAML scalar node tagged with an Atmos custom function and stores the resolved value in v.
// It dispatches handling for atmos.yaml-supported YAML tags to their respective handlers.
// If the node has no tag or a native YAML tag, the function is a no-op.
// It returns any error produced by the invoked handler.
func processScalarNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	if node.Tag == "" {
		return nil
	}

	if isStandardYAMLTag(node.Tag) {
		return nil
	}

	switch node.Tag {
	case u.AtmosYamlFuncUnset:
		// The !unset tag is handled in processMappingNode by skipping the key.
		// If we reach here, it means !unset was used in a context where it can't
		// prevent the key from being added (e.g., scalar value context).
		// In this case, we simply don't set any value and clear the tag.
		log.Debug("Unsetting configuration key", "path", currentPath)
		node.Tag = "" // Avoid re-processing.
		return nil
	case u.AtmosYamlFuncEnv:
		return handleEnv(node, v, currentPath)
	case u.AtmosYamlFuncExec:
		return handleExec(node, v, currentPath)
	case u.AtmosYamlFuncInclude, u.AtmosYamlFuncIncludeRaw:
		return handleInclude(node, v, currentPath)
	case u.AtmosYamlFuncGitRoot, u.AtmosYamlFuncGitRootAlias:
		return handleGitRoot(node, v, currentPath)
	case u.AtmosYamlFuncGitSha, u.AtmosYamlFuncGitRef:
		return handleGitSha(node, v, currentPath)
	case u.AtmosYamlFuncGitBranch:
		return handleGitBranch(node, v, currentPath)
	case u.AtmosYamlFuncGitRepository:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagRepository)
	case u.AtmosYamlFuncGitOwner:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagOwner)
	case u.AtmosYamlFuncGitName:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagName)
	case u.AtmosYamlFuncGitHost:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagHost)
	case u.AtmosYamlFuncGitUrl:
		return handleGitRepoInfo(node, v, currentPath, atmosGit.ProcessTagURL)
	case u.AtmosYamlFuncCwd:
		return handleCwd(node, v, currentPath)
	case u.AtmosYamlFuncRandom:
		return handleRandom(node, v, currentPath)
	default:
		return unsupportedAtmosYamlTagError(node.Tag, currentPath)
	}
}

func unsupportedAtmosYamlTagError(tag, currentPath string) error {
	supportedTags := strings.Join(fntag.AtmosConfigYAML(), ", ")
	if currentPath == "" {
		return fmt.Errorf("%w: '%s'. Supported tags for atmos.yaml are: %s",
			errUtils.ErrUnsupportedYamlTag, tag, supportedTags)
	}
	return fmt.Errorf("%w: '%s' at path '%s'. Supported tags for atmos.yaml are: %s",
		errUtils.ErrUnsupportedYamlTag, tag, currentPath, supportedTags)
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
			if err := node.Encode(data); err != nil {
				return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
			}
		} else {
			log.Warn(
				"Invalid value returned from the YAML function",
				functionKey, strFunc,
				"value", includeValue,
			)
		}
	} else {
		log.Debug(emptyValueWarning, functionKey, strFunc)
		node.Tag = "" // Avoid re-processing
	}
	return nil
}

// handleCwd evaluates a `!cwd` YAML tag and stores the current working directory string into Viper at the given path.
// If a path argument is provided, it is joined with CWD.
// If evaluation fails, it returns an error wrapped with ErrExecuteYamlFunctions.
func handleCwd(node *yaml.Node, v *viper.Viper, currentPath string) error {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)
	cwdValue, err := u.ProcessTagCwd(strFunc)
	if err != nil {
		log.Debug(failedToProcess, functionKey, strFunc, "error", err)
		return fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncCwd, node.Value, err)
	}
	cwdValue = strings.TrimSpace(cwdValue)
	if cwdValue == "" {
		log.Debug(emptyValueWarning, functionKey, strFunc)
	}
	// Set the value in Viper.
	v.Set(currentPath, cwdValue)
	node.Tag = "" // Avoid re-processing.
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
