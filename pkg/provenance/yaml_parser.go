package provenance

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	yamlArrayItemPrefix = "- " // YAML array item prefix
)

// normalizeProvenancePath removes common prefixes from provenance paths.
func normalizeProvenancePath(path string) string {
	defer perf.Track(nil, "provenance.normalizeProvenancePath")()

	parts := strings.Split(path, pathSeparator)

	// Remove "components.terraform.<component>." prefix
	if len(parts) >= 3 && parts[0] == "components" && parts[1] == "terraform" {
		// Skip components.terraform.<name> - return rest
		if len(parts) > 3 {
			return strings.Join(parts[3:], pathSeparator)
		}
		return ""
	}

	// Remove "terraform." prefix
	if len(parts) >= 2 && parts[0] == "terraform" {
		return strings.Join(parts[1:], pathSeparator)
	}

	return path
}

// isMultilineScalarIndicator checks if a value indicates a multi-line YAML scalar.
func isMultilineScalarIndicator(value string) bool {
	return value == "|" || value == "|-" || value == ">" || value == ">-"
}

// extractYAMLKey extracts the key from a YAML line, handling array items.
func extractYAMLKey(trimmed string) string {
	parts := strings.SplitN(trimmed, yamlKeySep, 2)
	key := strings.TrimSpace(parts[0])

	// Handle array items like "- key:"
	if strings.HasPrefix(key, yamlArrayItemPrefix) {
		key = strings.TrimPrefix(key, yamlArrayItemPrefix)
		key = strings.TrimSpace(key)
	}

	return key
}

// buildYAMLPath constructs a full YAML path from a stack and new key.
func buildYAMLPath(pathStack []string, key string) string {
	if len(pathStack) > 0 {
		return strings.Join(append(pathStack, key), pathSeparator)
	}
	return key
}

// getArrayIndex returns the array index for the current level.
func getArrayIndex(arrayIndexStack []int) (int, []int) {
	var arrayIndex int

	if len(arrayIndexStack) > 0 {
		arrayIndex = arrayIndexStack[len(arrayIndexStack)-1]
		newStack := make([]int, len(arrayIndexStack))
		copy(newStack, arrayIndexStack)
		newStack[len(newStack)-1]++ // Increment for next element
		return arrayIndex, newStack
	}

	arrayIndex = 0
	newStack := []int{1} // Start at 1 for next element
	return arrayIndex, newStack
}

// popStacksForIndent pops the path, indent, and array index stacks when indentation decreases.
func popStacksForIndent(indent int, pathStack []string, indentStack, arrayIndexStack []int) ([]string, []int, []int) {
	for len(indentStack) > 1 && indent <= indentStack[len(indentStack)-1] {
		pathStack = pathStack[:len(pathStack)-1]
		indentStack = indentStack[:len(indentStack)-1]
		if len(arrayIndexStack) > 0 {
			arrayIndexStack = arrayIndexStack[:len(arrayIndexStack)-1]
		}
	}
	return pathStack, indentStack, arrayIndexStack
}

// handleArrayItemLine processes a simple array item and records it.
func handleArrayItemLine(lineNum int, pathStack []string, arrayIndexStack []int, lineInfo map[int]YAMLLineInfo) []int {
	if len(pathStack) == 0 {
		return arrayIndexStack
	}

	parentKey := pathStack[len(pathStack)-1]
	arrayIndex, newStack := getArrayIndex(arrayIndexStack)

	// Build path: parent[index]
	currentPath := fmt.Sprintf("%s[%d]", parentKey, arrayIndex)

	// Record this line as an array element
	lineInfo[lineNum] = YAMLLineInfo{
		Path:           currentPath,
		IsKeyLine:      true,
		IsContinuation: false,
	}

	return newStack
}

// yamlPathState holds the state returned from handleKeyLine.
type yamlPathState struct {
	pathStack       []string
	indentStack     []int
	arrayIndexStack []int
	multilineStart  bool
	multilinePath   string
}

// handleKeyLineParams contains parameters for handleKeyLine.
type handleKeyLineParams struct {
	lineNum         int
	indent          int
	parts           []string
	trimmed         string
	pathStack       []string
	indentStack     []int
	arrayIndexStack []int
	lineInfo        map[int]YAMLLineInfo
}

// arrayElementPathResult holds the result of building an array element path.
type arrayElementPathResult struct {
	currentPath     string
	pathStack       []string
	arrayIndexStack []int
}

// buildArrayElementPath builds the path for an array-of-maps element.
func buildArrayElementPath(key string, pathStack []string, arrayIndexStack []int) arrayElementPathResult {
	lastElement := pathStack[len(pathStack)-1]

	// Get the array parent by stripping any existing index
	arrayParent := lastElement
	if idx := strings.Index(lastElement, "["); idx > 0 {
		arrayParent = lastElement[:idx]
	}

	// Get the current array index for this parent
	var arrayIndex int
	if len(arrayIndexStack) > 0 {
		arrayIndex = arrayIndexStack[len(arrayIndexStack)-1]
	}

	// Build the indexed parent path (e.g., "items[0]" or "items[1]")
	indexedParent := utils.AppendJSONPathIndex(arrayParent, arrayIndex)

	// Build the full path: grandparent(s) + indexed parent + key
	var pathComponents []string
	if len(pathStack) > 1 {
		pathComponents = append([]string{}, pathStack[:len(pathStack)-1]...)
	}
	pathComponents = append(pathComponents, indexedParent, key)

	// Build new path stack with indexed parent
	newPathStack := append([]string{}, pathStack[:len(pathStack)-1]...)
	newPathStack = append(newPathStack, indexedParent)

	// Increment array index for next element
	newArrayIndexStack := make([]int, len(arrayIndexStack))
	copy(newArrayIndexStack, arrayIndexStack)
	if len(newArrayIndexStack) > 0 {
		newArrayIndexStack[len(newArrayIndexStack)-1]++
	}

	return arrayElementPathResult{
		currentPath:     strings.Join(pathComponents, pathSeparator),
		pathStack:       newPathStack,
		arrayIndexStack: newArrayIndexStack,
	}
}

// handleKeyLine processes a key: value line and updates stacks.
func handleKeyLine(params *handleKeyLineParams) yamlPathState {
	isArrayElement := strings.HasPrefix(params.trimmed, yamlArrayItemPrefix)
	key := extractYAMLKey(params.trimmed)

	var currentPath string
	var newPathStack []string
	var newArrayIndexStack []int

	if isArrayElement && len(params.pathStack) > 0 {
		result := buildArrayElementPath(key, params.pathStack, params.arrayIndexStack)
		currentPath = result.currentPath
		newPathStack = result.pathStack
		newArrayIndexStack = result.arrayIndexStack
	} else {
		currentPath = buildYAMLPath(params.pathStack, key)
		newPathStack = params.pathStack
		newArrayIndexStack = params.arrayIndexStack
	}

	// Determine value type
	value := ""
	if len(params.parts) > 1 {
		value = strings.TrimSpace(params.parts[1])
	}

	// Check for multi-line scalar indicators
	isMultilineStart := isMultilineScalarIndicator(value)

	// Record this line as a key line
	params.lineInfo[params.lineNum] = YAMLLineInfo{
		Path:           currentPath,
		IsKeyLine:      true,
		IsContinuation: false,
	}

	state := yamlPathState{
		pathStack:       newPathStack,
		indentStack:     params.indentStack,
		arrayIndexStack: newArrayIndexStack,
		multilineStart:  isMultilineStart,
		multilinePath:   currentPath,
	}

	// Push to stack if this is a parent key
	if value == "" || value == "{}" || value == "[]" || isMultilineStart {
		state.pathStack = append(state.pathStack, key)
		state.indentStack = append(state.indentStack, params.indent)
		// Reset array index counter for this new parent
		state.arrayIndexStack = append(state.arrayIndexStack, 0)
	}

	return state
}

// yamlLineProcessState holds state for processing a YAML line.
type yamlLineProcessState struct {
	inMultilineValue bool
	multilineIndent  int
	multilinePath    string
	pathStack        []string
	indentStack      []int
	arrayIndexStack  []int
}

// processYAMLLine processes a single YAML line and updates state.
func processYAMLLine(
	lineNum int,
	line string,
	state *yamlLineProcessState,
	lineInfo map[int]YAMLLineInfo,
) {
	plainLine := stripANSI(line)
	indent := len(plainLine) - len(strings.TrimLeft(plainLine, pathSpace))
	trimmed := strings.TrimSpace(plainLine)

	// Skip empty lines or comments
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return
	}

	// Check if we're exiting a multi-line value
	if state.inMultilineValue && indent <= state.multilineIndent {
		state.inMultilineValue = false
	}

	// Handle continuation lines in multi-line values
	if state.inMultilineValue {
		lineInfo[lineNum] = YAMLLineInfo{
			Path:           state.multilinePath,
			IsKeyLine:      false,
			IsContinuation: true,
		}
		return
	}

	// Pop stack for decreased indentation
	state.pathStack, state.indentStack, state.arrayIndexStack = popStacksForIndent(
		indent, state.pathStack, state.indentStack, state.arrayIndexStack,
	)

	// Handle simple array items
	if strings.HasPrefix(trimmed, yamlArrayItemPrefix) && !strings.Contains(trimmed, yamlKeySep) {
		state.arrayIndexStack = handleArrayItemLine(lineNum, state.pathStack, state.arrayIndexStack, lineInfo)
		return
	}

	// Handle key: value lines
	if strings.Contains(trimmed, yamlKeySep) {
		parts := strings.SplitN(trimmed, yamlKeySep, 2)
		keyState := handleKeyLine(&handleKeyLineParams{
			lineNum:         lineNum,
			indent:          indent,
			parts:           parts,
			trimmed:         trimmed,
			pathStack:       state.pathStack,
			indentStack:     state.indentStack,
			arrayIndexStack: state.arrayIndexStack,
			lineInfo:        lineInfo,
		})

		state.pathStack = keyState.pathStack
		state.indentStack = keyState.indentStack
		state.arrayIndexStack = keyState.arrayIndexStack
		state.multilinePath = keyState.multilinePath

		if keyState.multilineStart {
			state.inMultilineValue = true
			state.multilineIndent = indent
		}
	}
}

// buildYAMLPathMap creates a mapping from line numbers to YAML line information.
// It parses YAML line-by-line, tracks nesting, and detects multi-line constructs.
func buildYAMLPathMap(yamlLines []string) map[int]YAMLLineInfo {
	defer perf.Track(nil, "provenance.buildYAMLPathMap")()

	lineInfo := make(map[int]YAMLLineInfo)
	state := &yamlLineProcessState{
		pathStack:       []string{},
		indentStack:     []int{-1},
		arrayIndexStack: []int{},
	}

	for lineNum, line := range yamlLines {
		processYAMLLine(lineNum, line, state, lineInfo)
	}

	return lineInfo
}
