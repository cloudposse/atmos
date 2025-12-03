// Package env provides utilities for working with environment variables.
package env

import (
	"fmt"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MergeGlobalEnv merges atmos.yaml global env into an environment slice.
// Global env has lowest priority and is appended (can be overridden by command-specific env).
// The baseEnv slice typically comes from os.Environ().
func MergeGlobalEnv(baseEnv []string, globalEnv map[string]string) []string {
	defer perf.Track(nil, "env.MergeGlobalEnv")()

	if len(globalEnv) == 0 {
		return baseEnv
	}

	// Convert global env map to slice and append to base env.
	// This ensures global env can be overridden by any subsequent env vars.
	globalEnvSlice := ConvertMapToSlice(globalEnv)

	// Append global env: os.Environ() already contains the system env,
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

// MergeSystemEnvWithGlobal merges system environment variables with global env from atmos.yaml
// and component-specific env. Priority order: system env < global env < component env.
// Special handling is applied for TF_CLI_ARGS_* variables where new values are prepended to existing values.
func MergeSystemEnvWithGlobal(componentEnvList []string, globalEnv map[string]string) []string {
	defer perf.Track(nil, "env.MergeSystemEnvWithGlobal")()

	return mergeSystemEnvInternal(componentEnvList, globalEnv, true)
}

// MergeSystemEnv merges system environment variables with component-specific env.
// Priority order: system env < component env.
// Special handling is applied for TF_CLI_ARGS_* variables where new values are prepended to existing values.
func MergeSystemEnv(componentEnvList []string) []string {
	defer perf.Track(nil, "env.MergeSystemEnv")()

	return mergeSystemEnvInternal(componentEnvList, nil, true)
}

// MergeSystemEnvSimpleWithGlobal merges system environment variables with global env from atmos.yaml
// and new env without TF_CLI_ARGS_* special handling.
// Priority order: system env < global env < new env list.
func MergeSystemEnvSimpleWithGlobal(newEnvList []string, globalEnv map[string]string) []string {
	defer perf.Track(nil, "env.MergeSystemEnvSimpleWithGlobal")()

	return mergeSystemEnvInternal(newEnvList, globalEnv, false)
}

// MergeSystemEnvSimple merges system environment variables with new env without TF_CLI_ARGS_* special handling.
// Priority order: system env < new env list.
func MergeSystemEnvSimple(newEnvList []string) []string {
	defer perf.Track(nil, "env.MergeSystemEnvSimple")()

	return mergeSystemEnvInternal(newEnvList, nil, false)
}

// mergeSystemEnvInternal is the shared implementation for all MergeSystemEnv* functions.
// It handles merging system env, optional global env, and component/new env.
// If handleTFCliArgs is true, TF_CLI_ARGS_* variables get special handling (prepend instead of replace).
func mergeSystemEnvInternal(envList []string, globalEnv map[string]string, handleTFCliArgs bool) []string {
	envMap := make(map[string]string)

	// Parse system environment variables.
	for _, env := range os.Environ() {
		if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// Apply global env from atmos.yaml if provided (can override system env).
	for k, v := range globalEnv {
		envMap[k] = v
	}

	// Merge with new environment variables (highest priority).
	for _, env := range envList {
		if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
			if handleTFCliArgs && strings.HasPrefix(parts[0], "TF_CLI_ARGS_") {
				// For TF_CLI_ARGS_* variables, prepend new values to existing values.
				if existing, exists := envMap[parts[0]]; exists {
					// Put the new, Atmos defined value first so it takes precedence.
					envMap[parts[0]] = parts[1] + " " + existing
				} else {
					envMap[parts[0]] = parts[1]
				}
			} else {
				// For all other environment variables, just override any existing value.
				envMap[parts[0]] = parts[1]
			}
		}
	}

	// Convert back to slice.
	merged := make([]string, 0, len(envMap))
	for k, v := range envMap {
		log.Trace("Setting ENV var", "key", k, "value", v)
		merged = append(merged, k+"="+v)
	}
	return merged
}
