package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	jsoniter "github.com/json-iterator/go"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// PrintAsJSON prints the provided value as a JSON document to the console with syntax highlighting.
// Use PrintAsJSONSimple for non-TTY output (pipes, redirects) to avoid expensive highlighting.
func PrintAsJSON(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsJSON")()

	highlighted, err := GetHighlightedJSON(atmosConfig, data)
	if err != nil {
		return err
	}
	PrintMessage(highlighted)
	return nil
}

// PrintAsJSONSimple prints the provided value as JSON document without syntax highlighting.
// This is a fast-path for non-TTY output (files, pipes, redirects) that skips expensive
// syntax highlighting, reducing output time significantly for large configurations.
func PrintAsJSONSimple(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsJSONSimple")()

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

func GetHighlightedJSON(atmosConfig *schema.AtmosConfiguration, data any) (string, error) {
	defer perf.Track(atmosConfig, "utils.GetHighlightedJSON")()

	j, err := ConvertToJSON(data)
	if err != nil {
		return "", err
	}

	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, []byte(j), "", "  ")
	if err != nil {
		return "", err
	}
	highlighted, err := HighlightCodeWithConfig(atmosConfig, prettyJSON.String())
	if err != nil {
		return prettyJSON.String(), nil
	}
	return highlighted, nil
}

func GetAtmosConfigJSON(atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "utils.GetAtmosConfigJSON")()

	j, err := ConvertToJSON(atmosConfig)
	if err != nil {
		return "", err
	}

	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, []byte(j), "", "  ")
	if err != nil {
		return "", err
	}

	highlighted, err := HighlightCodeWithConfig(atmosConfig, prettyJSON.String())
	if err == nil {
		return highlighted, nil
	}
	// Fallback to plain text if highlighting fails
	return prettyJSON.String(), nil
}

// PrintAsJSONToFileDescriptor prints the provided value as JSON document to a file descriptor
func PrintAsJSONToFileDescriptor(atmosConfig schema.AtmosConfiguration, data any) error {
	defer perf.Track(&atmosConfig, "utils.PrintAsJSONToFileDescriptor")()

	j, err := ConvertToJSON(data)
	if err != nil {
		return err
	}
	fmt.Println(j)
	return nil
}

// WriteToFileAsJSON converts the provided value to JSON and writes it to the specified file
func WriteToFileAsJSON(filePath string, data any, fileMode os.FileMode) error {
	defer perf.Track(nil, "utils.WriteToFileAsJSON")()

	j, err := ConvertToJSON(data)
	if err != nil {
		return err
	}

	// Convert data to indented JSON
	indentedJSON, err := json.MarshalIndent(json.RawMessage(j), "", "  ")
	if err != nil {
		return err
	}

	const newlineByte = '\n'

	// Ensure that the JSON content ends with a newline
	if len(indentedJSON) == 0 || indentedJSON[len(indentedJSON)-1] != newlineByte {
		indentedJSON = append(indentedJSON, newlineByte)
	}

	err = os.WriteFile(filePath, indentedJSON, fileMode)
	if err != nil {
		return err
	}
	return nil
}

// ConvertToJSON converts the provided value to a JSON-encoded string
func ConvertToJSON(data any) (string, error) {
	defer perf.Track(nil, "utils.ConvertToJSON")()

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
	defer perf.Track(nil, "utils.ConvertToJSONFast")()

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
	defer perf.Track(nil, "utils.ConvertFromJSON")()

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
	defer perf.Track(nil, "utils.JSONToMapOfInterfaces")()

	var data schema.AtmosSectionMapType
	byt := []byte(input)

	if err := json.Unmarshal(byt, &data); err != nil {
		return nil, err
	}
	return data, nil
}
