package utils

import "fmt"

// ConvertEnvVars convert ENV vars from a map to a list of strings in the format ["key1=val1", "key2=val2", "key3=val3" ...]
func ConvertEnvVars(envVarsMap map[any]any) []string {
	res := []string{}

	for k, v := range envVarsMap {
		if v != "null" && v != nil {
			res = append(res, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return res
}
