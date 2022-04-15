package utils

import (
	"fmt"
	jsoniter "github.com/json-iterator/go"
	"io/ioutil"
	"os"
	"strings"
)

// PrintAsJSON prints the provided value as JSON document to the console
func PrintAsJSON(data interface{}) error {
	var json = jsoniter.ConfigDefault
	j, err := json.MarshalIndent(data, "", strings.Repeat(" ", 2))
	if err != nil {
		return err
	}
	fmt.Println(string(j))
	return nil
}

// WriteToFileAsJSON converts the provided value to JSON and writes it to the provided file
func WriteToFileAsJSON(filePath string, data interface{}, fileMode os.FileMode) error {
	var json = jsoniter.ConfigDefault
	j, err := json.MarshalIndent(data, "", strings.Repeat(" ", 2))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filePath, j, fileMode)
	if err != nil {
		return err
	}
	return nil
}

// ConvertToJSONString converts the provided value to JSON string
func ConvertToJSONString(data interface{}) (string, error) {
	var json = jsoniter.ConfigDefault
	j, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(j), nil
}

// ConvertToJSONBytes converts the provided value to JSON-encoded slice of bytes
func ConvertToJSONBytes(data interface{}) ([]byte, error) {
	var json = jsoniter.ConfigDefault
	j, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return j, nil
}
