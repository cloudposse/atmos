package utils

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"os"
	"strings"
)

// PrintAsJSON prints the provided value as YAML document to the console
func PrintAsJSON(data any) error {
	j, err := ConvertToJSON(data)
	if err != nil {
		return err
	}
	fmt.Println(j)
	return nil
}

// WriteToFileAsJSON converts the provided value to YAML and writes it to the specified file
func WriteToFileAsJSON(filePath string, data any, fileMode os.FileMode) error {
	j, err := ConvertToJSON(data)
	if err != nil {
		return err
	}
	err = os.WriteFile(filePath, []byte(j), fileMode)
	if err != nil {
		return err
	}
	return nil
}

// ConvertToJSON converts the provided value to a JSON-encoded string
func ConvertToJSON(data any) (string, error) {
	// ConfigCompatibleWithStandardLibrary will sort the map keys in the alphabetical order
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	j, err := json.MarshalIndent(data, "", strings.Repeat(" ", 2))
	if err != nil {
		return "", err
	}
	return string(j), nil
}
