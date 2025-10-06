package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// BuildJSONPath constructs a JSONPath-style path from components.
// Examples:
//
//	BuildJSONPath("vars", "name") -> "vars.name"
//	BuildJSONPath("vars", "tags", "environment") -> "vars.tags.environment"
//	BuildJSONPath("import", "[0]") -> "import[0]"
func BuildJSONPath(components ...string) string {
	if len(components) == 0 {
		return ""
	}

	// Filter out empty components.
	filtered := make([]string, 0, len(components))
	for _, comp := range components {
		if comp != "" {
			filtered = append(filtered, comp)
		}
	}

	// Build the path, skipping dots before array indices.
	var sb strings.Builder
	for _, comp := range filtered {
		if sb.Len() > 0 {
			// Don't add a dot before array index components.
			if strings.HasPrefix(comp, "[") {
				sb.WriteString(comp)
				continue
			}
			sb.WriteString(".")
		}
		sb.WriteString(comp)
	}

	return sb.String()
}

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

// SplitJSONPath splits a JSONPath into its components.
// Examples:
//
//	SplitJSONPath("vars.name") -> ["vars", "name"]
//	SplitJSONPath("vars.zones[0]") -> ["vars", "zones", "[0]"]
//	SplitJSONPath("vars.tags.environment") -> ["vars", "tags", "environment"]
func SplitJSONPath(path string) []string {
	if path == "" {
		return []string{}
	}

	// Handle array indices: "vars.zones[0]" -> "vars", "zones", "[0]"
	parts := []string{}
	current := ""

	for i := 0; i < len(path); i++ {
		ch := path[i]

		switch ch {
		case '.':
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		case '[':
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
			// Find the closing bracket.
			end := strings.Index(path[i:], "]")
			if end != -1 {
				parts = append(parts, path[i:i+end+1])
				i += end
			} else {
				// Malformed path, just add the rest.
				current += string(ch)
			}
		default:
			current += string(ch)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// ParseJSONPathIndex extracts the index from an array index component.
// Examples:
//
//	ParseJSONPathIndex("[0]") -> 0, true
//	ParseJSONPathIndex("[42]") -> 42, true
//	ParseJSONPathIndex("name") -> 0, false
func ParseJSONPathIndex(component string) (int, bool) {
	if !strings.HasPrefix(component, "[") || !strings.HasSuffix(component, "]") {
		return 0, false
	}

	indexStr := component[1 : len(component)-1]
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return 0, false
	}

	return index, true
}

// IsJSONPathIndex checks if a component is an array index.
// Examples:
//
//	IsJSONPathIndex("[0]") -> true
//	IsJSONPathIndex("name") -> false
func IsJSONPathIndex(component string) bool {
	_, ok := ParseJSONPathIndex(component)
	return ok
}

// GetJSONPathParent returns the parent path of a JSONPath.
// Examples:
//
//	GetJSONPathParent("vars.name") -> "vars"
//	GetJSONPathParent("vars.zones[0]") -> "vars.zones"
//	GetJSONPathParent("vars") -> ""
func GetJSONPathParent(path string) string {
	if path == "" {
		return ""
	}

	// Handle array indices.
	if strings.HasSuffix(path, "]") {
		// Find the opening bracket.
		idx := strings.LastIndex(path, "[")
		if idx != -1 {
			return path[:idx]
		}
	}

	// Handle regular keys.
	idx := strings.LastIndex(path, ".")
	if idx != -1 {
		return path[:idx]
	}

	return ""
}

// GetJSONPathLeaf returns the last component of a JSONPath.
// Examples:
//
//	GetJSONPathLeaf("vars.name") -> "name"
//	GetJSONPathLeaf("vars.zones[0]") -> "[0]"
//	GetJSONPathLeaf("vars") -> "vars"
func GetJSONPathLeaf(path string) string {
	if path == "" {
		return ""
	}

	parts := SplitJSONPath(path)
	if len(parts) == 0 {
		return ""
	}

	return parts[len(parts)-1]
}
