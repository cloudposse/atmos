package utils

import (
	"fmt"
)

// AppendJSONPathKey appends a key to an existing JSONPath.
// Examples:
//
//	AppendJSONPathKey("vars", "name") -> "vars.name"
//	AppendJSONPathKey("", "vars") -> "vars"
func AppendJSONPathKey(basePath, key string) string {
	if basePath == "" {
		return key
	}
	if key == "" {
		return basePath
	}
	return basePath + "." + key
}

// AppendJSONPathIndex appends an array index to an existing JSONPath.
// Examples:
//
//	AppendJSONPathIndex("vars.zones", 0) -> "vars.zones[0]"
//	AppendJSONPathIndex("", 0) -> "[0]"
func AppendJSONPathIndex(basePath string, index int) string {
	indexStr := fmt.Sprintf("[%d]", index)
	if basePath == "" {
		return indexStr
	}
	return basePath + indexStr
}
