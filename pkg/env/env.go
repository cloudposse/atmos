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
	// PathPrefixLength is the length of "PATH=" prefix.
	pathPrefixLength = 5
	// EnvVarFormat is the format string for environment variables.
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
	defer perf.Track(nil, "env.CommandEnvToMap")()

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

// Builder provides a fluent API for constructing environment variable slices.
type Builder struct {
	env []string
}

// NewBuilder creates a new Builder with an optional initial environment slice.
// If no initial slice is provided, starts with an empty slice.
func NewBuilder(initial ...[]string) *Builder {
	defer perf.Track(nil, "env.NewBuilder")()

	var env []string
	if len(initial) > 0 && initial[0] != nil {
		// Copy to avoid mutating the original slice.
		env = make([]string, len(initial[0]))
		copy(env, initial[0])
	}
	return &Builder{env: env}
}

// WithEnv adds an environment variable in KEY=value format.
func (b *Builder) WithEnv(keyValue string) *Builder {
	defer perf.Track(nil, "env.Builder.WithEnv")()

	b.env = append(b.env, keyValue)
	return b
}

// WithEnvVar adds an environment variable with the given key and value.
func (b *Builder) WithEnvVar(key, value string) *Builder {
	defer perf.Track(nil, "env.Builder.WithEnvVar")()

	b.env = append(b.env, fmt.Sprintf(envVarFormat, key, value))
	return b
}

// WithEnvVarf adds an environment variable using a format string for the value.
func (b *Builder) WithEnvVarf(key, format string, args ...any) *Builder {
	defer perf.Track(nil, "env.Builder.WithEnvVarf")()

	value := fmt.Sprintf(format, args...)
	b.env = append(b.env, fmt.Sprintf(envVarFormat, key, value))
	return b
}

// WithEnvMap adds all environment variables from a map.
func (b *Builder) WithEnvMap(envMap map[string]string) *Builder {
	defer perf.Track(nil, "env.Builder.WithEnvMap")()

	for k, v := range envMap {
		b.env = append(b.env, fmt.Sprintf(envVarFormat, k, v))
	}
	return b
}

// Build returns the constructed environment slice.
func (b *Builder) Build() []string {
	defer perf.Track(nil, "env.Builder.Build")()

	return b.env
}
