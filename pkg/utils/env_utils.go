package utils

import (
	"fmt"
	"os"
)

// ConvertEnvVars converts ENV vars from a map to a list of strings in the format ["key1=val1", "key2=val2", "key3=val3" ...].
func ConvertEnvVars(envVarsMap map[string]any) []string {
	res := []string{}

	for k, v := range envVarsMap {
		if v != "null" && v != nil {
			res = append(res, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return res
}

// EnvironToMap converts all the environment variables in the environment into a map of strings.
func EnvironToMap() map[string]string {
	envMap := make(map[string]string)

	for _, env := range os.Environ() {
		pair := SplitStringAtFirstOccurrence(env, "=")
		k := pair[0]
		v := pair[1]
		envMap[k] = v
	}

	return envMap
}
