package utils

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
)

// PrintAsYAML prints the provided value as YAML document to the console
func PrintAsYAML(data any) error {
	y, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Println(string(y))
	return nil
}

// WriteToFileAsYAML converts the provided value to YAML and writes it to the provided file
func WriteToFileAsYAML(filePath string, data any, fileMode os.FileMode) error {
	y, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	err = os.WriteFile(filePath, y, fileMode)
	if err != nil {
		return err
	}
	return nil
}
