package telemetry

import (
	"os"
	"sort"
	"strings"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	ciEnvVar       = "CI"
	logKeyProvider = "provider"
)

var (
	// Inspired by https://github.com/watson/ci-info .

	// Map of CI providers that can be detected by checking if specific environment variables exist.
	ciProvidersEnvVarsExists = map[string]string{
		"CODEBUILD":          "CODEBUILD_BUILD_ARN",
		"AZURE_PIPELINES":    "TF_BUILD",
		"BAMBOO":             "bamboo_planKey",
		"BITBUCKET":          "BITBUCKET_COMMIT",
		"BITRISE":            "BITRISE_IO",
		"BUILDKITE":          "BUILDKITE",
		"CIRCLE":             "CIRCLECI",
		"CIRRUS":             "CIRRUS_CI",
		"CODEFRESH":          "CF_BUILD_ID",
		"DRONE":              "DRONE",
		"GERRIT":             "GERRIT_PROJECT",
		"GITEA_ACTIONS":      "GITEA_ACTIONS",
		"GITHUB_ACTIONS":     "GITHUB_ACTIONS",
		"GITLAB":             "GITLAB_CI",
		"GOCD":               "GO_PIPELINE_LABEL",
		"GOOGLE_CLOUD_BUILD": "BUILDER_OUTPUT",
		"HARNESS":            "HARNESS_BUILD_ID",
		"HUDSON":             "HUDSON_URL",
		"PROW":               "PROW_JOB_ID",
		"SEMAPHORE":          "SEMAPHORE",
		"TEAMCITY":           "TEAMCITY_VERSION",
		"TRAVIS":             "TRAVIS",
		"SPACELIFT":          "TF_VAR_spacelift_run_id",
	}

	// Map of CI providers that require ALL listed environment variables to exist.
	// Jenkins requires both JENKINS_URL and BUILD_ID to avoid false positives from build-harness.
	ciProvidersEnvVarsAllExist = map[string][]string{
		"JENKINS": {"JENKINS_URL", "BUILD_ID"},
	}

	// Map of CI providers that can be detected by checking if environment variables equal specific values.
	ciProvidersEnvVarsEquals = map[string]map[string]string{
		"CODESHIP": {
			"CI_NAME": "codeship",
		},
		"SOURCEHUT": {
			"CI_NAME": "sourcehut",
		},
	}
)

// isEnvVarExists checks if an environment variable exists and is not empty.
func isEnvVarExists(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}

// isEnvVarEquals checks if an environment variable exists and equals the specified value.
func isEnvVarEquals(key string, value string) bool {
	return isEnvVarExists(key) && os.Getenv(key) == value
}

// isEnvVarTrue checks if an environment variable exists and equals "true".
func isEnvVarTrue(key string) bool {
	return isEnvVarEquals(key, "true")
}

// IsCI determines if the current environment is a CI/CD environment.
// Returns true if CI=true or if a specific CI provider is detected.
func IsCI() bool {
	ciEnvTrue := isEnvVarTrue(ciEnvVar)
	provider := ciProvider()

	return ciEnvTrue || provider != ""
}

// PreserveCIEnvVars temporarily removes CI-related environment variables from the current process
// and returns a map containing the original values. This is useful for testing scenarios
// where you want to ensure a clean environment without CI detection.
//
// The function handles three categories of CI environment variables:
// 1. Variables from ciProvidersEnvVarsExists (existence-based detection)
// 2. Variables from ciProvidersEnvVarsEquals (value-based detection)
// 3. The general "CI" environment variable
//
// Returns a map of preserved environment variable names to their original values.
func PreserveCIEnvVars() map[string]string {
	// Initialize map to store original environment variable values
	envVars := make(map[string]string)

	// Preserve and unset CI provider variables that are detected by existence.
	for _, envVar := range ciProvidersEnvVarsExists {
		if isEnvVarExists(envVar) {
			envVars[envVar] = os.Getenv(envVar) //nolint:forbidigo // Legitimate use for CI env preservation
			os.Unsetenv(envVar)
		}
	}

	// Preserve and unset CI provider variables that are detected by specific values.
	for _, values := range ciProvidersEnvVarsEquals {
		for valueKey := range values {
			if isEnvVarExists(valueKey) {
				envVars[valueKey] = os.Getenv(valueKey)
				os.Unsetenv(valueKey)
			}
		}
	}

	// Preserve and unset the general CI environment variable.
	if isEnvVarExists(ciEnvVar) {
		envVars[ciEnvVar] = os.Getenv(ciEnvVar)
		os.Unsetenv(ciEnvVar)
	}

	return envVars
}

// RestoreCIEnvVars restores previously preserved CI environment variables back to the system.
// This function is typically called in a defer statement after PreserveCIEnvVars to ensure
// the original environment is restored, even if the calling function panics or returns early.
//
// Parameters:
//   - envVars: A map of environment variable names to their original values, typically
// RestoreCIEnvVars restores environment variables previously preserved by PreserveCIEnvVars.
// It sets each key in envVars back into the process environment with its saved value.
// If envVars is nil or empty, RestoreCIEnvVars does nothing.
func RestoreCIEnvVars(envVars map[string]string) {
	// Restore each environment variable to its original value
	for key, value := range envVars {
		os.Setenv(key, value)
	}
}

// FilterCIEnvVars removes CI-related environment variables from an env slice.
// Unlike PreserveCIEnvVars, this is a pure function that does not modify process
// environment, making it safe to call from parallel tests.
//
// Parameters:
//   - env: A slice of "KEY=VALUE" strings (e.g. from os.Environ() or cmd.Env).
//
// FilterCIEnvVars returns a copy of the provided environment slice with all known CI-related
// environment variables removed.
// It accepts entries in the form "KEY=VALUE" or "KEY" and excludes any entry whose key matches
// a CI variable name from the configured provider sets or the general "CI" variable. The input
// slice is not modified.
func FilterCIEnvVars(env []string) []string {
	ciVars := getCIEnvVarSet()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		key := e
		if idx := strings.IndexByte(e, '='); idx >= 0 {
			key = e[:idx]
		}
		if _, isCIVar := ciVars[key]; !isCIVar {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// ciEnvVarSetOnce guards one-time initialisation of ciEnvVarSetCache.
var ciEnvVarSetOnce sync.Once

// ciEnvVarSetCache holds the lazily-initialised set of CI env-var names.
var ciEnvVarSetCache map[string]struct{}

// getCIEnvVarSet returns the cached set of all known CI environment variable names.
// The set is built exactly once and reused across all FilterCIEnvVars calls.
// getCIEnvVarSet returns a cached set of all known CI-related environment variable names.
// The set is initialized once on first call and is safe for concurrent use.
// The returned map must be treated as read-only and must not be modified by callers.
func getCIEnvVarSet() map[string]struct{} {
	ciEnvVarSetOnce.Do(func() {
		ciEnvVarSetCache = buildCIEnvVarSet()
	})
	return ciEnvVarSetCache
}

// buildCIEnvVarSet constructs a set of all environment variable names used to detect CI providers.
// The returned map's keys are the variable names (values are empty structs); it includes provider-specific
// keys and the general "CI" variable.
func buildCIEnvVarSet() map[string]struct{} {
	ciVars := make(map[string]struct{})
	for _, envVar := range ciProvidersEnvVarsExists {
		ciVars[envVar] = struct{}{}
	}
	for _, values := range ciProvidersEnvVarsEquals {
		for valueKey := range values {
			ciVars[valueKey] = struct{}{}
		}
	}
	for _, vars := range ciProvidersEnvVarsAllExist {
		for _, v := range vars {
			ciVars[v] = struct{}{}
		}
	}
	ciVars[ciEnvVar] = struct{}{}
	return ciVars
}

// applyAlphabeticalOrder is a generic function that processes a map in alphabetical order.
// It applies a filter function to each value and returns the first key where the filter returns true.
// V can be either string or map[string]string.
func applyAlphabeticalOrder[V string | map[string]string](table map[string]V, filter func(V) bool) string {
	// Extract all keys from the map
	var keys []string
	for key := range table {
		keys = append(keys, key)
	}
	// Sort keys alphabetically for consistent ordering.
	sort.Strings(keys)
	// Apply filter to each value in alphabetical order.
	for _, key := range keys {
		if filter(table[key]) {
			return key
		}
	}
	return ""
}

// ciProvider detects which CI/CD provider is currently running.
// Returns the name of the detected provider or empty string if none found.
func ciProvider() string {
	// First, check providers that require ALL specified environment variables to exist.
	// This prevents false positives (e.g., Jenkins from build-harness).
	// Sort keys alphabetically for consistent ordering.
	var allExistKeys []string
	for key := range ciProvidersEnvVarsAllExist {
		allExistKeys = append(allExistKeys, key)
	}
	sort.Strings(allExistKeys)
	for _, key := range allExistKeys {
		vars := ciProvidersEnvVarsAllExist[key]
		allExist := true
		for _, envVar := range vars {
			if !isEnvVarExists(envVar) {
				allExist = false
				break
			}
		}
		if allExist {
			log.Debug("CI provider detected", logKeyProvider, key, "env", strings.Join(vars, ","))
			return key
		}
	}

	// Then, check providers that can be detected by single environment variable existence.
	// Process in alphabetical order for consistent results.
	if result := applyAlphabeticalOrder(ciProvidersEnvVarsExists, isEnvVarExists); result != "" {
		// Log which specific env var was detected.
		if envVar, exists := ciProvidersEnvVarsExists[result]; exists {
			log.Debug("CI provider detected", logKeyProvider, result, "env", envVar)
		}
		return result
	}

	// Helper function to check if any environment variable in the map equals its expected value.
	checkEnvVarsEquals := func(key map[string]string) bool {
		for envName, envValue := range key {
			if isEnvVarEquals(envName, envValue) {
				return true
			}
		}
		return false
	}

	// Finally, check providers that require specific environment variable values.
	// Process in alphabetical order for consistent results.
	if result := applyAlphabeticalOrder(ciProvidersEnvVarsEquals, checkEnvVarsEquals); result != "" {
		if envVars, exists := ciProvidersEnvVarsEquals[result]; exists {
			var detectedVars []string
			for envName := range envVars {
				if _, found := os.LookupEnv(envName); found {
					detectedVars = append(detectedVars, envName)
				}
			}
			if len(detectedVars) > 0 {
				log.Debug("CI provider detected", logKeyProvider, result, "env", strings.Join(detectedVars, ","))
			}
		}
		return result
	}

	// No CI provider detected.
	return ""
}
