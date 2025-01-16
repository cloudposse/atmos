package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Atmos YAML functions
	AtmosYamlFuncExec            = "!exec"
	AtmosYamlFuncStore           = "!store"
	AtmosYamlFuncTemplate        = "!template"
	AtmosYamlFuncTerraformOutput = "!terraform.output"
	AtmosYamlFuncEnv             = "!env"
	AtmosYamlFuncInclude         = "!include"
)

var (
	AtmosYamlTags = []string{
		AtmosYamlFuncExec,
		AtmosYamlFuncStore,
		AtmosYamlFuncTemplate,
		AtmosYamlFuncTerraformOutput,
		AtmosYamlFuncEnv,
		AtmosYamlFuncInclude,
	}
)

// PrintAsYAML prints the provided value as YAML document to the console
func PrintAsYAML(data any) error {
	y, err := ConvertToYAML(data)
	if err != nil {
		return err
	}

	atmosConfig := ExtractAtmosConfig(data)
	highlighted, err := HighlightCodeWithConfig(y, atmosConfig)
	if err != nil {
		// Fallback to plain text if highlighting fails
		PrintMessage(y)
		return nil
	}
	PrintMessage(highlighted)
	return nil
}

// PrintAsYAMLToFileDescriptor prints the provided value as YAML document to a file descriptor
func PrintAsYAMLToFileDescriptor(atmosConfig schema.AtmosConfiguration, data any) error {
	y, err := ConvertToYAML(data)
	if err != nil {
		return err
	}
	LogInfo(atmosConfig, y)
	return nil
}

// WriteToFileAsYAML converts the provided value to YAML and writes it to the specified file
func WriteToFileAsYAML(filePath string, data any, fileMode os.FileMode) error {
	y, err := ConvertToYAML(data)
	if err != nil {
		return err
	}
	err = os.WriteFile(filePath, []byte(y), fileMode)
	if err != nil {
		return err
	}
	return nil
}

// ConvertToYAML converts the provided data to a YAML string
func ConvertToYAML(data any) (string, error) {
	y, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(y), nil
}

func processCustomTags(atmosConfig *schema.AtmosConfiguration, node *yaml.Node, file string) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return processCustomTags(atmosConfig, node.Content[0], file)
	}

	for i := 0; i < len(node.Content); i++ {
		n := node.Content[i]

		if SliceContainsString(AtmosYamlTags, n.Tag) {
			val, err := getValueWithTag(atmosConfig, n, file)
			if err != nil {
				return err
			}
			n.Value = val
		}

		if err := processCustomTags(atmosConfig, n, file); err != nil {
			return err
		}
	}
	return nil
}

func getValueWithTag(atmosConfig *schema.AtmosConfiguration, n *yaml.Node, file string) (string, error) {
	val := n.Value
	tag := n.Tag

	if tag == AtmosYamlFuncInclude {
		// If absolute path is provided, check if the file exists
		if filepath.IsAbs(val) {
			if !FileExists(val) {
				return "", fmt.Errorf("the function '!include %s' points to a file that does not exist", val)
			}
			return tag + " " + val, nil
		}

		// Detect relative paths (relative to the manifest file) and convert to absolute paths
		if strings.HasPrefix(val, "./") || strings.HasPrefix(val, "../") {
			resolved := ResolveRelativePath(val, file)
			if !FileExists(resolved) {
				return "", fmt.Errorf("the function '!include %s' points to a file that does not exist", val)
			}
			return tag + " " + resolved, nil
		}

		// Check if the `!include` function points to an Atmos stack manifest relative to the `base_path` defined in `atmos.yaml`
		atmosManifestPath := filepath.Join(atmosConfig.BasePath, val)
		if FileExists(atmosManifestPath) {
			atmosManifestAbsolutePath, err := filepath.Abs(atmosManifestPath)
			if err != nil {
				return "", fmt.Errorf("error converting the file path to an ansolute path in the function '!include %s': %v", val, err)
			}
			return tag + " " + atmosManifestAbsolutePath, nil
		}
	}

	// Return it to be processed by `go-getter`
	return tag + " " + val, nil
}

func UnmarshalYAML[T any](input string) (T, error) {
	return UnmarshalYAMLFromFile[T](&schema.AtmosConfiguration{}, input, "")
}

func UnmarshalYAMLFromFile[T any](atmosConfig *schema.AtmosConfiguration, input string, file string) (T, error) {
	var zeroValue T
	var node yaml.Node
	b := []byte(input)

	// Unmarshal into yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return zeroValue, err
	}

	if err := processCustomTags(atmosConfig, &node, file); err != nil {
		return zeroValue, err
	}

	// Decode the yaml.Node into the desired type T
	var data T
	if err := node.Decode(&data); err != nil {
		return zeroValue, err
	}

	return data, nil
}

// IsYAML checks if data is in YAML format
func IsYAML(data string) bool {
	var yml any
	return yaml.Unmarshal([]byte(data), &yml) == nil
}
