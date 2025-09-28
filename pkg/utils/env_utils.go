package utils

import (
	"fmt"
	"os"
	"strings"
)

// ConvertEnvVars converts ENV vars from a map to a list of strings in the format ["key1=val1", "key2=val2", "key3=val3" ...].
// Values are converted to strings using Go's standard string representation.
// Nil values and the literal string "null" are excluded from the output.
func ConvertEnvVars(envVarsMap map[string]any) []string {
	if envVarsMap == nil {
		return []string{}
	}

	// Pre-allocate slice with estimated capacity to reduce allocations
	res := make([]string, 0, len(envVarsMap))

	for k, v := range envVarsMap {
		if v != "null" && v != nil {
			// Use %v for proper type conversion instead of %s.
			res = append(res, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return res
}

// EnvironToMap converts all the environment variables in the environment into a map of strings.
// Invalid environment variable entries (those without '=' separator) are skipped.
func EnvironToMap() map[string]string {
	envVars := os.Environ()
	// Pre-allocate map with capacity to reduce allocations.
	envMap := make(map[string]string, len(envVars))

	for _, env := range envVars {
		pair := SplitStringAtFirstOccurrence(env, "=")
		// Only add valid environment variables that have a non-empty key
		// (SplitStringAtFirstOccurrence always returns [2]string, but second element
		// is empty string if no separator found).
		if pair[0] != "" && strings.Contains(env, "=") {
			envMap[pair[0]] = pair[1]
		}
		// Skip malformed environment variables (no '=' or empty key).
	}
	return envMap
}
