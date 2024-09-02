package utils

import (
	"os"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
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
func PrintAsYAMLToFileDescriptor(cliConfig schema.CliConfiguration, data any) error {
	y, err := ConvertToYAML(data)
	if err != nil {
		return err
	}
	LogInfo(cliConfig, y)
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

// YAMLToMapOfStrings takes a YAML string as input and unmarshals it into a map[string]any
func YAMLToMapOfStrings(input string) (schema.AtmosSectionMapType, error) {
	var data schema.AtmosSectionMapType
	b := []byte(input)

	if err := yaml.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// UnmarshalYAML takes a YAML string as input and unmarshals it into a Go type
func UnmarshalYAML[T any](input string) (T, error) {
	var data T
	b := []byte(input)

	if err := yaml.Unmarshal(b, &data); err != nil {
		return data, err
	}
	return data, nil
}
