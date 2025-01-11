package utils

import (
	"os"

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
	PrintMessage(y)
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

func processCustomTags(node *yaml.Node) error {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return processCustomTags(node.Content[0])
	}

	for i := 0; i < len(node.Content); i++ {
		n := node.Content[i]

		if SliceContainsString(AtmosYamlTags, n.Tag) {
			n.Value = n.Tag + " " + n.Value
		}

		if err := processCustomTags(n); err != nil {
			return err
		}
	}
	return nil
}

func UnmarshalYAML[T any](input string) (T, error) {
	var zeroValue T
	var node yaml.Node
	b := []byte(input)

	// Unmarshal into yaml.Node
	if err := yaml.Unmarshal(b, &node); err != nil {
		return zeroValue, err
	}

	if err := processCustomTags(&node); err != nil {
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
