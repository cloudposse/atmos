package utils

import (
	"fmt"
	"gopkg.in/yaml.v2"
)

// PrintAsYAML prints the provided value as YAML document to the console
func PrintAsYAML(in interface{}) error {
	y, err := yaml.Marshal(in)
	if err != nil {
		return err
	}
	fmt.Println(string(y))
	return nil
}
