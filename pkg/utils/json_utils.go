package utils

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"

	jsoniter "github.com/json-iterator/go"

	"github.com/cloudposse/atmos/pkg/schema"
)

// PrintAsJSON prints the provided value as JSON document to the console
func PrintAsJSON(data any) error {
	j, err := ConvertToJSON(data)
	if err != nil {
		return err
	}

	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, []byte(j), "", "  ")
	if err != nil {
		return err
	}

	PrintMessage(prettyJSON.String())
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

	// Convert data to indented JSON
	indentedJSON, err := json.MarshalIndent(json.RawMessage(j), "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, indentedJSON, fileMode)
	if err != nil {
		return err
	}
	return nil
}

// ConvertToJSON converts the provided value to a JSON-encoded string
func ConvertToJSON(data any) (string, error) {
	jc := jsoniter.Config{
		EscapeHTML:                    true,
		ObjectFieldMustBeSimpleString: false,
		SortMapKeys:                   true,
		ValidateJsonRawMessage:        true,
	}

	j, err := jc.Froze().MarshalIndent(data, "", strings.Repeat(" ", 3))
	if err != nil {
		return "", err
	}
	return string(j), nil
}

// ConvertToJSONFast converts the provided value to a JSON-encoded string using 'ConfigFastest' config and json.Marshal without indents
func ConvertToJSONFast(data any) (string, error) {
	jc := jsoniter.Config{
		EscapeHTML:                    false,
		MarshalFloatWith6Digits:       true,
		ObjectFieldMustBeSimpleString: true,
		SortMapKeys:                   true,
		ValidateJsonRawMessage:        true,
	}

	j, err := jc.Froze().MarshalToString(data)
	if err != nil {
		return "", err
	}
	return j, nil
}

// ConvertFromJSON converts the provided JSON-encoded string to Go data types
func ConvertFromJSON(jsonString string) (any, error) {
	jc := jsoniter.Config{
		EscapeHTML:                    false,
		MarshalFloatWith6Digits:       true,
		ObjectFieldMustBeSimpleString: true,
		SortMapKeys:                   true,
		ValidateJsonRawMessage:        true,
	}

	var data any
	err := jc.Froze().Unmarshal([]byte(jsonString), &data)
	if err != nil {
		return "", err
	}
	return data, nil
}

// JSONToMapOfInterfaces takes a JSON string as input and returns a map[string]any
func JSONToMapOfInterfaces(input string) (schema.AtmosSectionMapType, error) {
	var data schema.AtmosSectionMapType
	byt := []byte(input)

	if err := json.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}
