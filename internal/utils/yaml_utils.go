package utils

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
)

// PrintAsYAML prints the provided value as YAML document to the console
func PrintAsYAML(data interface{}) error {
	y, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Println(string(y))
	return nil
}

// WriteToFileAsYAML converts the provided value to YAML and writes it to the provided file
func WriteToFileAsYAML(filePath string, data interface{}, fileMode os.FileMode) error {
	y, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filePath, y, fileMode)
	if err != nil {
		return err
	}
	return nil
}
