// Package env provides utilities for working with environment variables.
package env

import (
	"fmt"

	"github.com/cloudposse/atmos/pkg/perf"
)

// MergeGlobalEnv merges atmos.yaml global env into an environment slice.
// Global env has lowest priority and is prepended (can be overridden by command-specific env).
// The baseEnv slice typically comes from os.Environ().
func MergeGlobalEnv(baseEnv []string, globalEnv map[string]string) []string {
	defer perf.Track(nil, "env.MergeGlobalEnv")()

	if len(globalEnv) == 0 {
		return baseEnv
	}

	// Convert global env map to slice and prepend to base env.
	// This ensures global env can be overridden by any subsequent env vars.
	globalEnvSlice := ConvertMapToSlice(globalEnv)

	// Prepend global env: os.Environ() already contains the system env,
	// and we want global env to come after system env but before command-specific env.
	// So we append global env to base env (which is typically os.Environ()).
	result := make([]string, 0, len(baseEnv)+len(globalEnvSlice))
	result = append(result, baseEnv...)
	result = append(result, globalEnvSlice...)

	return result
}

// ConvertMapToSlice converts a map[string]string to []string{"KEY=value", ...}.
// The order of the resulting slice is not guaranteed due to map iteration.
func ConvertMapToSlice(envMap map[string]string) []string {
	defer perf.Track(nil, "env.ConvertMapToSlice")()

	if envMap == nil {
		return []string{}
	}

	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}

// ApplyGlobalEnvToSlice applies global env vars to an environment slice.
// For each key in globalEnv, if it doesn't exist in envSlice, it is added.
// If it exists, it is NOT overwritten (preserves higher-priority values).
// This is useful when global env needs to be applied as defaults.
func ApplyGlobalEnvToSlice(envSlice []string, globalEnv map[string]string) []string {
	defer perf.Track(nil, "env.ApplyGlobalEnvToSlice")()

	if len(globalEnv) == 0 {
		return envSlice
	}

	// Build a set of existing keys for quick lookup.
	existingKeys := make(map[string]bool, len(envSlice))
	for _, envVar := range envSlice {
		pair := splitStringAtFirstOccurrence(envVar, "=")
		existingKeys[pair[0]] = true
	}

	// Add global env vars that don't already exist.
	result := make([]string, 0, len(envSlice)+len(globalEnv))
	result = append(result, envSlice...)

	for k, v := range globalEnv {
		if !existingKeys[k] {
			result = append(result, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return result
}
