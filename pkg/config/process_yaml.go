package config

import (
	_ "embed"
	"fmt"
	"strings"

	log "github.com/charmbracelet/log"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const valueKey = "value"

// PreprocessYAML processes the given YAML content, replacing specific directives
// (such as !env) with their corresponding values .
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

	if err := processChildren(node.Content, v, currentPath); err != nil {
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

func processChildren(children []*yaml.Node, v *viper.Viper, currentPath string) error {
	for _, child := range children {
		if err := processNode(child, v, currentPath); err != nil {
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
		envValue, err := u.ProcessTagEnv(fmt.Sprintf("%s %s", node.Tag, node.Value))
		if err != nil {
			log.Debug("failed to process", "tag", node.Tag, valueKey, node.Value, "error", err)
			return fmt.Errorf("%w %v %v error %v", ErrExecuteYamlFunctions, u.AtmosYamlFuncEnv, node.Value, err)
		}
		envValue = strings.TrimSpace(envValue)
		if envValue == "" {
			log.Warn("execute returned empty value", "function", u.AtmosYamlFuncEnv, valueKey, node.Value)
		}
		node.Value = envValue
		v.Set(currentPath, node.Value)
		node.Tag = "" // Avoid re-processing  .

	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncExec):
		execValue, err := u.ProcessTagExec(fmt.Sprintf("%s %s", node.Tag, node.Value))
		if err != nil {
			log.Debug("failed to process", "tag", node.Tag, valueKey, node.Value, "error", err)
			return fmt.Errorf("%w %v %v error %v", ErrExecuteYamlFunctions, u.AtmosYamlFuncExec, node.Value, err)
		}
		if execValue != nil {
			v.Set(currentPath, execValue)
		} else {
			log.Warn("execute returned empty value", "function", node.Tag, valueKey, node.Value)
		}

		node.Tag = "" // Avoid re-processing  .

	case strings.HasPrefix(node.Tag, u.AtmosYamlFuncInclude):

		includeValue, err := u.UnmarshalYAML[map[any]any](fmt.Sprintf("%s: %s %s", "include_data", node.Tag, node.Value))
		if err != nil {
			log.Debug("failed to process", "tag", node.Tag, valueKey, node.Value, "error", err)
			return fmt.Errorf("%w %v %v error %v", ErrExecuteYamlFunctions, u.AtmosYamlFuncInclude, node.Value, err)
		}
		if includeValue != nil {
			data, ok := includeValue["include_data"]
			if ok {
				v.Set(currentPath, data)
			} else {
				log.Warn("invalid value returned from execute Yaml function",
					"function", fmt.Sprintf("%s %s", node.Tag, node.Value),
					"value", includeValue,
				)
			}
		} else {
			log.Warn("execute returned empty value", "function", node.Tag, valueKey, node.Value)
		}
		node.Tag = "" // Avoid re-processing  .
	}

	return nil
}
