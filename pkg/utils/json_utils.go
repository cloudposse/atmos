package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"

	jsoniter "github.com/json-iterator/go"

	pkgdata "github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ErrJSONTopLevelNotObject is returned by JSONToMapOfInterfaces when the JSON document
// decodes to a nil map (for example, a top-level `null`), because the result is not an object.
var ErrJSONTopLevelNotObject = errors.New("JSON top-level value is not an object")

// PrintAsJSON prints the provided value as a JSON document to the console with syntax highlighting.
// Use PrintAsJSONSimple for non-TTY output (pipes, redirects) to avoid expensive highlighting.
func PrintAsJSON(atmosConfig *schema.AtmosConfiguration, data any) error {
	defer perf.Track(atmosConfig, "utils.PrintAsJSON")()

	highlighted, err := GetHighlightedJSON(atmosConfig, data)
	if err != nil {
		return err
	}
	return pkgdata.Writeln(highlighted)
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
	return pkgdata.Writeln(prettyJSON.String())
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
		EscapeHTML:                    false,
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

// JSONToMapOfInterfaces takes a JSON-encoded string as input and returns a map of string
// keys to values. Unlike ConvertFromJSON, it always decodes into a map, so it returns an
// error for JSON documents whose top-level value is not an object.
//
// PUBLIC API — DO NOT REMOVE. This function has no callers inside the Atmos repository, so
// dead-code sweeps (for example, `go run golang.org/x/tools/cmd/deadcode@latest -test ./...`)
// will report it as unused. It is retained intentionally because it is part of the public Go
// API consumed by the external `cloudposse/terraform-provider-utils` provider. It was
// previously dropped as "dead" code in PR #2608, which broke that provider; see
// docs/fixes/2026-08-25-restore-public-provider-api-wrappers.md.
func JSONToMapOfInterfaces(input string) (schema.AtmosSectionMapType, error) {
	defer perf.Track(nil, "utils.JSONToMapOfInterfaces")()

	var data schema.AtmosSectionMapType
	if err := json.Unmarshal([]byte(input), &data); err != nil {
		return nil, err
	}
	// A top-level JSON `null` unmarshals into a nil map without error; reject it so callers
	// always receive a non-nil object or an error, matching this function's documented contract.
	if data == nil {
		return nil, ErrJSONTopLevelNotObject
	}
	return data, nil
}
