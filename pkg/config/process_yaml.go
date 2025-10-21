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

	if node.Kind == yaml.MappingNode {
		if err := processMappingNode(node, v, currentPath); err != nil {
			return err
		}
	}

	if node.Kind == yaml.ScalarNode && node.Tag != "" {
		if err := processScalarNode(node, v, currentPath); err != nil {
			return err
		}
	}

	// Determine if parent is a sequence to correctly index children
	parentIsSeq := node.Kind == yaml.SequenceNode
	if err := processChildren(node.Content, v, currentPath, parentIsSeq); err != nil {
		return err
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

func processChildren(children []*yaml.Node, v *viper.Viper, currentPath string, parentIsSeq bool) error {
	for idx, child := range children {
		newPath := currentPath
		if parentIsSeq {
			newPath = fmt.Sprintf("%s[%d]", currentPath, idx)
		}
		if err := processNode(child, v, newPath); err != nil {
			return err
		}
	}
	return nil
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
		log.Warn(emptyValueWarning, functionKey, strFunc)
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
		log.Warn(emptyValueWarning, functionKey, strFunc)
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
		log.Warn(emptyValueWarning, functionKey, strFunc)
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
		log.Warn(emptyValueWarning, functionKey, strFunc)
	}
	// Set the value in Viper .
	v.Set(currentPath, gitRootValue)
	node.Tag = "" // Avoid re-processing .
	return nil
}
