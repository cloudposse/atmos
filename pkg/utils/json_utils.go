package utils

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"os"
	"strings"

	jsoniter "github.com/json-iterator/go"
)

// PrintAsJSON prints the provided value as JSON document to the console
func PrintAsJSON(data any) error {
	j, err := ConvertToJSON(data)
	if err != nil {
		return err
	}
	PrintMessage(j)
	return nil
}

// PrintAsJSONToFileDescriptor prints the provided value as JSON document to a file descriptor
func PrintAsJSONToFileDescriptor(cliConfig schema.CliConfiguration, data any) error {
	j, err := ConvertToJSON(data)
	if err != nil {
		return err
	}
	LogInfo(cliConfig, j)
	return nil
}

// WriteToFileAsJSON converts the provided value to JSON and writes it to the specified file
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
	var json = jsoniter.Config{
		EscapeHTML:                    true,
		ObjectFieldMustBeSimpleString: false,
		SortMapKeys:                   true,
		ValidateJsonRawMessage:        true,
	}

	j, err := json.Froze().MarshalIndent(data, "", strings.Repeat(" ", 3))
	if err != nil {
		return "", err
	}
	return string(j), nil
}

// ConvertToJSONFast converts the provided value to a JSON-encoded string using 'ConfigFastest' config and json.Marshal without indents
func ConvertToJSONFast(data any) (string, error) {
	var json = jsoniter.Config{
		EscapeHTML:                    false,
		MarshalFloatWith6Digits:       true,
		ObjectFieldMustBeSimpleString: true,
		SortMapKeys:                   true,
		ValidateJsonRawMessage:        true,
	}

	j, err := json.Froze().MarshalToString(data)
	if err != nil {
		return "", err
	}
	return j, nil
}

// ConvertFromJSON converts the provided JSON-encoded string to Go data types
func ConvertFromJSON(jsonString string) (any, error) {
	var json = jsoniter.Config{
		EscapeHTML:                    false,
		MarshalFloatWith6Digits:       true,
		ObjectFieldMustBeSimpleString: true,
		SortMapKeys:                   true,
		ValidateJsonRawMessage:        true,
	}

	var data any
	err := json.Froze().Unmarshal([]byte(jsonString), &data)
	if err != nil {
		return "", err
	}
	return data, nil
}
