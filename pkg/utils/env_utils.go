package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// PathPrefixLength is the length of "PATH=" prefix.
	pathPrefixLength = 5
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

// PrependToPath adds a directory to the beginning of the PATH environment variable.
// Returns the new PATH value.
func PrependToPath(currentPath, newDir string) string {
	if currentPath == "" {
		return newDir
	}
	return fmt.Sprintf("%s%c%s", newDir, os.PathListSeparator, currentPath)
}

// UpdateEnvironmentPath updates the PATH in an environment slice.
// Returns a new environment slice with the updated PATH.
func UpdateEnvironmentPath(env []string, newDir string) []string {
	var updatedEnv []string
	pathUpdated := false

	for _, envVar := range env {
		if strings.HasPrefix(envVar, "PATH=") {
			currentPath := envVar[pathPrefixLength:] // Remove "PATH=" prefix
			newPath := fmt.Sprintf("PATH=%s", PrependToPath(currentPath, newDir))
			updatedEnv = append(updatedEnv, newPath)
			pathUpdated = true
		} else {
			updatedEnv = append(updatedEnv, envVar)
		}
	}

	// If PATH wasn't found, add it
	if !pathUpdated {
		updatedEnv = append(updatedEnv, fmt.Sprintf("PATH=%s", newDir))
	}

	return updatedEnv
}

// GetPathFromEnvironment extracts the PATH value from an environment slice.
func GetPathFromEnvironment(env []string) string {
	for _, envVar := range env {
		if strings.HasPrefix(envVar, "PATH=") {
			return envVar[pathPrefixLength:] // Remove "PATH=" prefix
		}
	}
	return ""
}

// EnsureBinaryInPath checks if a binary directory is in PATH and adds it if missing.
// Returns updated environment with the binary directory prepended to PATH.
func EnsureBinaryInPath(env []string, binaryPath string) []string {
	binaryDir := filepath.Dir(binaryPath)
	currentPath := GetPathFromEnvironment(env)

	// Check if binary directory is already in PATH
	if strings.Contains(currentPath, binaryDir) {
		return env // Already in PATH
	}

	return UpdateEnvironmentPath(env, binaryDir)
}
