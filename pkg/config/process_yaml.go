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
// - currentPath: The hierarchical key path used to track nested YAML structures.
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

func processMappingNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		newPath := keyNode.Value

		if currentPath != "" {
			newPath = currentPath + "." + newPath
		}

		if err := processNode(valueNode, v, newPath); err != nil {
			return err
		}
	}
	return nil
}

func processSequenceNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	// Check if any child in the sequence has a custom tag that needs processing.
	needsProcessing := false
	for _, child := range node.Content {
		if child.Kind == yaml.ScalarNode && hasCustomTag(child.Tag) {
			needsProcessing = true
			break
		}
		if child.Kind == yaml.MappingNode && containsCustomTags(child) {
			needsProcessing = true
			break
		}
	}

	// If no custom tags, skip processing and let normal YAML handling work.
	if !needsProcessing {
		return nil
	}

	// Collect all processed values from the sequence.
	var values []any

	for idx, child := range node.Content {
		// Build the path for this sequence element.
		elementPath := fmt.Sprintf("%s[%d]", currentPath, idx)

		// Handle different node types in the sequence.
		if child.Kind == yaml.ScalarNode && child.Tag != "" {
			// Scalar with a tag: process the tag and get the value.
			value, err := processScalarNodeValue(child)
			if err != nil {
				return err
			}
			values = append(values, value)
			// Also set the individual element for path-based access.
			v.Set(elementPath, value)
			// Clear the tag to avoid re-processing.
			child.Tag = ""
		} else if child.Kind == yaml.MappingNode {
			// Nested mapping: process recursively to enable path-based access.
			if err := processMappingNode(child, v, elementPath); err != nil {
				return err
			}
			// Decode the mapping for the slice.
			var val any
			if err := child.Decode(&val); err != nil {
				return err
			}
			values = append(values, val)
		} else {
			// Other types: decode normally.
			var val any
			if err := child.Decode(&val); err != nil {
				return err
			}
			values = append(values, val)
			// Set the individual element.
			v.Set(elementPath, val)
		}
	}

	// Set the complete sequence in Viper.
	if len(values) > 0 && currentPath != "" {
		v.Set(currentPath, values)
	}

	return nil
}

// hasCustomTag checks if a tag is a custom Atmos YAML function tag.
func hasCustomTag(tag string) bool {
	return strings.HasPrefix(tag, u.AtmosYamlFuncEnv) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncExec) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncInclude) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncGitRoot) ||
		strings.HasPrefix(tag, u.AtmosYamlFuncRandom)
}

// containsCustomTags recursively checks if a node or its descendants contain custom tags.
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

// processScalarNodeValue processes a scalar node with a tag and returns its value.
func processScalarNodeValue(node *yaml.Node) (any, error) {
	strFunc := fmt.Sprintf(tagValueFormat, node.Tag, node.Value)

	switch {
	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncEnv):
		envValue, err := u.ProcessTagEnv(strFunc)
		if err != nil {
			log.Debug(failedToProcess, functionKey, strFunc, "error", err)
			return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncEnv, node.Value, err)
		}
		return strings.TrimSpace(envValue), nil

	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncExec):
		execValue, err := u.ProcessTagExec(strFunc)
		if err != nil {
			log.Debug(failedToProcess, functionKey, strFunc, "error", err)
			return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncExec, node.Value, err)
		}
		return execValue, nil

	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncInclude):
		includeValue, err := u.UnmarshalYAML[map[any]any](fmt.Sprintf("%s: %s %s", "include_data", node.Tag, node.Value))
		if err != nil {
			log.Debug(failedToProcess, functionKey, strFunc, "error", err)
			return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
		}
		if includeValue != nil {
			if data, ok := includeValue["include_data"]; ok {
				return data, nil
			}
		}
		return nil, nil

	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncGitRoot):
		gitRootValue, err := u.ProcessTagGitRoot(strFunc)
		if err != nil {
			log.Debug(failedToProcess, functionKey, strFunc, "error", err)
			return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncGitRoot, node.Value, err)
		}
		return strings.TrimSpace(gitRootValue), nil

	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncRandom):
		randomValue, err := u.ProcessTagRandom(strFunc)
		if err != nil {
			log.Debug(failedToProcess, functionKey, strFunc, "error", err)
			return nil, fmt.Errorf(errorFormat, ErrExecuteYamlFunctions, u.AtmosYamlFuncRandom, node.Value, err)
		}
		return randomValue, nil

	default:
		// Unknown tag, decode as-is.
		var val any
		if err := node.Decode(&val); err != nil {
			return nil, err
		}
		return val, nil
	}
}

func processScalarNode(node *yaml.Node, v *viper.Viper, currentPath string) error {
	if node.Tag == "" {
		return nil
	}

	switch {
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
	envValue, err := u.ProcessTagEnv(strFunc)
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

// handleGitRoot processes a YAML node with an !repo-root tag and sets the value in Viper, returns an error if the processing fails, warns if the value is empty.
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

// handleRandom processes a YAML node with an !random tag and sets the value in Viper.
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
