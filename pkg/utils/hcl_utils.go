package utils

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
)

// PrintAsHcl prints the provided value as HCL (HashiCorp Language) document to the console
func PrintAsHcl(data any) error {
	y, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Println(string(y))
	return nil
}

// WriteToFileAsHcl converts the provided value to HCL (HashiCorp Language) and writes it to the provided file
func WriteToFileAsHcl(filePath string, data any, fileMode os.FileMode) error {
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
