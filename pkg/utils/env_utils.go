package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// PathPrefixLength is the length of "PATH=" prefix.
	pathPrefixLength = 5
	// EnvVarFormat is the format string for environment variables.
	envVarFormat = "%s=%s"
)

// ConvertEnvVars converts ENV vars from a map to a list of strings in the format ["key1=val1", "key2=val2", "key3=val3" ...].
func ConvertEnvVars(envVarsMap map[string]any) []string {
	defer perf.Track(nil, "utils.ConvertEnvVars")()

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
	defer perf.Track(nil, "utils.EnvironToMap")()

	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		pair := SplitStringAtFirstOccurrence(env, "=")
		k := pair[0]
		v := pair[1]
		envMap[k] = v
	}
	return envMap
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
	defer perf.Track(nil, "utils.PrependToPath")()

	if currentPath == "" {
		return newDir
	}
	return fmt.Sprintf("%s%c%s", newDir, os.PathListSeparator, currentPath)
}

// findPathIndex returns the index of the PATH entry (case-insensitive) and the key casing to use ("PATH" or "Path").
func findPathIndex(env []string) (int, string) {
	defer perf.Track(nil, "utils.findPathIndex")()

	for i, envVar := range env {
		if len(envVar) >= pathPrefixLength && strings.EqualFold(envVar[:pathPrefixLength], "PATH=") {
			return i, envVar[:pathPrefixLength-1] // keep existing key's casing (exclude "=").
		}
	}
	return -1, "PATH"
}

// UpdateEnvironmentPath updates the PATH in an environment slice.
// Returns a new environment slice with the updated PATH.
func UpdateEnvironmentPath(env []string, newDir string) []string {
	defer perf.Track(nil, "utils.UpdateEnvironmentPath")()

	idx, key := findPathIndex(env) // case-insensitive match for "PATH=".
	if idx >= 0 {
		updated := make([]string, 0, len(env))
		for i, envVar := range env {
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
	return append(append([]string{}, env...), fmt.Sprintf(envVarFormat, "PATH", newDir))
}

// GetPathFromEnvironment extracts the PATH value from an environment slice.
func GetPathFromEnvironment(env []string) string {
	defer perf.Track(nil, "utils.GetPathFromEnvironment")()

	idx, key := findPathIndex(env)
	if idx >= 0 {
		return env[idx][len(key)+1:] // Remove "KEY=" prefix
	}
	return ""
}

// EnsureBinaryInPath checks if a binary directory is in PATH and adds it if missing.
// Returns updated environment with the binary directory prepended to PATH.
func EnsureBinaryInPath(env []string, binaryPath string) []string {
	defer perf.Track(nil, "utils.EnsureBinaryInPath")()

	binaryDir := filepath.Dir(binaryPath)
	currentPath := GetPathFromEnvironment(env)

	// Check if binary directory is already in PATH
	if strings.Contains(currentPath, binaryDir) {
		return env // Already in PATH
	}

	return UpdateEnvironmentPath(env, binaryDir)
}

// UpdateEnvVar updates or adds an environment variable in an environment slice.
// Returns a new environment slice with the variable updated.
func UpdateEnvVar(env []string, key, value string) []string {
	defer perf.Track(nil, "utils.UpdateEnvVar")()

	keyPrefix := key + "="

	// Look for existing variable (case-sensitive for non-PATH variables)
	for i, envVar := range env {
		if strings.HasPrefix(envVar, keyPrefix) {
			// Update existing variable
			updated := make([]string, 0, len(env))
			for j, e := range env {
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
	return append(append([]string{}, env...), fmt.Sprintf(envVarFormat, key, value))
}
