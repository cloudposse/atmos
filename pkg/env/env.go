// Package env provides utilities for working with environment variables.
// It includes functions for converting, merging, updating, and managing
// environment variable slices used throughout atmos.
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// pathPrefixLength is the length of "PATH=" prefix.
	pathPrefixLength = 5
	// envVarFormat is the format string for environment variables.
	envVarFormat = "%s=%s"
)

// ConvertEnvVars converts ENV vars from a map to a list of strings in the format ["key1=val1", "key2=val2", "key3=val3" ...].
// Variables with nil or "null" values are skipped.
func ConvertEnvVars(envVarsMap map[string]any) []string {
	defer perf.Track(nil, "env.ConvertEnvVars")()

	res := []string{}

	for k, v := range envVarsMap {
		if v != "null" && v != nil {
			res = append(res, fmt.Sprintf(envVarFormat, k, fmt.Sprint(v)))
		}
	}
	return res
}

// EnvironToMap converts all the environment variables in the environment into a map of strings.
func EnvironToMap() map[string]string {
	defer perf.Track(nil, "env.EnvironToMap")()

	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		pair := splitStringAtFirstOccurrence(e, "=")
		k := pair[0]
		v := pair[1]
		envMap[k] = v
	}
	return envMap
}

// splitStringAtFirstOccurrence splits a string at the first occurrence of a separator.
// Returns [2]string with the part before and after the separator.
// If separator is not found, returns the original string and empty string.
func splitStringAtFirstOccurrence(s string, sep string) [2]string {
	idx := strings.Index(s, sep)
	if idx == -1 {
		return [2]string{s, ""}
	}
	return [2]string{s[:idx], s[idx+len(sep):]}
}

// CommandEnvToMap converts a slice of schema.CommandEnv to a map[string]string.
// Keys are taken from the Key field; later entries overwrite earlier ones on duplicate keys.
func CommandEnvToMap(envs []schema.CommandEnv) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		m[e.Key] = e.Value
	}
	return m
}

// PrependToPath adds a directory to the beginning of the PATH environment variable.
// Returns the new PATH value.
func PrependToPath(currentPath, newDir string) string {
	defer perf.Track(nil, "env.PrependToPath")()

	if currentPath == "" {
		return newDir
	}
	return fmt.Sprintf("%s%c%s", newDir, os.PathListSeparator, currentPath)
}

// findPathIndex returns the index of the PATH entry (case-insensitive) and the key casing to use ("PATH" or "Path").
func findPathIndex(envSlice []string) (int, string) {
	defer perf.Track(nil, "env.findPathIndex")()

	for i, envVar := range envSlice {
		if len(envVar) >= pathPrefixLength && strings.EqualFold(envVar[:pathPrefixLength], "PATH=") {
			return i, envVar[:pathPrefixLength-1] // keep existing key's casing (exclude "=").
		}
	}
	return -1, "PATH"
}

// UpdateEnvironmentPath updates the PATH in an environment slice.
// Returns a new environment slice with the updated PATH.
func UpdateEnvironmentPath(envSlice []string, newDir string) []string {
	defer perf.Track(nil, "env.UpdateEnvironmentPath")()

	idx, key := findPathIndex(envSlice) // case-insensitive match for "PATH=".
	if idx >= 0 {
		updated := make([]string, 0, len(envSlice))
		for i, envVar := range envSlice {
			if i == idx {
				currentPath := envVar[len(key)+1:] // Remove "KEY=" prefix
				updated = append(updated, fmt.Sprintf(envVarFormat, key, PrependToPath(currentPath, newDir)))
			} else {
				updated = append(updated, envVar)
			}
		}
		return updated
	}
	// If PATH wasn't found, add it with canonical "PATH" key.
	return append(append([]string{}, envSlice...), fmt.Sprintf(envVarFormat, "PATH", newDir))
}

// GetPathFromEnvironment extracts the PATH value from an environment slice.
func GetPathFromEnvironment(envSlice []string) string {
	defer perf.Track(nil, "env.GetPathFromEnvironment")()

	idx, key := findPathIndex(envSlice)
	if idx >= 0 {
		return envSlice[idx][len(key)+1:] // Remove "KEY=" prefix
	}
	return ""
}

// EnsureBinaryInPath checks if a binary directory is in PATH and adds it if missing.
// Returns updated environment with the binary directory prepended to PATH.
func EnsureBinaryInPath(envSlice []string, binaryPath string) []string {
	defer perf.Track(nil, "env.EnsureBinaryInPath")()

	binaryDir := filepath.Dir(binaryPath)
	currentPath := GetPathFromEnvironment(envSlice)

	// Check if binary directory is already in PATH using exact match.
	// We split by path separator to avoid substring false positives
	// (e.g., "/usr/local/b" matching "/usr/local/bin").
	for _, dir := range strings.Split(currentPath, string(os.PathListSeparator)) {
		if dir == binaryDir {
			return envSlice // Already in PATH
		}
	}

	return UpdateEnvironmentPath(envSlice, binaryDir)
}

// UpdateEnvVar updates or adds an environment variable in an environment slice.
// Returns a new environment slice with the variable updated.
func UpdateEnvVar(envSlice []string, key, value string) []string {
	defer perf.Track(nil, "env.UpdateEnvVar")()

	keyPrefix := key + "="

	// Look for existing variable (case-sensitive for non-PATH variables)
	for i, envVar := range envSlice {
		if strings.HasPrefix(envVar, keyPrefix) {
			// Update existing variable
			updated := make([]string, 0, len(envSlice))
			for j, e := range envSlice {
				if j == i {
					updated = append(updated, fmt.Sprintf(envVarFormat, key, value))
				} else {
					updated = append(updated, e)
				}
			}
			return updated
		}
	}

	// Variable not found, add it
	return append(append([]string{}, envSlice...), fmt.Sprintf(envVarFormat, key, value))
}

// ConvertMapStringToAny converts a map[string]string to map[string]any.
// Returns nil if the input map is nil.
func ConvertMapStringToAny(env map[string]string) map[string]any {
	defer perf.Track(nil, "env.ConvertMapStringToAny")()

	if env == nil {
		return nil
	}
	result := make(map[string]any, len(env))
	for k, v := range env {
		result[k] = v
	}
	return result
}

// EnvironToMapFiltered converts environment variables to a map, excluding specified keys and prefixes.
// This is useful for terraform-exec which prohibits certain environment variables.
func EnvironToMapFiltered(excludeKeys []string, excludePrefixes []string) map[string]string {
	defer perf.Track(nil, "env.EnvironToMapFiltered")()

	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		pair := splitStringAtFirstOccurrence(e, "=")
		k := pair[0]
		v := pair[1]

		// Check if key should be excluded.
		excluded := false
		for _, excludeKey := range excludeKeys {
			if k == excludeKey {
				excluded = true
				break
			}
		}
		if !excluded {
			for _, prefix := range excludePrefixes {
				if strings.HasPrefix(k, prefix) {
					excluded = true
					break
				}
			}
		}

		if !excluded {
			envMap[k] = v
		}
	}
	return envMap
}
